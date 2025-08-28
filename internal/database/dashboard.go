package database

import (
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"

	"fuel-monitor-api/internal/models"
)

// GetUserAdminPreference retrieves admin preference
func (db *DB) GetUserAdminPreference(userID int) (*models.AdminPreference, error) {
	query := `SELECT id, user_id, view_mode, updated_at FROM admin_preferences WHERE user_id = $1`

	var pref models.AdminPreference
	var updatedAt time.Time

	err := db.QueryRow(query, userID).Scan(&pref.ID, &pref.UserID, &pref.ViewMode, &updatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get admin preference: %w", err)
	}

	pref.UpdatedAt = updatedAt
	return &pref, nil
}

// GetDashboardSitesForUser - ultra fast without subqueries
func (db *DB) GetDashboardSitesForUser(userID int, userRole string) ([]*models.Site, error) {
	var query string
	var args []interface{}

	if userRole == "admin" {
		query = `
			SELECT id, name, location, device_id, is_active, created_at
			FROM sites 
			WHERE is_active = true AND device_id LIKE 'simbisa-%'
			ORDER BY name
		`
		args = []interface{}{}
	} else {
		query = `
			SELECT s.id, s.name, s.location, s.device_id, s.is_active, s.created_at
			FROM sites s 
			INNER JOIN user_site_assignments usa ON usa.site_id = s.id
			WHERE s.is_active = true 
			  AND s.device_id LIKE 'simbisa-%'
			  AND usa.user_id = $1
			ORDER BY s.name
		`
		args = []interface{}{userID}
	}

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to get sites: %w", err)
	}
	defer rows.Close()

	var sites []*models.Site
	for rows.Next() {
		var site models.Site
		var createdAt time.Time

		err := rows.Scan(&site.ID, &site.Name, &site.Location, &site.DeviceID, &site.IsActive, &createdAt)
		if err != nil {
			return nil, fmt.Errorf("failed to scan site: %w", err)
		}

		site.CreatedAt = createdAt
		sites = append(sites, &site)
	}

	return sites, nil
}

// GetSingleDeviceReading - optimized for single device using your index perfectly
func (db *DB) GetSingleDeviceReading(deviceID string) *models.SensorReading {
	// Single super-fast query per device using your idx_sensor_readings_device_time index
	query := `
		SELECT DISTINCT ON (sensor_name)
			sensor_name,
			value,
			time
		FROM sensor_readings 
		WHERE device_id = $1
		  AND sensor_name IN ('fuel_sensor_level', 'fuel_sensor_volume', 'fuel_sensor_temp', 'fuel_sensor_temperature', 'generator_state', 'zesa_state')
		  AND value IS NOT NULL
		ORDER BY sensor_name, time DESC
	`

	rows, err := db.Query(query, deviceID)
	if err != nil {
		return nil
	}
	defer rows.Close()

	reading := &models.SensorReading{
		DeviceID:       deviceID,
		FuelVolume:     "0.00",
		GeneratorState: "unknown",
		ZesaState:      "unknown",
	}

	hasFuelLevel := false
	var fuelTimestamp time.Time

	for rows.Next() {
		var sensorName, value string
		var timestamp time.Time

		if err := rows.Scan(&sensorName, &value, &timestamp); err != nil {
			continue
		}

		switch sensorName {
		case "fuel_sensor_level":
			reading.FuelLevel = value
			fuelTimestamp = timestamp
			hasFuelLevel = true
		case "fuel_sensor_volume":
			reading.FuelVolume = value
		case "fuel_sensor_temp", "fuel_sensor_temperature":
			reading.Temperature = &value
		case "generator_state":
			reading.GeneratorState = value
		case "zesa_state":
			reading.ZesaState = value
		}
	}

	if !hasFuelLevel {
		return nil
	}

	reading.CapturedAt = fuelTimestamp
	reading.CreatedAt = fuelTimestamp
	return reading
}

