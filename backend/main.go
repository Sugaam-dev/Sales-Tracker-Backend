package main

import (
	"os"

	"github.com/gin-gonic/gin"

	"crm-auth-service/conf"
	"crm-auth-service/controllers"
	"crm-auth-service/helpers"
	"crm-auth-service/middleware"
	"crm-auth-service/repository"
	"crm-auth-service/routes"
	"crm-auth-service/services"
)

func main() {
	// Load configuration settings from environment or .env file.
	cfg, err := conf.LoadConfig()
	if err != nil {
		os.Stderr.WriteString("config error: " + err.Error() + "\n")
		os.Exit(1)
	}

	// Initialize structured logging system.
	log := helpers.NewLogger(cfg.Server.Env)

	// Connect to database and perform migrations.
	db, err := conf.ConnectDB(cfg.DB, log)
	if err != nil {
		log.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	// Dependency Injection: Repository data-access layer.
	userRepo := repository.NewUserRepository(db)
	sessionRepo := repository.NewSessionRepository(db)
	otpRepo := repository.NewUserEmailOTPRepository(db)

	// Dependency Injection: Utility helpers.
	jwtManager := helpers.NewJWTManager(cfg.JWT)
	emailService := helpers.NewConsoleEmailService(log)
	smsService := helpers.NewConsoleSMSService(log)
	rateLimiter := middleware.NewRateLimiter(cfg.Rate.MaxAttempts, cfg.Rate.Window)

	// Dependency Injection: Core business services layer.
	authService := services.NewAuthService(
		userRepo,
		sessionRepo,
		otpRepo,
		jwtManager,
		emailService,
		smsService,
		rateLimiter,
		log,
	)

	// Dependency Injection: HTTP layer controller.
	authController := controllers.NewAuthController(authService, log)

	// Configure routing engine.
	if cfg.Server.Env == "production" {
		gin.SetMode(gin.ReleaseMode)
	}
	router := gin.New()
	router.SetTrustedProxies(nil)
	router.Use(gin.Recovery())
	router.Use(middleware.CORSMiddleware())

	// Bind application endpoint route mappings.
	routes.RegisterRoutes(router, authController)

	log.Info("starting server", "port", cfg.Server.Port, "env", cfg.Server.Env)
	if err := router.Run(":" + cfg.Server.Port); err != nil {
		log.Error("server stopped", "error", err)
		os.Exit(1)
	}
}