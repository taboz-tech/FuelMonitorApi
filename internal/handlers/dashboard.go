package handlers

import (
	"fmt"
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"fuel-monitor-api/internal/database"
	"fuel-monitor-api/internal/middleware"
	"fuel-monitor-api/internal/models"

	"github.com/gin-gonic/gin"
)

type DashboardHandler struct {
	DB *database.DB
}

func NewDashboardHandler(db *database.DB) *DashboardHandler {
	return &DashboardHandler{
		DB: db,
	}
}

// GetDashboard retrieves dashboard data with aggressive parallel optimization
func (h *DashboardHandler) GetDashboard(c *gin.Context) {
	startTime := time.Now()
	user, exists := middleware.GetUserFromContext(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, models.ErrorResponse{
			Message: "Authentication required",
		})
		return
	}

	log.Printf("DASHBOARD START: User=%s, Role=%s", user.Username, user.Role)

	// Parallel Step 1 & 2: Get view mode and sites simultaneously
	var viewMode string
	var sites []*models.Site
	var err error

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		viewMode = "closing"
		if user.Role == "admin" {
			if pref, err := h.DB.GetUserAdminPreference(user.ID); err == nil && pref != nil {
				viewMode = pref.ViewMode
			}
		}
	}()

	go func() {
		defer wg.Done()
		sites, err = h.DB.GetDashboardSitesForUser(user.ID, user.Role)
	}()

	wg.Wait()

	if err != nil {
		log.Printf("Failed to get sites: %v", err)
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Message: "Failed to get sites",
		})
		return
	}

	log.Printf("Sites retrieved: %d sites, Mode: %s", len(sites), viewMode)

	if len(sites) == 0 {
		c.JSON(http.StatusOK, models.DashboardData{
			Sites:          []*models.SiteWithReadings{},
			SystemStatus:   createEmptySystemStatus(),
			RecentActivity: []models.ActivityItem{},
			ViewMode:       viewMode,
		})
		return
	}

	// Step 3: Get readings with maximum parallel processing
	readingsStart := time.Now()
	var sitesWithReadings []*models.SiteWithReadings

	if viewMode == "realtime" && user.Role == "admin" {
		sitesWithReadings, err = h.getAggressiveParallelRealTimeReadings(sites)
	} else {
		sitesWithReadings, err = h.getAggressiveParallelDailyClosingReadings(sites)
	}

	if err != nil {
		log.Printf("Failed to get readings: %v", err)
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Message: "Failed to get readings",
		})
		return
	}

	log.Printf("Readings completed: %d sites with data (took %v)", len(sitesWithReadings), time.Since(readingsStart))

	// Sort by fuel level descending
	sort.Slice(sitesWithReadings, func(i, j int) bool {
		return sitesWithReadings[i].FuelLevelPercentage > sitesWithReadings[j].FuelLevelPercentage
	})

	// Calculate system status and recent activity
	systemStatus := calculateSystemStatus(sitesWithReadings, len(sites))
	recentActivity := generateRecentActivity(sitesWithReadings)

	totalTime := time.Since(startTime)
	log.Printf("DASHBOARD COMPLETE: User=%s, Mode=%s, Sites=%d/%d, Total=%v",
		user.Username, viewMode, len(sitesWithReadings), len(sites), totalTime)

	c.JSON(http.StatusOK, models.DashboardData{
		Sites:          sitesWithReadings,
		SystemStatus:   systemStatus,
		RecentActivity: recentActivity,
		ViewMode:       viewMode,
	})
}

// getAggressiveParallelRealTimeReadings uses maximum parallelism for real-time data
func (h *DashboardHandler) getAggressiveParallelRealTimeReadings(sites []*models.Site) ([]*models.SiteWithReadings, error) {
	start := time.Now()

	// Use more workers with smaller batches for maximum parallelism
	const maxWorkers = 15
	const batchSize = 4

	deviceChan := make(chan string, len(sites))
	resultChan := make(chan *models.SiteWithReadings, len(sites))

	// Start aggressive worker pool
	var wg sync.WaitGroup
	for i := 0; i < maxWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for deviceID := range deviceChan {
				// Get readings for single device (fastest possible)
				reading := h.DB.GetSingleDeviceReading(deviceID)
				if reading != nil && reading.FuelLevel != "" {
					// Find the site for this device
					var site *models.Site
					for _, s := range sites {
						if s.DeviceID == deviceID {
							site = s
							break
						}
					}
					if site != nil {
						siteWithReading := processSiteReading(site, reading)
						resultChan <- siteWithReading
					}
				}
			}
		}(i)
	}

	// Send all device IDs to workers
	go func() {
		defer close(deviceChan)
		for _, site := range sites {
			deviceChan <- site.DeviceID
		}
	}()

	// Collect results
	go func() {
		wg.Wait()
		close(resultChan)
	}()

	var sitesWithReadings []*models.SiteWithReadings
	for siteWithReading := range resultChan {
		sitesWithReadings = append(sitesWithReadings, siteWithReading)
	}

	log.Printf("Aggressive parallel real-time completed: %d sites (took %v)", len(sitesWithReadings), time.Since(start))
	return sitesWithReadings, nil
}

