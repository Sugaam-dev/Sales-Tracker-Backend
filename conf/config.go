// Package conf owns two things: reading environment configuration, and
// the database connection lifecycle (create-if-missing, connect,
// migrate). Nothing outside this package should call os.Getenv or open
// a *gorm.DB directly.
package conf

import (
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/joho/godotenv"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"crm-auth-service/models"
)

// ---------------------------------------------------------------------
// Config
// ---------------------------------------------------------------------

type Config struct {
	Server ServerConfig
	DB     DBConfig
	JWT    JWTConfig
	Rate   RateLimitConfig
}

type ServerConfig struct {
	Port string
	Env  string // "development" | "production"
}

type DBConfig struct {
	Host     string
	Port     string
	User     string
	Password string
	Name     string
	SSLMode  string
}

// DSN returns a Postgres connection string. Pass "" to connect to the
// "postgres" maintenance database (used only to create the target DB).
func (d DBConfig) DSN(dbName string) string {
	name := dbName
	if name == "" {
		name = "postgres"
	}
	return fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		d.Host, d.Port, d.User, d.Password, name, d.SSLMode,
	)
}

type JWTConfig struct {
	Secret             string
	AccessTokenTTL     time.Duration
	RefreshTokenTTL    time.Duration
	TempTokenTTL       time.Duration
	MFAPendingTokenTTL time.Duration
}

type RateLimitConfig struct {
	MaxAttempts int
	Window      time.Duration
}

// LoadConfig reads .env (missing file is fine — production sets real env
// vars) and returns a validated Config.
func LoadConfig() (*Config, error) {
	_ = godotenv.Load()

	cfg := &Config{
		Server: ServerConfig{
			Port: getEnv("SERVER_PORT", "8080"),
			Env:  getEnv("APP_ENV", "development"),
		},
		DB: DBConfig{
			Host:     getEnv("DB_HOST", "localhost"),
			Port:     getEnv("DB_PORT", "5432"),
			User:     getEnv("DB_USER", ""),
			Password: getEnv("DB_PASSWORD", ""),
			Name:     getEnv("DB_NAME", ""),
			SSLMode:  getEnv("DB_SSLMODE", "disable"),
		},
		JWT: JWTConfig{
			Secret:             getEnv("JWT_SECRET", ""),
			AccessTokenTTL:     15 * time.Minute,
			RefreshTokenTTL:    7 * 24 * time.Hour,
			TempTokenTTL:       15 * time.Minute,
			MFAPendingTokenTTL: 5 * time.Minute,
		},
		Rate: RateLimitConfig{
			MaxAttempts: 5,
			Window:      15 * time.Minute,
		},
	}

	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

func (c *Config) validate() error {
	if c.DB.User == "" || c.DB.Name == "" {
		return fmt.Errorf("config: DB_USER and DB_NAME are required")
	}
	if c.JWT.Secret == "" {
		return fmt.Errorf("config: JWT_SECRET is required")
	}
	if len(c.JWT.Secret) < 32 {
		return fmt.Errorf("config: JWT_SECRET must be at least 32 characters")
	}
	return nil
}

func getEnv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}

// ---------------------------------------------------------------------
// Database connection & migration
// ---------------------------------------------------------------------

// ConnectDB creates the target database if it doesn't exist, opens a
// GORM connection, enables pgcrypto (needed for gen_random_uuid()
// defaults), and runs AutoMigrate for Module A's tables only — users,
// refresh_tokens, mfa_otps. Future modules add their own migration step
// for tables they own; this function must never be extended beyond
// Module A's scope.
func ConnectDB(cfg DBConfig, log *slog.Logger) (*gorm.DB, error) {
	
	db, err := gorm.Open(postgres.Open(cfg.DSN(cfg.Name)), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Warn),
	})
	if err != nil {
		return nil, fmt.Errorf("conf: connect: %w", err)
	}

	if err := db.Exec(`CREATE EXTENSION IF NOT EXISTS pgcrypto`).Error; err != nil {
		return nil, fmt.Errorf("conf: enable pgcrypto: %w", err)
	}

	if err := db.AutoMigrate(
		&models.User{},
		&models.RefreshToken{},
		&models.MFAOtp{},
	); err != nil {
		return nil, fmt.Errorf("conf: automigrate: %w", err)
	}

	log.Info("database ready", "database", cfg.Name)
	return db, nil
}

