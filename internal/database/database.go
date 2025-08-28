package database

import (
	"database/sql"
	"fmt"
	"log"
	"strings"
	"time"

	"fuel-monitor-api/internal/config"
	"fuel-monitor-api/internal/models"

	_ "github.com/lib/pq"
)

type DB struct {
	*sql.DB
}

func Connect(cfg config.DatabaseConfig) (*DB, error) {
	dsn := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable",
		cfg.Host, cfg.Port, cfg.User, cfg.Password, cfg.Name)

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Configure connection pool
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)
	db.SetConnMaxIdleTime(1 * time.Minute)

	// Test the connection
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	log.Println("Database connection established")
	return &DB{db}, nil
}

// GetUserByUsername retrieves a user by username
func (db *DB) GetUserByUsername(username string) (*models.User, error) {
	query := `
		SELECT id, username, email, password, role, full_name, is_active, last_login, created_at
		FROM users 
		WHERE username = $1 AND is_active = true
	`

	var user models.User
	var lastLogin sql.NullTime

	err := db.QueryRow(query, username).Scan(
		&user.ID,
		&user.Username,
		&user.Email,
		&user.Password,
		&user.Role,
		&user.FullName,
		&user.IsActive,
		&lastLogin,
		&user.CreatedAt,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil // User not found
		}
		return nil, fmt.Errorf("failed to get user by username: %w", err)
	}

	// Handle nullable last_login
	if lastLogin.Valid {
		user.LastLogin = &lastLogin.Time
	}

	return &user, nil
}

// UpdateUserLastLogin updates the user's last login timestamp
func (db *DB) UpdateUserLastLogin(userID int, loginTime time.Time) error {
	query := `UPDATE users SET last_login = $1 WHERE id = $2`

	_, err := db.Exec(query, loginTime, userID)
	if err != nil {
		return fmt.Errorf("failed to update user last login: %w", err)
	}

	return nil
}

// GetUserByID retrieves a user by ID
func (db *DB) GetUserByID(id int) (*models.User, error) {
	query := `
		SELECT id, username, email, password, role, full_name, is_active, last_login, created_at
		FROM users 
		WHERE id = $1 AND is_active = true
	`

	var user models.User
	var lastLogin sql.NullTime

	err := db.QueryRow(query, id).Scan(
		&user.ID,
		&user.Username,
		&user.Email,
		&user.Password,
		&user.Role,
		&user.FullName,
		&user.IsActive,
		&lastLogin,
		&user.CreatedAt,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil // User not found
		}
		return nil, fmt.Errorf("failed to get user by ID: %w", err)
	}

	// Handle nullable last_login
	if lastLogin.Valid {
		user.LastLogin = &lastLogin.Time
	}

	return &user, nil
}

// GetAllUsers retrieves all active users
func (db *DB) GetAllUsers() ([]*models.User, error) {
	query := `
		SELECT id, username, email, password, role, full_name, is_active, last_login, created_at
		FROM users 
		WHERE is_active = true
		ORDER BY created_at
	`

	rows, err := db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to get all users: %w", err)
	}
	defer rows.Close()

	var users []*models.User
	for rows.Next() {
		var user models.User
		var lastLogin sql.NullTime

		err := rows.Scan(
			&user.ID,
			&user.Username,
			&user.Email,
			&user.Password,
			&user.Role,
			&user.FullName,
			&user.IsActive,
			&lastLogin,
			&user.CreatedAt,
		)

		if err != nil {
			return nil, fmt.Errorf("failed to scan user: %w", err)
		}

		if lastLogin.Valid {
			user.LastLogin = &lastLogin.Time
		}

		users = append(users, &user)
	}

	return users, nil
}

