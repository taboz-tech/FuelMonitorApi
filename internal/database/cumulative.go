package database

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"fuel-monitor-api/internal/models"
)

// GetExistingCumulativeReadings gets existing cumulative readings for sites on a specific date
func (db *DB) GetExistingCumulativeReadings(date string, sites []*models.Site) ([]*models.CumulativeReading, error) {
	if len(sites) == 0 {
		return []*models.CumulativeReading{}, nil
	}

	// Build query with site IDs
	siteIDs := make([]interface{}, len(sites))
	placeholders := make([]string, len(sites))
	for i, site := range sites {
		siteIDs[i] = site.ID
		placeholders[i] = fmt.Sprintf("$%d", i+2) // +2 because $1 is for date
	}

	query := fmt.Sprintf(`
		SELECT id, site_id, device_id, date, total_fuel_consumed, total_fuel_topped_up, 
		       fuel_consumed_percent, fuel_topped_up_percent, total_generator_runtime, 
		       total_zesa_runtime, total_offline_time, calculated_at, created_at
		FROM cumulative_readings 
		WHERE date = $1 AND site_id IN (%s)
	`, strings.Join(placeholders, ", "))

	args := []interface{}{date}
	args = append(args, siteIDs...)

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to get existing cumulative readings: %w", err)
	}
	defer rows.Close()

	var readings []*models.CumulativeReading
	for rows.Next() {
		var reading models.CumulativeReading
		err := rows.Scan(
			&reading.ID,
			&reading.SiteID,
			&reading.DeviceID,
			&reading.Date,
			&reading.TotalFuelConsumed,
			&reading.TotalFuelTopped,
			&reading.FuelConsumedPercent,
			&reading.FuelToppedPercent,
			&reading.TotalGeneratorRuntime,
			&reading.TotalZesaRuntime,
			&reading.TotalOfflineTime,
			&reading.CalculatedAt,
			&reading.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan cumulative reading: %w", err)
		}
		readings = append(readings, &reading)
	}

	return readings, nil
}

// CreateOrUpdateCumulativeReading creates a new cumulative reading or updates existing one
func (db *DB) CreateOrUpdateCumulativeReading(siteID int, deviceID, date string, fuelMetrics models.FuelMetrics, powerMetrics models.PowerMetrics) (*models.CumulativeReading, error) {
	query := `
		INSERT INTO cumulative_readings (
			site_id, device_id, date, total_fuel_consumed, total_fuel_topped_up,
			fuel_consumed_percent, fuel_topped_up_percent, total_generator_runtime,
			total_zesa_runtime, total_offline_time, calculated_at, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		ON CONFLICT (site_id, date) 
		DO UPDATE SET 
			total_fuel_consumed = EXCLUDED.total_fuel_consumed,
			total_fuel_topped_up = EXCLUDED.total_fuel_topped_up,
			fuel_consumed_percent = EXCLUDED.fuel_consumed_percent,
			fuel_topped_up_percent = EXCLUDED.fuel_topped_up_percent,
			total_generator_runtime = EXCLUDED.total_generator_runtime,
			total_zesa_runtime = EXCLUDED.total_zesa_runtime,
			total_offline_time = EXCLUDED.total_offline_time,
			calculated_at = EXCLUDED.calculated_at
		RETURNING id, site_id, device_id, date, total_fuel_consumed, total_fuel_topped_up,
		          fuel_consumed_percent, fuel_topped_up_percent, total_generator_runtime,
		          total_zesa_runtime, total_offline_time, calculated_at, created_at
	`

	now := time.Now()
	var reading models.CumulativeReading

	err := db.QueryRow(
		query,
		siteID,
		deviceID,
		date,
		fmt.Sprintf("%.2f", fuelMetrics.TotalFuelConsumed),
		fmt.Sprintf("%.2f", fuelMetrics.TotalFuelTopped),
		fmt.Sprintf("%.2f", fuelMetrics.FuelConsumedPercent),
		fmt.Sprintf("%.2f", fuelMetrics.FuelToppedPercent),
		fmt.Sprintf("%.2f", powerMetrics.TotalGeneratorRuntime),
		fmt.Sprintf("%.2f", powerMetrics.TotalZesaRuntime),
		fmt.Sprintf("%.2f", powerMetrics.TotalOfflineTime),
		now,
		now,
	).Scan(
		&reading.ID,
		&reading.SiteID,
		&reading.DeviceID,
		&reading.Date,
		&reading.TotalFuelConsumed,
		&reading.TotalFuelTopped,
		&reading.FuelConsumedPercent,
		&reading.FuelToppedPercent,
		&reading.TotalGeneratorRuntime,
		&reading.TotalZesaRuntime,
		&reading.TotalOfflineTime,
		&reading.CalculatedAt,
		&reading.CreatedAt,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to create or update cumulative reading: %w", err)
	}

	return &reading, nil
}

