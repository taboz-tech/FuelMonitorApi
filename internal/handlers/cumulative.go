package handlers

import (
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"fuel-monitor-api/internal/database"
	"fuel-monitor-api/internal/middleware"
	"fuel-monitor-api/internal/models"

	"github.com/gin-gonic/gin"
)

type CumulativeHandler struct {
	DB *database.DB
}

func NewCumulativeHandler(db *database.DB) *CumulativeHandler {
	return &CumulativeHandler{
		DB: db,
	}
}

// GetCumulativeReadings processes cumulative readings for a specific date
func (h *CumulativeHandler) GetCumulativeReadings(c *gin.Context) {
	user, exists := middleware.GetUserFromContext(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, models.ErrorResponse{
			Message: "Authentication required",
		})
		return
	}

	var req models.CumulativeReadingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Message: "Invalid request format",
		})
		return
	}

	// Parse target date
	targetDate, err := h.parseDate(req.Date)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Message: "Invalid date format. Use DD/MM/YYYY or YYYY-MM-DD",
		})
		return
	}

	dateString := targetDate.Format("2006-01-02")
	log.Printf("Processing cumulative readings for %s requested by %s", dateString, user.Username)

	// Get user's accessible sites
	sites, err := h.DB.GetSitesForUser(user.ID, user.Role)
	if err != nil {
		log.Printf("Failed to get sites for user %s: %v", user.Username, err)
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Message: "Failed to get sites",
		})
		return
	}

	if len(sites) == 0 {
		response := models.CumulativeReadingsResponse{
			Date:        dateString,
			ProcessedAt: time.Now().Format(time.RFC3339),
			User: models.UserInfo{
				Username: user.Username,
				Role:     user.Role,
			},
			Sites: []models.CumulativeSiteResult{},
			Summary: models.CumulativeSummary{
				TotalSites:          0,
				ProcessedSites:      0,
				ErrorSites:          0,
				TotalFuelConsumed:   0,
				TotalFuelTopped:     0,
				TotalGeneratorHours: 0,
				TotalZesaHours:      0,
				TotalOfflineHours:   0,
			},
		}
		c.JSON(http.StatusOK, response)
		return
	}

	log.Printf("Processing %d sites for date %s", len(sites), dateString)

	// Check for existing cumulative readings (for status determination only)
	existingReadings, err := h.DB.GetExistingCumulativeReadings(dateString, sites)
	if err != nil {
		log.Printf("Failed to get existing readings: %v", err)
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Message: "Failed to check existing readings",
		})
		return
	}

	// Create map of existing readings for status determination
	existingBySiteID := make(map[int]*models.CumulativeReading)
	for _, reading := range existingReadings {
		existingBySiteID[reading.SiteID] = reading
	}

	// Process sites in parallel batches
	results := h.processSitesInBatches(sites, existingBySiteID, targetDate, dateString)

	// Calculate summary
	summary := h.calculateSummary(results, len(sites))

	response := models.CumulativeReadingsResponse{
		Date:        dateString,
		ProcessedAt: time.Now().Format(time.RFC3339),
		User: models.UserInfo{
			Username: user.Username,
			Role:     user.Role,
		},
		Sites:   results,
		Summary: summary,
	}

	log.Printf("Cumulative readings completed for %s: %+v", dateString, summary)

	// Ensure response is sent
	c.Header("Content-Type", "application/json")
	c.JSON(http.StatusOK, response)
	log.Printf("Response sent successfully for %s", dateString)
}

// parseDate handles both DD/MM/YYYY and YYYY-MM-DD formats
func (h *CumulativeHandler) parseDate(dateStr string) (time.Time, error) {
	if dateStr == "" {
		now := time.Now()
		return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()), nil
	}

	// Try DD/MM/YYYY format first
	if len(dateStr) == 10 && dateStr[2] == '/' && dateStr[5] == '/' {
		return time.Parse("02/01/2006", dateStr)
	}

	// Try YYYY-MM-DD format
	return time.Parse("2006-01-02", dateStr)
}

