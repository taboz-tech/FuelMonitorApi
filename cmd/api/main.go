package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"fuel-monitor-api/internal/config"
	"fuel-monitor-api/internal/database"
	"fuel-monitor-api/internal/handlers"
	"fuel-monitor-api/internal/middleware"
	"fuel-monitor-api/internal/ssh"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
)

func main() {
	// Load environment variables
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, using environment variables")
	}

	// Load configuration
	cfg := config.Load()

	// Setup SSH tunnel
	sshClient, localPort, err := ssh.SetupTunnel(cfg)
	if err != nil {
		log.Fatalf("Failed to setup SSH tunnel: %v", err)
	}
	defer sshClient.Close()

	// Update database config with local tunnel port
	cfg.Database.Host = "127.0.0.1"
	cfg.Database.Port = localPort

	// Connect to database
	db, err := database.Connect(cfg.Database)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	// Test database connection
	if err := db.Ping(); err != nil {
		log.Fatalf("Database ping failed: %v", err)
	}
	log.Println("Database connected successfully")

	// Fast auto-create sites from sensor_readings
	if err := db.FastAutoCreateSites(); err != nil {
		log.Printf("Warning: Failed to auto-create sites: %v", err)
	}

	// Setup Gin router
	router := setupRouter(cfg, db)

	// Create HTTP server
	server := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Server.Port),
		Handler:      router,
		ReadTimeout:  60 * time.Second,
		WriteTimeout: 0,
		IdleTimeout:  300 * time.Second,
	}

	// Start server in goroutine
	go func() {
		log.Printf("Server starting on port %d", cfg.Server.Port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server failed to start: %v", err)
		}
	}()

	// Wait for interrupt signal to gracefully shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("Shutting down server...")

	// Graceful shutdown with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}

	log.Println("Server exited")
}

func setupRouter(cfg *config.Config, db *database.DB) *gin.Engine {
	// Set Gin mode based on environment
	if cfg.Server.Environment == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	router := gin.New()

	// Add middleware
	router.Use(gin.Logger())
	router.Use(gin.Recovery())

	// CORS configuration
	corsConfig := cors.Config{
		AllowOrigins: []string{
			"http://localhost:4173",
			"http://154.119.80.28:4173",
			"http://127.0.0.1:4173",
		},
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Content-Type", "Authorization"},
		AllowCredentials: true,
	}
	router.Use(cors.New(corsConfig))

	// Initialize handlers
	authHandler := handlers.NewAuthHandler(db, cfg)
	userHandler := handlers.NewUserHandler(db)
	sitesHandler := handlers.NewSitesHandler(db)
	dashboardHandler := handlers.NewDashboardHandler(db)

	// Routes
	setupRoutes(router, authHandler, userHandler, sitesHandler, dashboardHandler)

	return router
}

func setupRoutes(router *gin.Engine, authHandler *handlers.AuthHandler, userHandler *handlers.UserHandler, sitesHandler *handlers.SitesHandler, dashboardHandler *handlers.DashboardHandler) {
	// Health check
	router.GET("/api/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":    "healthy",
			"timestamp": time.Now().Format(time.RFC3339),
		})
	})

	// Auth routes
	auth := router.Group("/api/auth")
	{
		auth.POST("/login", authHandler.Login)
		auth.POST("/logout", middleware.AuthRequired(authHandler.Config.JWT.Secret), authHandler.Logout)
		auth.GET("/validate", middleware.AuthRequired(authHandler.Config.JWT.Secret), authHandler.ValidateToken)
	}

	// Dashboard route (authenticated users)
	router.GET("/api/dashboard", middleware.AuthRequired(authHandler.Config.JWT.Secret), dashboardHandler.GetDashboard)

	// Sites routes (authenticated users)
	sites := router.Group("/api/sites")
	sites.Use(middleware.AuthRequired(authHandler.Config.JWT.Secret))
	{
		sites.GET("", sitesHandler.GetSites)
	}

	// User management routes (admin only)
	users := router.Group("/api/users")
	users.Use(middleware.AuthRequired(authHandler.Config.JWT.Secret))
	users.Use(middleware.RequireAdmin())
	{
		users.GET("", userHandler.GetUsers)
		users.GET("/:id", userHandler.GetUserByID)
		users.POST("", userHandler.CreateUser)
		users.PUT("/:id", userHandler.UpdateUser)
		users.DELETE("/:id", userHandler.DeleteUser)
	}

	// User-Site assignment routes (admin only) - different base path to avoid conflicts
	assignments := router.Group("/api/assignments")
	assignments.Use(middleware.AuthRequired(authHandler.Config.JWT.Secret))
	assignments.Use(middleware.RequireAdmin())
	{
		assignments.POST("/user/:userId/sites", sitesHandler.AssignSitesToUser)
		assignments.GET("/user/:userId/sites", sitesHandler.GetUserSiteAssignments)
	}
}
