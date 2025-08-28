package handlers

import (
	"net/http"
	"strconv"
	"strings"

	"fuel-monitor-api/internal/database"
	"fuel-monitor-api/internal/middleware"
	"fuel-monitor-api/internal/models"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
)

type UserHandler struct {
	DB *database.DB
}

func NewUserHandler(db *database.DB) *UserHandler {
	return &UserHandler{
		DB: db,
	}
}

// GetUsers retrieves all active users (admin only)
func (h *UserHandler) GetUsers(c *gin.Context) {
	users, err := h.DB.GetAllUsers()
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Message: "Internal server error",
		})
		return
	}

	// Convert to response format
	userResponses := make([]models.UserResponse, len(users))
	for i, user := range users {
		userResponses[i] = user.ToResponse()
	}

	c.JSON(http.StatusOK, userResponses)
}

// GetUserByID retrieves a user by ID (admin only)
func (h *UserHandler) GetUserByID(c *gin.Context) {
	userIDParam := c.Param("id")
	userID, err := strconv.Atoi(userIDParam)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Message: "Invalid user ID",
		})
		return
	}

	user, err := h.DB.GetUserByID(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Message: "Internal server error",
		})
		return
	}

	if user == nil {
		c.JSON(http.StatusNotFound, models.ErrorResponse{
			Message: "User not found",
		})
		return
	}

	c.JSON(http.StatusOK, user.ToResponse())
}

// CreateUser creates a new user (admin only)
func (h *UserHandler) CreateUser(c *gin.Context) {
	var req models.CreateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Message: "Invalid data provided",
		})
		return
	}

	// Validate required fields
	if strings.TrimSpace(req.Username) == "" {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Message: "Username is required",
		})
		return
	}

	if strings.TrimSpace(req.Email) == "" {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Message: "Email is required",
		})
		return
	}

	if len(req.Password) < 6 {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Message: "Password must be at least 6 characters",
		})
		return
	}

	// Check if username already exists
	existingUser, err := h.DB.GetUserByUsername(req.Username)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Message: "Database error",
		})
		return
	}

	if existingUser != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Message: "Username already exists",
		})
		return
	}

	// Check if email already exists
	existingEmail, err := h.DB.GetUserByEmail(req.Email)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Message: "Database error",
		})
		return
	}

	if existingEmail != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Message: "Email already exists",
		})
		return
	}

	// Hash password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Message: "Failed to process password",
		})
		return
	}

	// Create user
	user, err := h.DB.CreateUser(&models.CreateUserData{
		Username: req.Username,
		Email:    req.Email,
		Password: string(hashedPassword),
		Role:     req.Role,
		FullName: req.FullName,
		IsActive: req.IsActive,
	})

	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Message: "Failed to create user",
		})
		return
	}

	c.JSON(http.StatusCreated, user.ToResponse())
}

// UpdateUser updates an existing user (admin only)
func (h *UserHandler) UpdateUser(c *gin.Context) {
	userIDParam := c.Param("id")
	userID, err := strconv.Atoi(userIDParam)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Message: "Invalid user ID",
		})
		return
	}

	var req models.UpdateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Message: "Invalid data provided",
		})
		return
	}

	// Check if user exists
	existingUser, err := h.DB.GetUserByID(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Message: "Database error",
		})
		return
	}

	if existingUser == nil {
		c.JSON(http.StatusNotFound, models.ErrorResponse{
			Message: "User not found",
		})
		return
	}

	// Check if email already exists for another user
	if req.Email != "" && req.Email != existingUser.Email {
		existingEmail, err := h.DB.GetUserByEmail(req.Email)
		if err != nil {
			c.JSON(http.StatusInternalServerError, models.ErrorResponse{
				Message: "Database error",
			})
			return
		}

		if existingEmail != nil && existingEmail.ID != userID {
			c.JSON(http.StatusBadRequest, models.ErrorResponse{
				Message: "Email already exists",
			})
			return
		}
	}

	// Prepare update data
	updateData := &models.UpdateUserData{
		Email:    req.Email,
		Role:     req.Role,
		FullName: req.FullName,
		IsActive: req.IsActive,
	}

	// Hash password if provided
	if req.Password != "" {
		if len(req.Password) < 6 {
			c.JSON(http.StatusBadRequest, models.ErrorResponse{
				Message: "Password must be at least 6 characters",
			})
			return
		}

		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
		if err != nil {
			c.JSON(http.StatusInternalServerError, models.ErrorResponse{
				Message: "Failed to process password",
			})
			return
		}
		updateData.Password = string(hashedPassword)
	}

	// Update user
	user, err := h.DB.UpdateUser(userID, updateData)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Message: "Failed to update user",
		})
		return
	}

	c.JSON(http.StatusOK, user.ToResponse())
}

// DeleteUser deletes a user (admin only)
func (h *UserHandler) DeleteUser(c *gin.Context) {
	userIDParam := c.Param("id")
	userID, err := strconv.Atoi(userIDParam)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Message: "Invalid user ID",
		})
		return
	}

	// Get current user from context
	currentUser, exists := middleware.GetUserFromContext(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, models.ErrorResponse{
			Message: "Authentication required",
		})
		return
	}

	// Prevent self-deletion
	if userID == currentUser.ID {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Message: "Cannot delete your own account",
		})
		return
	}

	// Check if user exists
	existingUser, err := h.DB.GetUserByID(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Message: "Database error",
		})
		return
	}

	if existingUser == nil {
		c.JSON(http.StatusNotFound, models.ErrorResponse{
			Message: "User not found",
		})
		return
	}

	// Delete user
	err = h.DB.DeleteUser(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Message: "Failed to delete user",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "User deleted successfully",
	})
}