// processSitesInBatches processes sites in parallel batches
func (h *CumulativeHandler) processSitesInBatches(sites []*models.Site, existingReadings map[int]*models.CumulativeReading, targetDate time.Time, dateString string) []models.CumulativeSiteResult {
	const batchSize = 10
	var allResults []models.CumulativeSiteResult
	var resultMutex sync.Mutex

	var wg sync.WaitGroup

	for i := 0; i < len(sites); i += batchSize {
		end := i + batchSize
		if end > len(sites) {
			end = len(sites)
		}
		batch := sites[i:end]

		wg.Add(1)
		go func(batchSites []*models.Site) {
			defer wg.Done()

			batchResults := h.processBatch(batchSites, existingReadings, targetDate, dateString)

			resultMutex.Lock()
			allResults = append(allResults, batchResults...)
			resultMutex.Unlock()
		}(batch)
	}

	wg.Wait()

	// Sort by fuel consumed (highest first)
	h.sortResultsByFuelConsumed(allResults)

	return allResults
}

// processBatch processes a batch of sites
func (h *CumulativeHandler) processBatch(sites []*models.Site, existingReadings map[int]*models.CumulativeReading, targetDate time.Time, dateString string) []models.CumulativeSiteResult {
	var results []models.CumulativeSiteResult

	for _, site := range sites {
		result := h.processSingleSite(site, existingReadings[site.ID], targetDate, dateString)
		results = append(results, result)
	}

	return results
}

// processSingleSite processes a single site
func (h *CumulativeHandler) processSingleSite(site *models.Site, existingReading *models.CumulativeReading, targetDate time.Time, dateString string) models.CumulativeSiteResult {
	log.Printf("Processing site: %s (%s)", site.Name, site.DeviceID)

	// Calculate fuel and power metrics in parallel
	var fuelMetrics models.FuelMetrics
	var powerMetrics models.PowerMetrics
	var wg sync.WaitGroup
	var fuelErr, powerErr error

	wg.Add(2)

	go func() {
		defer wg.Done()
		fuelMetrics, fuelErr = h.DB.CalculateFuelChanges(site.DeviceID, targetDate)
	}()

	go func() {
		defer wg.Done()
		powerMetrics, powerErr = h.DB.CalculatePowerRuntimes(site.DeviceID, targetDate)
	}()

	wg.Wait()

	if fuelErr != nil || powerErr != nil {
		log.Printf("Error calculating metrics for site %s: fuel=%v, power=%v", site.Name, fuelErr, powerErr)
		return models.CumulativeSiteResult{
			SiteID:   site.ID,
			SiteName: site.Name,
			DeviceID: site.DeviceID,
			Status:   "ERROR",
			Error:    fmt.Sprintf("Calculation error: fuel=%v, power=%v", fuelErr, powerErr),
		}
	}

	// Use UPSERT - automatically handles create or update
	log.Printf("Creating/updating cumulative reading for %s", site.Name)
	_, err := h.DB.CreateOrUpdateCumulativeReading(site.ID, site.DeviceID, dateString, fuelMetrics, powerMetrics)

	var status string
	if err != nil {
		log.Printf("Error saving cumulative reading for site %s: %v", site.Name, err)
		return models.CumulativeSiteResult{
			SiteID:   site.ID,
			SiteName: site.Name,
			DeviceID: site.DeviceID,
			Status:   "ERROR",
			Error:    err.Error(),
		}
	}

	// Determine status based on whether record existed
	if existingReading != nil {
		status = "UPDATED"
	} else {
		status = "CREATED"
	}

	return models.CumulativeSiteResult{
		SiteID:              site.ID,
		SiteName:            site.Name,
		DeviceID:            site.DeviceID,
		FuelConsumed:        fuelMetrics.TotalFuelConsumed,
		FuelTopped:          fuelMetrics.TotalFuelTopped,
		FuelConsumedPercent: fuelMetrics.FuelConsumedPercent,
		FuelToppedPercent:   fuelMetrics.FuelToppedPercent,
		GeneratorHours:      powerMetrics.TotalGeneratorRuntime,
		ZesaHours:           powerMetrics.TotalZesaRuntime,
		OfflineHours:        powerMetrics.TotalOfflineTime,
		Status:              status,
		CalculatedAt:        time.Now(),
	}
}