// getAggressiveParallelDailyClosingReadings uses maximum parallelism for daily closing
func (h *DashboardHandler) getAggressiveParallelDailyClosingReadings(sites []*models.Site) ([]*models.SiteWithReadings, error) {
	start := time.Now()

	const maxWorkers = 12

	siteChan := make(chan *models.Site, len(sites))
	resultChan := make(chan *models.SiteWithReadings, len(sites))

	// Start worker pool for sites
	var wg sync.WaitGroup
	for i := 0; i < maxWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for site := range siteChan {
				// Get daily closing for single site + live states
				reading := h.DB.GetSingleSiteDailyClosing(site.ID, site.DeviceID)
				if reading != nil && reading.FuelLevel != "" {
					siteWithReading := processSiteReading(site, reading)
					resultChan <- siteWithReading
				}
			}
		}(i)
	}

	// Send all sites to workers
	go func() {
		defer close(siteChan)
		for _, site := range sites {
			siteChan <- site
		}
	}()

	// Collect results
	go func() {
		wg.Wait()
		close(resultChan)
	}()

	var sitesWithReadings []*models.SiteWithReadings
	for siteWithReading := range resultChan {
		sitesWithReadings = append(sitesWithReadings, siteWithReading)
	}

	log.Printf("Aggressive parallel daily closing completed: %d sites (took %v)", len(sitesWithReadings), time.Since(start))
	return sitesWithReadings, nil
}

// processSiteReading processes a site with its sensor reading into SiteWithReadings
func processSiteReading(site *models.Site, reading *models.SensorReading) *models.SiteWithReadings {
	// Parse fuel level percentage
	fuelLevelPercentage := 0.0
	if reading.FuelLevel != "" {
		if level, err := strconv.ParseFloat(reading.FuelLevel, 64); err == nil {
			if level < 0 {
				level = 0
			} else if level > 100 {
				level = 100
			}
			fuelLevelPercentage = level
		}
	}

	// Determine power states
	generatorOnline := isStateOnline(reading.GeneratorState)
	zesaOnline := isStateOnline(reading.ZesaState)

	// Determine alert status
	alertStatus := "normal"
	if fuelLevelPercentage <= 25.0 {
		alertStatus = "low_fuel"
	} else if !generatorOnline && fuelLevelPercentage > 0 {
		alertStatus = "generator_off"
	}

	return &models.SiteWithReadings{
		Site:                site,
		LatestReading:       reading,
		GeneratorOnline:     generatorOnline,
		ZesaOnline:          zesaOnline,
		FuelLevelPercentage: fuelLevelPercentage,
		AlertStatus:         alertStatus,
	}
}

// isStateOnline checks if a state string represents "online" status
func isStateOnline(state string) bool {
	state = strings.ToLower(strings.TrimSpace(state))
	return state == "1" || state == "on" || state == "true" || state == "1.0"
}

// calculateSystemStatus calculates overall system status
func calculateSystemStatus(sitesWithReadings []*models.SiteWithReadings, totalSites int) models.SystemStatus {
	lowFuelCount := 0
	generatorsRunningCount := 0
	zesaRunningCount := 0

	for _, site := range sitesWithReadings {
		if site.AlertStatus == "low_fuel" {
			lowFuelCount++
		}
		if site.GeneratorOnline {
			generatorsRunningCount++
		}
		if site.ZesaOnline {
			zesaRunningCount++
		}
	}

	return models.SystemStatus{
		SitesOnline:       len(sitesWithReadings),
		TotalSites:        totalSites,
		LowFuelAlerts:     lowFuelCount,
		GeneratorsRunning: generatorsRunningCount,
		ZesaRunning:       zesaRunningCount,
		OfflineSites:      totalSites - len(sitesWithReadings),
	}
}

// generateRecentActivity creates recent activity items from sites
func generateRecentActivity(sitesWithReadings []*models.SiteWithReadings) []models.ActivityItem {
	var activities []models.ActivityItem

	count := 0
	for _, site := range sitesWithReadings {
		if site.LatestReading == nil || count >= 10 {
			break
		}

		event := "Normal Reading"
		status := "Normal"

		if site.AlertStatus == "low_fuel" {
			event = "Low Fuel Alert"
			status = "Low Fuel"
		} else if site.AlertStatus == "generator_off" {
			event = "Generator Offline"
			status = "Offline"
		}

		fuelVolume := "0"
		if site.LatestReading.FuelVolume != "" {
			fuelVolume = site.LatestReading.FuelVolume
		}

		activity := models.ActivityItem{
			ID:        count + 1,
			SiteID:    site.ID,
			SiteName:  site.Name,
			Event:     event,
			Value:     fmt.Sprintf("%.1f%% (%sL)", site.FuelLevelPercentage, fuelVolume),
			Timestamp: site.LatestReading.CapturedAt,
			Status:    status,
		}

		activities = append(activities, activity)
		count++
	}

	// Sort by timestamp descending (newest first)
	sort.Slice(activities, func(i, j int) bool {
		return activities[i].Timestamp.After(activities[j].Timestamp)
	})

	return activities
}

// createEmptySystemStatus creates an empty system status
func createEmptySystemStatus() models.SystemStatus {
	return models.SystemStatus{
		SitesOnline:       0,
		TotalSites:        0,
		LowFuelAlerts:     0,
		GeneratorsRunning: 0,
		ZesaRunning:       0,
		OfflineSites:      0,
	}
}