// GetUserByEmail retrieves a user by email
func (db *DB) GetUserByEmail(email string) (*models.User, error) {
	query := `
		SELECT id, username, email, password, role, full_name, is_active, last_login, created_at
		FROM users 
		WHERE email = $1 AND is_active = true
	`

	var user models.User
	var lastLogin sql.NullTime

	err := db.QueryRow(query, email).Scan(
		&user.ID,
		&user.Username,
		&user.Email,
		&user.Password,
		&user.Role,
		&user.FullName,
		&user.IsActive,
		&lastLogin,
		&user.CreatedAt,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil // User not found
		}
		return nil, fmt.Errorf("failed to get user by email: %w", err)
	}

	if lastLogin.Valid {
		user.LastLogin = &lastLogin.Time
	}

	return &user, nil
}

// CreateUser creates a new user
func (db *DB) CreateUser(userData *models.CreateUserData) (*models.User, error) {
	query := `
		INSERT INTO users (username, email, password, role, full_name, is_active)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, username, email, password, role, full_name, is_active, last_login, created_at
	`

	var user models.User
	var lastLogin sql.NullTime

	err := db.QueryRow(
		query,
		userData.Username,
		userData.Email,
		userData.Password,
		userData.Role,
		userData.FullName,
		userData.IsActive,
	).Scan(
		&user.ID,
		&user.Username,
		&user.Email,
		&user.Password,
		&user.Role,
		&user.FullName,
		&user.IsActive,
		&lastLogin,
		&user.CreatedAt,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	if lastLogin.Valid {
		user.LastLogin = &lastLogin.Time
	}

	return &user, nil
}

// UpdateUser updates an existing user
func (db *DB) UpdateUser(userID int, userData *models.UpdateUserData) (*models.User, error) {
	// Build dynamic query based on what fields are provided
	setParts := []string{}
	args := []interface{}{}
	argIndex := 1

	if userData.Email != "" {
		setParts = append(setParts, fmt.Sprintf("email = $%d", argIndex))
		args = append(args, userData.Email)
		argIndex++
	}

	if userData.Password != "" {
		setParts = append(setParts, fmt.Sprintf("password = $%d", argIndex))
		args = append(args, userData.Password)
		argIndex++
	}

	if userData.Role != "" {
		setParts = append(setParts, fmt.Sprintf("role = $%d", argIndex))
		args = append(args, userData.Role)
		argIndex++
	}

	if userData.FullName != "" {
		setParts = append(setParts, fmt.Sprintf("full_name = $%d", argIndex))
		args = append(args, userData.FullName)
		argIndex++
	}

	// Always update is_active (boolean can be false)
	setParts = append(setParts, fmt.Sprintf("is_active = $%d", argIndex))
	args = append(args, userData.IsActive)
	argIndex++

	if len(setParts) == 1 { // Only is_active was set
		return nil, fmt.Errorf("no fields to update")
	}

	// Add WHERE clause parameter
	args = append(args, userID)

	query := fmt.Sprintf(`
		UPDATE users 
		SET %s
		WHERE id = $%d
		RETURNING id, username, email, password, role, full_name, is_active, last_login, created_at
	`, strings.Join(setParts, ", "), argIndex)

	var user models.User
	var lastLogin sql.NullTime

	err := db.QueryRow(query, args...).Scan(
		&user.ID,
		&user.Username,
		&user.Email,
		&user.Password,
		&user.Role,
		&user.FullName,
		&user.IsActive,
		&lastLogin,
		&user.CreatedAt,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to update user: %w", err)
	}

	if lastLogin.Valid {
		user.LastLogin = &lastLogin.Time
	}

	return &user, nil
}

// DeleteUser deletes a user (soft delete by setting is_active to false)
func (db *DB) DeleteUser(userID int) error {
	// First delete related records
	queries := []string{
		"DELETE FROM user_site_assignments WHERE user_id = $1",
		"DELETE FROM admin_preferences WHERE user_id = $1",
		"UPDATE users SET is_active = false WHERE id = $1",
	}

	for _, query := range queries {
		_, err := db.Exec(query, userID)
		if err != nil {
			return fmt.Errorf("failed to delete user related data: %w", err)
		}
	}

	return nil
}