// calculateSummary calculates the summary statistics
func (h *CumulativeHandler) calculateSummary(results []models.CumulativeSiteResult, totalSites int) models.CumulativeSummary {
	var totalFuelConsumed, totalFuelTopped, totalGeneratorHours, totalZesaHours, totalOfflineHours float64
	var processedSites, errorSites int

	for _, result := range results {
		if result.Status == "ERROR" {
			errorSites++
		} else {
			processedSites++
			totalFuelConsumed += result.FuelConsumed
			totalFuelTopped += result.FuelTopped
			totalGeneratorHours += result.GeneratorHours
			totalZesaHours += result.ZesaHours
			totalOfflineHours += result.OfflineHours
		}
	}

	return models.CumulativeSummary{
		TotalSites:          totalSites,
		ProcessedSites:      processedSites,
		ErrorSites:          errorSites,
		TotalFuelConsumed:   h.roundToDecimal(totalFuelConsumed, 1),
		TotalFuelTopped:     h.roundToDecimal(totalFuelTopped, 1),
		TotalGeneratorHours: h.roundToDecimal(totalGeneratorHours, 2),
		TotalZesaHours:      h.roundToDecimal(totalZesaHours, 2),
		TotalOfflineHours:   h.roundToDecimal(totalOfflineHours, 2),
	}
}

// sortResultsByFuelConsumed sorts results by fuel consumed in descending order
func (h *CumulativeHandler) sortResultsByFuelConsumed(results []models.CumulativeSiteResult) {
	for i := 0; i < len(results)-1; i++ {
		for j := i + 1; j < len(results); j++ {
			if results[j].FuelConsumed > results[i].FuelConsumed {
				results[i], results[j] = results[j], results[i]
			}
		}
	}
}

// roundToDecimal rounds a float to specified decimal places
func (h *CumulativeHandler) roundToDecimal(val float64, decimals int) float64 {
	multiplier := 1.0
	for i := 0; i < decimals; i++ {
		multiplier *= 10
	}
	return float64(int(val*multiplier+0.5)) / multiplier
}

