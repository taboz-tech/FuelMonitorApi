package database

import (
	"database/sql"
	"fmt"
	"log"
	"strings"

	"fuel-monitor-api/internal/models"
)

// FastAutoCreateSites creates sites from distinct device_ids in sensor_readings
func (db *DB) FastAutoCreateSites() error {
	log.Println("🚀 FAST auto-creating sites from sensor_readings...")

	// Check if sensor_readings table exists
	tableExistsQuery := `
		SELECT EXISTS (
			SELECT FROM information_schema.tables 
			WHERE table_schema = 'public' AND table_name = 'sensor_readings'
		)
	`

	var tableExists bool
	err := db.QueryRow(tableExistsQuery).Scan(&tableExists)
	if err != nil {
		return fmt.Errorf("failed to check if sensor_readings table exists: %w", err)
	}

	if !tableExists {
		log.Println("⚠️ sensor_readings table not found")
		return nil
	}

	// Get distinct device_ids from sensor_readings
	distinctDevicesQuery := `
		SELECT DISTINCT device_id 
		FROM sensor_readings 
		WHERE device_id LIKE 'simbisa-%'
		ORDER BY device_id
	`

	rows, err := db.Query(distinctDevicesQuery)
	if err != nil {
		return fmt.Errorf("failed to get distinct devices: %w", err)
	}
	defer rows.Close()

	var deviceIds []string
	for rows.Next() {
		var deviceId string
		if err := rows.Scan(&deviceId); err != nil {
			return fmt.Errorf("failed to scan device_id: %w", err)
		}
		deviceIds = append(deviceIds, deviceId)
	}

	log.Printf("📊 Found %d distinct devices", len(deviceIds))

	if len(deviceIds) == 0 {
		log.Println("⚠️ No devices found in sensor_readings")
		return nil
	}

	createdCount := 0
	for _, deviceId := range deviceIds {
		// Check if site already exists
		existsQuery := `SELECT id FROM sites WHERE device_id = $1`
		var existingId int
		err := db.QueryRow(existsQuery, deviceId).Scan(&existingId)

		if err == nil {
			// Site already exists, skip
			continue
		}

		// Create new site
		siteName := deviceId                   // Keep exact: simbisa-avondale
		siteLocation := deviceId + " location" // simbisa-avondale location

		insertQuery := `
			INSERT INTO sites (name, location, device_id, is_active, created_at)
			VALUES ($1, $2, $3, $4, NOW())
		`

		_, err = db.Exec(insertQuery, siteName, siteLocation, deviceId, true)
		if err != nil {
			log.Printf("❌ Error creating site for %s: %v", deviceId, err)
			continue
		}

		log.Printf("✅ Created: %s (%s)", siteName, deviceId)
		createdCount++
	}

	if createdCount > 0 {
		log.Printf("🎉 FAST created %d sites from %d sensor devices", createdCount, len(deviceIds))
	} else {
		log.Println("ℹ️ All sensor devices already have sites")
	}

	return nil
}

// GetSiteByDeviceID retrieves a site by device ID
func (db *DB) GetSiteByDeviceID(deviceId string) (*models.Site, error) {
	query := `
		SELECT id, name, location, device_id, is_active, created_at
		FROM sites 
		WHERE device_id = $1
	`

	var site models.Site
	err := db.QueryRow(query, deviceId).Scan(
		&site.ID,
		&site.Name,
		&site.Location,
		&site.DeviceID,
		&site.IsActive,
		&site.CreatedAt,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil // Site not found
		}
		return nil, fmt.Errorf("failed to get site by device ID: %w", err)
	}

	return &site, nil
}

