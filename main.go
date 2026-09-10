package main

import (
	"os"

	"github.com/gin-gonic/gin"

	"crm-auth-service/conf"
	"crm-auth-service/controllers"
	"crm-auth-service/helpers"
	"crm-auth-service/middleware"
	"crm-auth-service/models"
	"crm-auth-service/repository"
	"crm-auth-service/routes"
	"crm-auth-service/services"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
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

	// Initialize GORM
	importGormStr := cfg.DB.DSN(cfg.DB.Name)
	gormDB, err := gorm.Open(postgres.Open(importGormStr), &gorm.Config{})
	if err != nil {
		log.Error("failed to connect gorm", "error", err)
		os.Exit(1)
	}

	// Migrate Lead, Commercial, and RBAC models
	gormDB.AutoMigrate(
		&models.Lead{}, &models.Activity{}, &models.LeadStage{},
		&models.CommercialEstimation{}, &models.CommercialResource{},
		&models.CommercialExpense{}, &models.SDLCAllocation{},
		&models.User{}, &models.LeaderDelegation{},
	)
	gormDB.Exec("CREATE SEQUENCE IF NOT EXISTS lead_id_seq START 1")

	// Dependency Injection: Repository data-access layer.
	userRepo := repository.NewUserRepository(db)
	sessionRepo := repository.NewSessionRepository(db)
	otpRepo := repository.NewUserEmailOTPRepository(db)
	emailOTPRepo := repository.NewEmailOTPRepository(db)
	mobileOTPRepo := repository.NewMobileOTPRepository(db)
	forgotPasswordRepo := repository.NewForgotPasswordRepository(db)
	oauthStateRepo := repository.NewOAuthStateRepository(db)

	// Dependency Injection: Utility helpers.
	jwtManager := helpers.NewJWTManager(cfg.JWT)
	emailService := helpers.NewEmailService(helpers.SMTPConfig{
		Host:     cfg.SMTP.Host,
		Port:     cfg.SMTP.Port,
		Username: cfg.SMTP.Username,
		Password: cfg.SMTP.Password,
	}, log)
	smsService := helpers.NewConsoleSMSService(log)
	rateLimiter := middleware.NewRateLimiter(cfg.Rate.MaxAttempts, cfg.Rate.Window)

	// Dependency Injection: Core business services layer.
	authService := services.NewAuthService(
		userRepo,
		sessionRepo,
		otpRepo,
		emailOTPRepo,
		mobileOTPRepo,
		forgotPasswordRepo,
		oauthStateRepo,
		jwtManager,
		emailService,
		smsService,
		rateLimiter,
		log,
	)

	// Dependency Injection: HTTP layer controller.
	authController := controllers.NewAuthController(authService, log)

	// Lead Module dependencies
	leadRepo := repository.NewLeadRepository(db, gormDB)
	leadService := services.NewLeadService(leadRepo, userRepo)
	leadController := controllers.NewLeadController(leadService, log)

	// Commercial Module dependencies
	commRepo := repository.NewCommercialRepository(db, gormDB)
	commService := services.NewCommercialService(commRepo, leadRepo, userRepo)
	commController := controllers.NewCommercialController(commService, log)

	// Analytics Module dependencies
	analyticsRepo := repository.NewAnalyticsRepository(db, gormDB)
	analyticsService := services.NewAnalyticsService(analyticsRepo, leadRepo, userRepo)
	analyticsController := controllers.NewAnalyticsController(analyticsService, log)

	// Configure routing engine.
	if cfg.Server.Env == "production" {
		gin.SetMode(gin.ReleaseMode)
	}
	router := gin.New()
	router.SetTrustedProxies(nil)
	router.Use(gin.Recovery())
	router.Use(middleware.CORSMiddleware())

	// Bind application endpoint route mappings.
	routes.RegisterRoutes(router, authController, leadController, commController, analyticsController, jwtManager, userRepo)

	log.Info("starting server", "port", cfg.Server.Port, "env", cfg.Server.Env)
	if err := router.Run(":" + cfg.Server.Port); err != nil {
		log.Error("server stopped", "error", err)
		os.Exit(1)
	}
}