// GetCumulativeReadingsByDateRange retrieves cumulative readings for a date range
func (h *CumulativeHandler) GetCumulativeReadingsByDateRange(c *gin.Context) {
	user, exists := middleware.GetUserFromContext(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, models.ErrorResponse{
			Message: "Authentication required",
		})
		return
	}

	// Get query parameters
	startDateStr := c.Query("startDate")
	endDateStr := c.Query("endDate")

	if startDateStr == "" {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Message: "startDate parameter is required",
		})
		return
	}

	// Parse dates
	startDate, err := h.parseDate(startDateStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Message: "Invalid startDate format. Use DD/MM/YYYY or YYYY-MM-DD",
		})
		return
	}

	var endDate time.Time
	if endDateStr != "" {
		endDate, err = h.parseDate(endDateStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, models.ErrorResponse{
				Message: "Invalid endDate format. Use DD/MM/YYYY or YYYY-MM-DD",
			})
			return
		}
	} else {
		endDate = startDate
	}

	startDateString := startDate.Format("2006-01-02")
	endDateString := endDate.Format("2006-01-02")

	log.Printf("Getting cumulative readings from %s to %s for user: %s", startDateString, endDateString, user.Username)

	// Get user's accessible sites
	sites, err := h.DB.GetSitesForUser(user.ID, user.Role)
	if err != nil {
		log.Printf("Failed to get sites for user %s: %v", user.Username, err)
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Message: "Failed to get sites",
		})
		return
	}

	if len(sites) == 0 {
		log.Printf("No accessible sites for user: %s", user.Username)
		response := models.CumulativeReadingsRangeResponse{
			Sites: []models.CumulativeSiteRangeResult{},
			Summary: models.CumulativeRangeSummary{
				DateRange: models.DateRange{
					Start: startDateString,
					End:   endDateString,
				},
				TotalSites:          0,
				TotalFuelConsumed:   0,
				TotalGeneratorHours: 0,
				TotalZesaHours:      0,
				AverageFuelPerSite:  0,
				DaysIncluded:        h.calculateDaysDifference(startDate, endDate),
			},
		}
		c.JSON(http.StatusOK, response)
		return
	}

	log.Printf("Found %d accessible sites for %s (%s)", len(sites), user.Username, user.Role)

	// Get cumulative readings for the date range with parallel processing
	siteReadings := h.getCumulativeReadingsForRange(sites, startDateString, endDateString)

	// Calculate summary
	summary := h.calculateRangeSummary(siteReadings, startDateString, endDateString, startDate, endDate)

	response := models.CumulativeReadingsRangeResponse{
		Sites:   siteReadings,
		Summary: summary,
	}

	log.Printf("Cumulative readings range query completed: %s to %s, Sites: %d, Total Fuel: %.1fL, Gen Hours: %.2fh, Zesa Hours: %.2fh",
		startDateString, endDateString, len(siteReadings), summary.TotalFuelConsumed, summary.TotalGeneratorHours, summary.TotalZesaHours)

	c.Header("Content-Type", "application/json")
	c.JSON(http.StatusOK, response)
}

// getCumulativeReadingsForRange retrieves and aggregates cumulative readings for multiple sites in parallel
func (h *CumulativeHandler) getCumulativeReadingsForRange(sites []*models.Site, startDate, endDate string) []models.CumulativeSiteRangeResult {
	const batchSize = 20
	var allResults []models.CumulativeSiteRangeResult
	var resultMutex sync.Mutex

	var wg sync.WaitGroup

	// Process sites in parallel batches
	for i := 0; i < len(sites); i += batchSize {
		end := i + batchSize
		if end > len(sites) {
			end = len(sites)
		}
		batch := sites[i:end]

		wg.Add(1)
		go func(batchSites []*models.Site) {
			defer wg.Done()

			batchResults := h.processSiteRangeBatch(batchSites, startDate, endDate)

			resultMutex.Lock()
			allResults = append(allResults, batchResults...)
			resultMutex.Unlock()
		}(batch)
	}

	wg.Wait()

	// Sort by total fuel consumed (highest first)
	h.sortRangeResultsByFuelConsumed(allResults)

	return allResults
}

// processSiteRangeBatch processes a batch of sites for date range query
func (h *CumulativeHandler) processSiteRangeBatch(sites []*models.Site, startDate, endDate string) []models.CumulativeSiteRangeResult {
	var results []models.CumulativeSiteRangeResult

	for _, site := range sites {
		result := h.getSiteRangeData(site, startDate, endDate)
		if result != nil {
			results = append(results, *result)
		}
	}

	return results
}

