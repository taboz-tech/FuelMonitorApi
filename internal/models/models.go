package models

import (
	"time"
)

// User represents a user in the system
type User struct {
	ID        int        `json:"id"`
	Username  string     `json:"username"`
	Email     string     `json:"email"`
	Password  string     `json:"-"` // Never serialize password
	Role      string     `json:"role"`
	FullName  string     `json:"fullName"`
	IsActive  bool       `json:"isActive"`
	LastLogin *time.Time `json:"lastLogin"`
	CreatedAt time.Time  `json:"createdAt"`
}

// Site represents a site in the system
type Site struct {
	ID        int       `json:"id"`
	Name      string    `json:"name"`
	Location  string    `json:"location"`
	DeviceID  string    `json:"deviceId"`
	IsActive  bool      `json:"isActive"`
	CreatedAt time.Time `json:"createdAt"`
}

// UserSiteAssignment represents a user-site assignment in the system
type UserSiteAssignment struct {
	ID        int       `json:"id"`
	UserID    int       `json:"userId"`
	SiteID    int       `json:"siteId"`
	CreatedAt time.Time `json:"createdAt"`
}

// UserSiteAssignmentResponse represents assignment with site details
type UserSiteAssignmentResponse struct {
	SiteID       int    `json:"siteId"`
	SiteName     string `json:"siteName"`
	SiteLocation string `json:"siteLocation"`
}

// AssignSitesRequest represents request to assign sites to user
type AssignSitesRequest struct {
	SiteIds []int `json:"siteIds" binding:"required"`
}

// Dashboard models
type DashboardData struct {
	Sites          []*SiteWithReadings `json:"sites"`
	SystemStatus   SystemStatus        `json:"systemStatus"`
	RecentActivity []ActivityItem      `json:"recentActivity"`
	ViewMode       string              `json:"viewMode"`
}

type SiteWithReadings struct {
	*Site
	LatestReading       *SensorReading `json:"latestReading"`
	GeneratorOnline     bool           `json:"generatorOnline"`
	ZesaOnline          bool           `json:"zesaOnline"`
	FuelLevelPercentage float64        `json:"fuelLevelPercentage"`
	AlertStatus         string         `json:"alertStatus"` // "normal", "low_fuel", "generator_off"
}

type SensorReading struct {
	ID             int       `json:"id"`
	SiteID         int       `json:"siteId"`
	DeviceID       string    `json:"deviceId"`
	FuelLevel      string    `json:"fuelLevel"`
	FuelVolume     string    `json:"fuelVolume"`
	Temperature    *string   `json:"temperature"`
	GeneratorState string    `json:"generatorState"`
	ZesaState      string    `json:"zesaState"`
	CapturedAt     time.Time `json:"capturedAt"`
	CreatedAt      time.Time `json:"createdAt"`
}

type SystemStatus struct {
	SitesOnline       int `json:"sitesOnline"`
	TotalSites        int `json:"totalSites"`
	LowFuelAlerts     int `json:"lowFuelAlerts"`
	GeneratorsRunning int `json:"generatorsRunning"`
	ZesaRunning       int `json:"zesaRunning"`
	OfflineSites      int `json:"offlineSites"`
}

type ActivityItem struct {
	ID        int       `json:"id"`
	SiteID    int       `json:"siteId"`
	SiteName  string    `json:"siteName"`
	Event     string    `json:"event"`
	Value     string    `json:"value"`
	Timestamp time.Time `json:"timestamp"`
	Status    string    `json:"status"`
}

type AdminPreference struct {
	ID        int       `json:"id"`
	UserID    int       `json:"userId"`
	ViewMode  string    `json:"viewMode"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// UserResponse represents a user in API responses (without password)
type UserResponse struct {
	ID        int        `json:"id"`
	Username  string     `json:"username"`
	Email     string     `json:"email"`
	Role      string     `json:"role"`
	FullName  string     `json:"fullName"`
	IsActive  bool       `json:"isActive"`
	LastLogin *time.Time `json:"lastLogin"`
	CreatedAt time.Time  `json:"createdAt"`
}

// LoginRequest represents login request data
type LoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// LoginResponse represents login response data
type LoginResponse struct {
	User  UserResponse `json:"user"`
	Token string       `json:"token"`
}

// ErrorResponse represents error response data
type ErrorResponse struct {
	Message string `json:"message"`
}

// HealthResponse represents health check response
type HealthResponse struct {
	Status    string `json:"status"`
	Timestamp string `json:"timestamp"`
}

// TokenValidationResponse represents token validation response
type TokenValidationResponse struct {
	Valid     bool         `json:"valid"`
	User      UserResponse `json:"user,omitempty"`
	Timestamp string       `json:"timestamp"`
	TokenInfo *TokenInfo   `json:"tokenInfo,omitempty"`
}

// TokenInfo represents token information
type TokenInfo struct {
	UserID   int    `json:"userId"`
	Username string `json:"username"`
	Role     string `json:"role"`
}

// CreateUserRequest represents create user request data
type CreateUserRequest struct {
	Username string `json:"username" binding:"required"`
	Email    string `json:"email" binding:"required"`
	Password string `json:"password" binding:"required"`
	Role     string `json:"role" binding:"required"`
	FullName string `json:"fullName" binding:"required"`
	IsActive bool   `json:"isActive"`
}

// UpdateUserRequest represents update user request data
type UpdateUserRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Role     string `json:"role"`
	FullName string `json:"fullName"`
	IsActive bool   `json:"isActive"`
}

// CreateUserData represents data for creating a user in database
type CreateUserData struct {
	Username string
	Email    string
	Password string
	Role     string
	FullName string
	IsActive bool
}

// UpdateUserData represents data for updating a user in database
type UpdateUserData struct {
	Email    string
	Password string
	Role     string
	FullName string
	IsActive bool
}

// LiveStates represents live generator and zesa states
type LiveStates struct {
	GeneratorState string `json:"generatorState"`
	ZesaState      string `json:"zesaState"`
}

// ToResponse converts User to UserResponse
func (u *User) ToResponse() UserResponse {
	return UserResponse{
		ID:        u.ID,
		Username:  u.Username,
		Email:     u.Email,
		Role:      u.Role,
		FullName:  u.FullName,
		IsActive:  u.IsActive,
		LastLogin: u.LastLogin,
		CreatedAt: u.CreatedAt,
	}
}
