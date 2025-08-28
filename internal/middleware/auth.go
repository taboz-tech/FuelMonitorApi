package middleware

import (
	"fmt"
	"net/http"
	"strings"

	"fuel-monitor-api/internal/models"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

// Claims represents JWT claims
type Claims struct {
	ID       int    `json:"id"`
	Username string `json:"username"`
	Role     string `json:"role"`
	Email    string `json:"email"`
	FullName string `json:"fullName"`
	jwt.RegisteredClaims
}

// AuthRequired middleware validates JWT token
func AuthRequired(jwtSecret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, models.ErrorResponse{
				Message: "Access token required",
			})
			c.Abort()
			return
		}

		// Extract token from "Bearer <token>"
		tokenString := strings.TrimPrefix(authHeader, "Bearer ")
		if tokenString == authHeader {
			c.JSON(http.StatusUnauthorized, models.ErrorResponse{
				Message: "Invalid authorization format",
			})
			c.Abort()
			return
		}

		// Parse and validate token
		token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
			// Verify signing method
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
			}
			return []byte(jwtSecret), nil
		})

		if err != nil || !token.Valid {
			c.JSON(http.StatusForbidden, models.ErrorResponse{
				Message: "Invalid or expired token",
			})
			c.Abort()
			return
		}

		// Extract claims
		claims, ok := token.Claims.(*Claims)
		if !ok {
			c.JSON(http.StatusForbidden, models.ErrorResponse{
				Message: "Invalid token claims",
			})
			c.Abort()
			return
		}

		// Store user information in context
		c.Set("user", models.UserResponse{
			ID:       claims.ID,
			Username: claims.Username,
			Email:    claims.Email,
			Role:     claims.Role,
			FullName: claims.FullName,
			IsActive: true,
		})

		c.Next()
	}
}

// RequireRole middleware checks if user has required role
func RequireRole(roles ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		user, exists := c.Get("user")
		if !exists {
			c.JSON(http.StatusUnauthorized, models.ErrorResponse{
				Message: "Authentication required",
			})
			c.Abort()
			return
		}

		userInfo, ok := user.(models.UserResponse)
		if !ok {
			c.JSON(http.StatusInternalServerError, models.ErrorResponse{
				Message: "Invalid user context",
			})
			c.Abort()
			return
		}

		// Check if user has any of the required roles
		hasRole := false
		for _, role := range roles {
			if userInfo.Role == role {
				hasRole = true
				break
			}
		}

		if !hasRole {
			c.JSON(http.StatusForbidden, models.ErrorResponse{
				Message: "Insufficient permissions",
			})
			c.Abort()
			return
		}

		c.Next()
	}
}

// RequireAdmin middleware checks if user is admin
func RequireAdmin() gin.HandlerFunc {
	return RequireRole("admin")
}

// GetUserFromContext extracts user from gin context
func GetUserFromContext(c *gin.Context) (*models.UserResponse, bool) {
	user, exists := c.Get("user")
	if !exists {
		return nil, false
	}

	userInfo, ok := user.(models.UserResponse)
	return &userInfo, ok
}

// GetUserIDFromContext extracts user ID from gin context
func GetUserIDFromContext(c *gin.Context) (int, bool) {
	user, ok := GetUserFromContext(c)
	if !ok {
		return 0, false
	}
	return user.ID, true
}