// CalculateFuelChanges calculates fuel consumption and topping metrics for a device on a specific date
func (db *DB) CalculateFuelChanges(deviceID string, targetDate time.Time) (models.FuelMetrics, error) {
	// Ensure we capture the full day in UTC
	startOfDay := time.Date(targetDate.Year(), targetDate.Month(), targetDate.Day(), 0, 0, 0, 0, time.UTC)
	endOfDay := startOfDay.Add(24 * time.Hour).Add(-1 * time.Nanosecond)

	// Check if generator was running during the day
	hasGeneratorRuntime, err := db.hasGeneratorActivity(deviceID, startOfDay, endOfDay)
	if err != nil {
		return models.FuelMetrics{}, fmt.Errorf("failed to check generator activity: %w", err)
	}

	// Get ALL fuel readings for the day (both level and volume), ordered by time
	levelQuery := `
		SELECT value, time, sensor_name
		FROM sensor_readings 
		WHERE device_id = $1 
		  AND sensor_name IN ('fuel_sensor_level', 'fuel_sensor_volume')
		  AND time >= $2 AND time <= $3 
		  AND value IS NOT NULL
		ORDER BY time ASC
	`

	rows, err := db.Query(levelQuery, deviceID, startOfDay, endOfDay)
	if err != nil {
		return models.FuelMetrics{}, fmt.Errorf("failed to get fuel readings: %w", err)
	}
	defer rows.Close()

	var levelReadings []struct {
		Value float64
		Time  time.Time
	}
	var volumeReadings []struct {
		Value float64
		Time  time.Time
	}

	for rows.Next() {
		var valueStr, sensorName string
		var timestamp time.Time
		if err := rows.Scan(&valueStr, &timestamp, &sensorName); err != nil {
			continue
		}

		if value, err := strconv.ParseFloat(valueStr, 64); err == nil {
			reading := struct {
				Value float64
				Time  time.Time
			}{Value: value, Time: timestamp}

			if sensorName == "fuel_sensor_level" {
				levelReadings = append(levelReadings, reading)
			} else if sensorName == "fuel_sensor_volume" {
				volumeReadings = append(volumeReadings, reading)
			}
		}
	}

	// Calculate fuel level changes (percentage)
	var totalConsumedPercent, totalToppedPercent float64
	for i := 1; i < len(levelReadings); i++ {
		prev := levelReadings[i-1].Value
		curr := levelReadings[i].Value
		change := curr - prev

		// Skip small changes if no generator runtime
		changePercent := math.Abs(change)
		if !hasGeneratorRuntime && changePercent < 2.0 {
			continue
		}

		if change > 0 { // Increase = topping up
			totalToppedPercent += change
		} else if change < 0 { // Decrease = consumption
			totalConsumedPercent += -change // Make positive
		}
	}

	// Calculate fuel volume changes (liters)
	var totalConsumedVolume, totalToppedVolume float64
	for i := 1; i < len(volumeReadings); i++ {
		prev := volumeReadings[i-1].Value
		curr := volumeReadings[i].Value
		change := curr - prev

		// Skip small changes if no generator runtime
		// Convert to percentage for comparison (assuming typical tank capacity)
		if prev > 0 {
			changePercent := math.Abs(change) / prev * 100
			if !hasGeneratorRuntime && changePercent < 2.0 {
				continue
			}
		}

		if change > 0 { // Increase = topping up
			totalToppedVolume += change
		} else if change < 0 { // Decrease = consumption
			totalConsumedVolume += -change // Make positive
		}
	}

	return models.FuelMetrics{
		TotalFuelConsumed:   totalConsumedVolume,  // Volume consumed in liters
		TotalFuelTopped:     totalToppedVolume,    // Volume topped in liters
		FuelConsumedPercent: totalConsumedPercent, // Percentage consumed
		FuelToppedPercent:   totalToppedPercent,   // Percentage topped
	}, nil
}