// getSiteRangeData gets aggregated data for a single site over a date range
func (h *CumulativeHandler) getSiteRangeData(site *models.Site, startDate, endDate string) *models.CumulativeSiteRangeResult {
	query := `
		SELECT 
			COUNT(*) as reading_days,
			SUM(CAST(total_fuel_consumed AS DECIMAL)) as total_fuel_consumed,
			SUM(CAST(total_fuel_topped_up AS DECIMAL)) as total_fuel_topped,
			SUM(CAST(total_generator_runtime AS DECIMAL)) as total_generator_hours,
			SUM(CAST(total_zesa_runtime AS DECIMAL)) as total_zesa_hours,
			SUM(CAST(total_offline_time AS DECIMAL)) as total_offline_hours,
			MIN(date) as first_date,
			MAX(date) as last_date
		FROM cumulative_readings 
		WHERE site_id = $1 
		  AND date >= $2 
		  AND date <= $3
	`

	var readingDays int
	var totalFuelConsumed, totalFuelTopped, totalGeneratorHours, totalZesaHours, totalOfflineHours float64
	var firstDate, lastDate string

	err := h.DB.QueryRow(query, site.ID, startDate, endDate).Scan(
		&readingDays,
		&totalFuelConsumed,
		&totalFuelTopped,
		&totalGeneratorHours,
		&totalZesaHours,
		&totalOfflineHours,
		&firstDate,
		&lastDate,
	)

	if err != nil {
		log.Printf("Error getting range data for site %s: %v", site.Name, err)
		return nil
	}

	// Only return sites that have readings in the date range
	if readingDays == 0 {
		return nil
	}

	return &models.CumulativeSiteRangeResult{
		SiteID:              site.ID,
		SiteName:            site.Name,
		DeviceID:            site.DeviceID,
		TotalFuelConsumed:   h.roundToDecimal(totalFuelConsumed, 1),
		TotalFuelTopped:     h.roundToDecimal(totalFuelTopped, 1),
		TotalGeneratorHours: h.roundToDecimal(totalGeneratorHours, 2),
		TotalZesaHours:      h.roundToDecimal(totalZesaHours, 2),
		TotalOfflineHours:   h.roundToDecimal(totalOfflineHours, 2),
		ReadingDays:         readingDays,
		DateRange: models.DateRange{
			Start: firstDate,
			End:   lastDate,
		},
	}
}

// calculateRangeSummary calculates summary statistics for the date range
func (h *CumulativeHandler) calculateRangeSummary(results []models.CumulativeSiteRangeResult, startDate, endDate string, startDateTime, endDateTime time.Time) models.CumulativeRangeSummary {
	var totalFuelConsumed, totalFuelTopped, totalGeneratorHours, totalZesaHours, totalOfflineHours float64

	for _, result := range results {
		totalFuelConsumed += result.TotalFuelConsumed
		totalFuelTopped += result.TotalFuelTopped
		totalGeneratorHours += result.TotalGeneratorHours
		totalZesaHours += result.TotalZesaHours
		totalOfflineHours += result.TotalOfflineHours
	}

	var averageFuelPerSite float64
	if len(results) > 0 {
		averageFuelPerSite = totalFuelConsumed / float64(len(results))
	}

	return models.CumulativeRangeSummary{
		DateRange: models.DateRange{
			Start:   startDate,
			End:     endDate,
			IsRange: startDate != endDate,
		},
		TotalSites:          len(results),
		TotalFuelConsumed:   h.roundToDecimal(totalFuelConsumed, 1),
		TotalFuelTopped:     h.roundToDecimal(totalFuelTopped, 1),
		TotalGeneratorHours: h.roundToDecimal(totalGeneratorHours, 2),
		TotalZesaHours:      h.roundToDecimal(totalZesaHours, 2),
		TotalOfflineHours:   h.roundToDecimal(totalOfflineHours, 2),
		AverageFuelPerSite:  h.roundToDecimal(averageFuelPerSite, 1),
		DaysIncluded:        h.calculateDaysDifference(startDateTime, endDateTime),
	}
}

// calculateDaysDifference calculates the number of days between two dates
func (h *CumulativeHandler) calculateDaysDifference(startDate, endDate time.Time) int {
	if startDate.Equal(endDate) {
		return 1
	}
	diff := endDate.Sub(startDate)
	return int(diff.Hours()/24) + 1
}

// sortRangeResultsByFuelConsumed sorts results by total fuel consumed in descending order
func (h *CumulativeHandler) sortRangeResultsByFuelConsumed(results []models.CumulativeSiteRangeResult) {
	for i := 0; i < len(results)-1; i++ {
		for j := i + 1; j < len(results); j++ {
			if results[j].TotalFuelConsumed > results[i].TotalFuelConsumed {
				results[i], results[j] = results[j], results[i]
			}
		}
	}
}
