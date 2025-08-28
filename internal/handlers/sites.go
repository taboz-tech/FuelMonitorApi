package handlers

import (
	"net/http"
	"strconv"

	"fuel-monitor-api/internal/database"
	"fuel-monitor-api/internal/middleware"
	"fuel-monitor-api/internal/models"

	"github.com/gin-gonic/gin"
)

type SitesHandler struct {
	DB *database.DB
}

func NewSitesHandler(db *database.DB) *SitesHandler {
	return &SitesHandler{
		DB: db,
	}
}

// GetSites retrieves sites based on user role and permissions
func (h *SitesHandler) GetSites(c *gin.Context) {
	user, exists := middleware.GetUserFromContext(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, models.ErrorResponse{
			Message: "Authentication required",
		})
		return
	}

	sites, err := h.DB.GetSitesForUser(user.ID, user.Role)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Message: "Internal server error",
		})
		return
	}

	c.JSON(http.StatusOK, sites)
}

// AssignSitesToUser assigns sites to a specific user (admin only)
func (h *SitesHandler) AssignSitesToUser(c *gin.Context) {
	userIDParam := c.Param("userId")
	userID, err := strconv.Atoi(userIDParam)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Message: "Invalid user ID",
		})
		return
	}

	var req models.AssignSitesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Message: "Invalid site IDs",
		})
		return
	}

	// Validate that user exists
	user, err := h.DB.GetUserByID(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Message: "Database error",
		})
		return
	}

	if user == nil {
		c.JSON(http.StatusNotFound, models.ErrorResponse{
			Message: "User not found",
		})
		return
	}

	// Assign sites to user
	err = h.DB.AssignSitesToUser(userID, req.SiteIds)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Message: "Failed to update site assignments",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Site assignments updated successfully",
	})
}

// GetUserSiteAssignments retrieves site assignments for a specific user (admin only)
func (h *SitesHandler) GetUserSiteAssignments(c *gin.Context) {
	userIDParam := c.Param("userId")
	userID, err := strconv.Atoi(userIDParam)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Message: "Invalid user ID",
		})
		return
	}

	assignments, err := h.DB.GetUserSiteAssignments(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Message: "Internal server error",
		})
		return
	}

	c.JSON(http.StatusOK, assignments)
}