// hasGeneratorActivity checks if the generator was running during the specified time period
func (db *DB) hasGeneratorActivity(deviceID string, startOfDay, endOfDay time.Time) (bool, error) {
	query := `
		SELECT COUNT(*) 
		FROM sensor_readings 
		WHERE device_id = $1 
		  AND sensor_name = 'generator_state'
		  AND time >= $2 AND time <= $3 
		  AND value IS NOT NULL
		  AND (value = '1' OR value = '1.0')
	`

	var count int
	err := db.QueryRow(query, deviceID, startOfDay, endOfDay).Scan(&count)
	if err != nil {
		return false, err
	}

	return count > 0, nil
}

// CalculatePowerRuntimes calculates generator and zesa runtime for a device on a specific date
func (db *DB) CalculatePowerRuntimes(deviceID string, targetDate time.Time) (models.PowerMetrics, error) {
	// Ensure we capture the full day in UTC
	startOfDay := time.Date(targetDate.Year(), targetDate.Month(), targetDate.Day(), 0, 0, 0, 0, time.UTC)
	endOfDay := startOfDay.Add(24 * time.Hour).Add(-1 * time.Nanosecond)

	// Calculate generator runtime
	generatorHours, err := db.calculateStateRuntime(deviceID, "generator_state", startOfDay, endOfDay)
	if err != nil {
		return models.PowerMetrics{}, fmt.Errorf("failed to calculate generator runtime: %w", err)
	}

	// Calculate zesa runtime
	zesaHours, err := db.calculateStateRuntime(deviceID, "zesa_state", startOfDay, endOfDay)
	if err != nil {
		return models.PowerMetrics{}, fmt.Errorf("failed to calculate zesa runtime: %w", err)
	}

	// Calculate offline time (24 hours - active time)
	// Note: generator and zesa can run simultaneously, so we don't simply add them
	totalActiveHours := generatorHours + zesaHours
	offlineHours := 0.0
	if totalActiveHours < 24 {
		offlineHours = 24 - totalActiveHours
	}

	return models.PowerMetrics{
		TotalGeneratorRuntime: generatorHours,
		TotalZesaRuntime:      zesaHours,
		TotalOfflineTime:      offlineHours,
	}, nil
}

// calculateStateRuntime calculates runtime for a specific state (helper method)
func (db *DB) calculateStateRuntime(deviceID, sensorName string, startOfDay, endOfDay time.Time) (float64, error) {
	query := `
		SELECT value, time 
		FROM sensor_readings 
		WHERE device_id = $1 
		  AND sensor_name = $2
		  AND time >= $3 AND time <= $4 
		  AND value IS NOT NULL
		ORDER BY time ASC
	`

	rows, err := db.Query(query, deviceID, sensorName, startOfDay, endOfDay)
	if err != nil {
		return 0, fmt.Errorf("failed to get state readings: %w", err)
	}
	defer rows.Close()

	var runtime float64
	var lastTime time.Time
	var lastState bool
	var hasData bool

	for rows.Next() {
		var valueStr string
		var timestamp time.Time
		if err := rows.Scan(&valueStr, &timestamp); err != nil {
			continue
		}

		// Parse state: 1=on, 0=off, anything else=off
		currentState := (valueStr == "1" || valueStr == "1.0")

		if hasData && lastState {
			// Add runtime for the period when state was ON
			duration := timestamp.Sub(lastTime)
			runtime += duration.Hours()
		}

		lastTime = timestamp
		lastState = currentState
		hasData = true
	}

	// Handle case where last state was ON and extends to end of day
	if hasData && lastState && lastTime.Before(endOfDay) {
		duration := endOfDay.Sub(lastTime)
		runtime += duration.Hours()
	}

	return runtime, nil
}
