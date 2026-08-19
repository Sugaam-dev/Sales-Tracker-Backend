package main

import (
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"

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
	cfg, err := conf.LoadConfig()
	if err != nil {
		// Logger isn't built yet — config failure is always fatal, so a
		// plain stderr write is correct here, not a slog call.
		os.Stderr.WriteString("config error: " + err.Error() + "\n")
		os.Exit(1)
	}

	log := newLogger(cfg.Server.Env)

	db, err := conf.ConnectDB(cfg.DB, log)
	if err != nil {
		log.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}
	sqlDB, err := db.DB()
	if err != nil {
		log.Error("failed to get underlying sql.DB", "error", err)
		os.Exit(1)
	}
	defer sqlDB.Close()

	// --- Dependency wiring ---------------------------------------------
	// Repositories (data access)
	userRepo := repository.NewUserRepository(db)
	sessionRepo := repository.NewSessionRepository(db)
	otpRepo := repository.NewUserEmailOTPRepository(db)
	oauthStateRepo := repository.NewOAuthStateRepository(db)

	// Cross-cutting helpers
	jwtManager := helpers.NewJWTManager(cfg.JWT)
	emailService := helpers.NewConsoleEmailService(log)
	smsService := helpers.NewConsoleSMSService(log)
	rateLimiter := middleware.NewRateLimiter(cfg.Rate.MaxAttempts, cfg.Rate.Window)

	// Business logic
	authService := services.NewAuthService(
		userRepo, sessionRepo, otpRepo, oauthStateRepo,
		jwtManager, emailService, smsService,
		rateLimiter, log,
	)

	// HTTP layer
	authController := controllers.NewAuthController(authService, log)

	
	if cfg.Server.Env == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	router := gin.New()

	// Set trusted proxies BEFORE starting the server
	router.SetTrustedProxies(nil)

	router.Use(gin.Recovery())


	// Register application routes
	routes.RegisterRoutes(router, authController, jwtManager)

	log.Info("starting server", "port", cfg.Server.Port, "env", cfg.Server.Env)

	// Attempt to bind to the configured port; if it's already in use, try the next few ports.
	basePort, err := strconv.Atoi(cfg.Server.Port)
	if err != nil {
		// Non-numeric port (e.g., named socket) — try to listen directly and fail with clearer message.
		addr := ":" + cfg.Server.Port
		ln, lerr := net.Listen("tcp", addr)
		if lerr != nil {
			log.Error("failed to bind to configured address", "addr", addr, "error", lerr)
			os.Exit(1)
		}
		if serr := http.Serve(ln, router); serr != nil {
			log.Error("server stopped", "error", serr)
			os.Exit(1)
		}
		return
	}

	var ln net.Listener
	found := false
	for i := 0; i <= 10; i++ {
		p := basePort + i
		addr := fmt.Sprintf(":%d", p)
		ln, err = net.Listen("tcp", addr)
		if err != nil {
			// If address in use, try next port. For other errors, fail fast.
			if strings.Contains(err.Error(), "Only one usage of each socket address") || strings.Contains(strings.ToLower(err.Error()), "address already in use") {
				continue
			}
			log.Error("failed to bind", "addr", addr, "error", err)
			os.Exit(1)
		}
		// success
		log.Info("bound and serving", "addr", addr)
		found = true
		break
	}
	if !found {
		log.Error("could not bind to any available port in range", "base_port", basePort)
		os.Exit(1)
	}
	if serr := http.Serve(ln, router); serr != nil {
		log.Error("server stopped", "error", serr)
		os.Exit(1)
	}
}

// newLogger builds the structured logger: JSON in production, text in
// development.
func newLogger(env string) *slog.Logger {
	opts := &slog.HandlerOptions{Level: slog.LevelInfo}
	if env == "development" {
		opts.Level = slog.LevelDebug 
	}
	var handler slog.Handler
	if env == "production" {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	} else {
		handler = slog.NewTextHandler(os.Stdout, opts)
	}
	return slog.New(handler)
}