// GetAllSites retrieves all active sites
func (db *DB) GetAllSites() ([]*models.Site, error) {
	query := `
		SELECT id, name, location, device_id, is_active, created_at
		FROM sites 
		WHERE is_active = true
		ORDER BY name
	`

	rows, err := db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to get all sites: %w", err)
	}
	defer rows.Close()

	var sites []*models.Site
	for rows.Next() {
		var site models.Site
		err := rows.Scan(
			&site.ID,
			&site.Name,
			&site.Location,
			&site.DeviceID,
			&site.IsActive,
			&site.CreatedAt,
		)

		if err != nil {
			return nil, fmt.Errorf("failed to scan site: %w", err)
		}

		sites = append(sites, &site)
	}

	return sites, nil
}

// GetUserSiteAssignments retrieves site assignments for a user
func (db *DB) GetUserSiteAssignments(userID int) ([]*models.UserSiteAssignmentResponse, error) {
	query := `
		SELECT usa.site_id, s.name, s.location
		FROM user_site_assignments usa
		INNER JOIN sites s ON s.id = usa.site_id
		WHERE usa.user_id = $1 AND s.is_active = true
		ORDER BY s.name
	`

	rows, err := db.Query(query, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user site assignments: %w", err)
	}
	defer rows.Close()

	var assignments []*models.UserSiteAssignmentResponse
	for rows.Next() {
		var assignment models.UserSiteAssignmentResponse
		err := rows.Scan(
			&assignment.SiteID,
			&assignment.SiteName,
			&assignment.SiteLocation,
		)

		if err != nil {
			return nil, fmt.Errorf("failed to scan assignment: %w", err)
		}

		assignments = append(assignments, &assignment)
	}

	return assignments, nil
}

// GetSitesForUser retrieves sites visible to a user (all for admin, assigned for others)
func (db *DB) GetSitesForUser(userID int, userRole string) ([]*models.Site, error) {
	if userRole == "admin" {
		// Admin can see all active sites
		return db.GetAllSites()
	}

	// Manager/Supervisor can only see assigned sites
	query := `
		SELECT s.id, s.name, s.location, s.device_id, s.is_active, s.created_at
		FROM sites s
		INNER JOIN user_site_assignments usa ON usa.site_id = s.id
		WHERE usa.user_id = $1 AND s.is_active = true
		ORDER BY s.name
	`

	rows, err := db.Query(query, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user sites: %w", err)
	}
	defer rows.Close()

	var sites []*models.Site
	for rows.Next() {
		var site models.Site
		err := rows.Scan(
			&site.ID,
			&site.Name,
			&site.Location,
			&site.DeviceID,
			&site.IsActive,
			&site.CreatedAt,
		)

		if err != nil {
			return nil, fmt.Errorf("failed to scan site: %w", err)
		}

		sites = append(sites, &site)
	}

	return sites, nil
}

// AssignSitesToUser assigns sites to a user (replaces existing assignments)
func (db *DB) AssignSitesToUser(userID int, siteIDs []int) error {
	// Start transaction
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Delete existing assignments
	_, err = tx.Exec("DELETE FROM user_site_assignments WHERE user_id = $1", userID)
	if err != nil {
		return fmt.Errorf("failed to delete existing assignments: %w", err)
	}

	// Insert new assignments in batches
	if len(siteIDs) > 0 {
		batchSize := 100
		for i := 0; i < len(siteIDs); i += batchSize {
			end := i + batchSize
			if end > len(siteIDs) {
				end = len(siteIDs)
			}
			batch := siteIDs[i:end]

			// Build batch insert query
			values := make([]string, len(batch))
			args := make([]interface{}, 0, len(batch)*2)

			for j, siteID := range batch {
				values[j] = fmt.Sprintf("($%d, $%d, NOW())", j*2+1, j*2+2)
				args = append(args, userID, siteID)
			}

			query := fmt.Sprintf(
				"INSERT INTO user_site_assignments (user_id, site_id, created_at) VALUES %s",
				strings.Join(values, ", "),
			)

			_, err = tx.Exec(query, args...)
			if err != nil {
				return fmt.Errorf("failed to insert assignments: %w", err)
			}
		}
	}

	return tx.Commit()
}