// GetSingleSiteDailyClosing - gets daily closing data + live states for one site
func (db *DB) GetSingleSiteDailyClosing(siteID int, deviceID string) *models.SensorReading {
	// Get daily closing fuel data using your idx_daily_closing_site_latest index
	dailyQuery := `
		SELECT fuel_level, fuel_volume, temperature, captured_at
		FROM daily_closing_readings
		WHERE site_id = $1 AND fuel_level IS NOT NULL 
		ORDER BY captured_at DESC
		LIMIT 1
	`

	var fuelLevel, fuelVolume, temperature sql.NullString
	var capturedAt time.Time

	err := db.QueryRow(dailyQuery, siteID).Scan(&fuelLevel, &fuelVolume, &temperature, &capturedAt)
	if err != nil {
		return nil
	}

	reading := &models.SensorReading{
		SiteID:         siteID,
		DeviceID:       deviceID,
		FuelVolume:     "0.00",
		GeneratorState: "unknown",
		ZesaState:      "unknown",
		CapturedAt:     capturedAt,
		CreatedAt:      capturedAt,
	}

	// Handle daily closing fields
	if fuelLevel.Valid {
		reading.FuelLevel = fuelLevel.String
	}
	if fuelVolume.Valid {
		reading.FuelVolume = fuelVolume.String
	}
	if temperature.Valid {
		reading.Temperature = &temperature.String
	}

	// Get live generator state
	generatorQuery := `
		SELECT value FROM sensor_readings 
		WHERE device_id = $1 AND sensor_name = 'generator_state' AND value IS NOT NULL
		ORDER BY time DESC LIMIT 1
	`
	var generatorState string
	if err := db.QueryRow(generatorQuery, deviceID).Scan(&generatorState); err == nil {
		reading.GeneratorState = generatorState
	}

	// Get live zesa state
	zesaQuery := `
		SELECT value FROM sensor_readings 
		WHERE device_id = $1 AND sensor_name = 'zesa_state' AND value IS NOT NULL
		ORDER BY time DESC LIMIT 1
	`
	var zesaState string
	if err := db.QueryRow(zesaQuery, deviceID).Scan(&zesaState); err == nil {
		reading.ZesaState = zesaState
	}

	return reading
}

// Legacy methods for compatibility
func (db *DB) GetBatchRealTimeReadings(deviceIDs []string) (map[string]*models.SensorReading, error) {
	result := make(map[string]*models.SensorReading)
	for _, deviceID := range deviceIDs {
		if reading := db.GetSingleDeviceReading(deviceID); reading != nil {
			result[deviceID] = reading
		}
	}
	return result, nil
}

func (db *DB) GetBatchDailyClosingReadings(siteIDs []int) (map[int]*models.SensorReading, error) {
	// This won't be used with the new parallel approach, but keeping for compatibility
	return make(map[int]*models.SensorReading), nil
}

// GetAllActiveSites gets sites that have sensor data
func (db *DB) GetAllActiveSites() ([]string, error) {
	query := `
		SELECT DISTINCT device_id 
		FROM sensor_readings 
		WHERE value IS NOT NULL
		ORDER BY device_id
	`

	rows, err := db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to get active sites: %w", err)
	}
	defer rows.Close()

	var sites []string
	for rows.Next() {
		var deviceID string
		if err := rows.Scan(&deviceID); err != nil {
			continue
		}
		sites = append(sites, deviceID)
	}

	return sites, nil
}

// GetLatestReadingForSite gets the absolute latest reading
func (db *DB) GetLatestReadingForSite(siteID, sensorName string) (*time.Time, *float64, error) {
	query := `
		SELECT time, value
		FROM sensor_readings
		WHERE device_id = $1 AND sensor_name = $2 AND value IS NOT NULL
		ORDER BY time DESC LIMIT 1
	`

	var timestamp time.Time
	var valueStr string

	err := db.QueryRow(query, siteID, sensorName).Scan(&timestamp, &valueStr)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil, nil
		}
		return nil, nil, fmt.Errorf("failed to get latest reading: %w", err)
	}

	value, err := strconv.ParseFloat(strings.TrimSpace(valueStr), 64)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse value '%s': %w", valueStr, err)
	}

	return &timestamp, &value, nil
}
