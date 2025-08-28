package handlers

import (
	"net/http"
	"time"

	"fuel-monitor-api/internal/config"
	"fuel-monitor-api/internal/database"
	"fuel-monitor-api/internal/middleware"
	"fuel-monitor-api/internal/models"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

type AuthHandler struct {
	DB     *database.DB
	Config *config.Config
}

func NewAuthHandler(db *database.DB, cfg *config.Config) *AuthHandler {
	return &AuthHandler{
		DB:     db,
		Config: cfg,
	}
}

// Login handles user authentication
func (h *AuthHandler) Login(c *gin.Context) {
	var req models.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Message: "Invalid request format",
		})
		return
	}

	// Get user from database
	user, err := h.DB.GetUserByUsername(req.Username)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Message: "Database error",
		})
		return
	}

	if user == nil {
		c.JSON(http.StatusUnauthorized, models.ErrorResponse{
			Message: "Invalid credentials",
		})
		return
	}

	if !user.IsActive {
		c.JSON(http.StatusUnauthorized, models.ErrorResponse{
			Message: "Account is inactive",
		})
		return
	}

	// Verify password
	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.Password)); err != nil {
		c.JSON(http.StatusUnauthorized, models.ErrorResponse{
			Message: "Invalid credentials",
		})
		return
	}

	// Update last login
	now := time.Now()
	if err := h.DB.UpdateUserLastLogin(user.ID, now); err != nil {
		// Log error but don't fail the login
		// log.Printf("Failed to update last login for user %s: %v", user.Username, err)
	}

	// Generate JWT token
	token, err := h.generateToken(user)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Message: "Failed to generate token",
		})
		return
	}

	// Update user's last login for response
	user.LastLogin = &now

	c.JSON(http.StatusOK, models.LoginResponse{
		User:  user.ToResponse(),
		Token: token,
	})
}

// Logout handles user logout
func (h *AuthHandler) Logout(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"message": "Logged out successfully",
	})
}

// ValidateToken validates the JWT token and returns user info
func (h *AuthHandler) ValidateToken(c *gin.Context) {
	user, exists := middleware.GetUserFromContext(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, models.TokenValidationResponse{
			Valid:     false,
			Timestamp: time.Now().Format(time.RFC3339),
		})
		return
	}

	c.JSON(http.StatusOK, models.TokenValidationResponse{
		Valid:     true,
		User:      *user,
		Timestamp: time.Now().Format(time.RFC3339),
		TokenInfo: &models.TokenInfo{
			UserID:   user.ID,
			Username: user.Username,
			Role:     user.Role,
		},
	})
}

// generateToken creates a JWT token for the user
func (h *AuthHandler) generateToken(user *models.User) (string, error) {
	// Calculate expiration time (24 hours from now)
	expirationTime := time.Now().Add(24 * time.Hour)

	// Create claims
	claims := &middleware.Claims{
		ID:       user.ID,
		Username: user.Username,
		Role:     user.Role,
		Email:    user.Email,
		FullName: user.FullName,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expirationTime),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			NotBefore: jwt.NewNumericDate(time.Now()),
		},
	}

	// Create token
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	// Sign token with secret
	tokenString, err := token.SignedString([]byte(h.Config.JWT.Secret))
	if err != nil {
		return "", err
	}

	return tokenString, nil
}
