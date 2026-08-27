// Package conf owns environment configuration and database pool lifecycle.
package conf

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
)

// Config holds the full application settings.
type Config struct {
	Server ServerConfig
	DB     DBConfig
	JWT    JWTConfig
	Rate   RateLimitConfig
}

// ServerConfig defines server environment and execution port.
type ServerConfig struct {
	Port string
	Env  string
}

// DBConfig defines PostgreSQL database connectivity settings.
type DBConfig struct {
	Host     string
	Port     string
	User     string
	Password string
	Name     string
	SSLMode  string
}

// DSN returns a Postgres connection string.
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

// JWTConfig holds secret signing key and TTL parameters.
type JWTConfig struct {
	Secret             string
	AccessTokenTTL     time.Duration
	RefreshTokenTTL    time.Duration
	TempTokenTTL       time.Duration
	MFAPendingTokenTTL time.Duration
}

// RateLimitConfig holds attempt count limits and evaluation window size.
type RateLimitConfig struct {
	MaxAttempts int
	Window      time.Duration
}

// LoadConfig reads configuration settings from the environment or a .env file.
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

// ConnectDB establishes a connection pool to the target PostgreSQL database using pgxpool.
func ConnectDB(cfg DBConfig, log *slog.Logger) (*pgxpool.Pool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	log.Info(
		"database configuration",
		"host", cfg.Host,
		"port", cfg.Port,
		"user", cfg.User,
		"database", cfg.Name,
		"sslmode", cfg.SSLMode,
	)

	pool, err := pgxpool.New(ctx, cfg.DSN(cfg.Name))
	if err != nil {
		return nil, fmt.Errorf("conf: connect: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("conf: ping database: %w", err)
	}

	if _, err := pool.Exec(ctx, "CREATE EXTENSION IF NOT EXISTS pgcrypto"); err != nil {
		pool.Close()
		return nil, fmt.Errorf("conf: enable pgcrypto: %w", err)
	}

	if err := executeMigrations(ctx, pool); err != nil {
		pool.Close()
		return nil, fmt.Errorf("conf: migrations: %w", err)
	}

	log.Info("database ready", "database", cfg.Name)

	return pool, nil
}

func executeMigrations(ctx context.Context, pool *pgxpool.Pool) error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS users (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			email VARCHAR UNIQUE NOT NULL,
			mobile VARCHAR UNIQUE,
			password_hash VARCHAR NOT NULL,
			role VARCHAR NOT NULL,
			is_first_login BOOLEAN NOT NULL DEFAULT TRUE,
			email_verified BOOLEAN NOT NULL DEFAULT FALSE,
			mobile_verified BOOLEAN NOT NULL DEFAULT FALSE,
			mfa_enabled BOOLEAN NOT NULL DEFAULT FALSE,
			mfa_method VARCHAR,
			sso_provider VARCHAR,
			sso_subject_id VARCHAR,
			created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS refresh_tokens (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			user_id UUID NOT NULL,
			token_hash VARCHAR UNIQUE NOT NULL,
			revoked BOOLEAN NOT NULL DEFAULT FALSE,
			expires_at TIMESTAMP WITH TIME ZONE NOT NULL,
			created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS mfa_otps (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			user_id UUID NOT NULL,
			otp_hash VARCHAR NOT NULL,
			used BOOLEAN NOT NULL DEFAULT FALSE,
			expires_at TIMESTAMP WITH TIME ZONE NOT NULL,
			created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS email_otps (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			user_id UUID NOT NULL,
			otp_hash VARCHAR NOT NULL,
			used BOOLEAN NOT NULL DEFAULT FALSE,
			expires_at TIMESTAMP WITH TIME ZONE NOT NULL,
			created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS mobile_otps (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			user_id UUID NOT NULL,
			otp_hash VARCHAR NOT NULL,
			used BOOLEAN NOT NULL DEFAULT FALSE,
			expires_at TIMESTAMP WITH TIME ZONE NOT NULL,
			created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS password_reset_tokens (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			user_id UUID NOT NULL,
			token_hash VARCHAR UNIQUE NOT NULL,
			used BOOLEAN NOT NULL DEFAULT FALSE,
			expires_at TIMESTAMP WITH TIME ZONE NOT NULL,
			created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS oauth_states (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			state VARCHAR UNIQUE NOT NULL,
			provider VARCHAR NOT NULL,
			expires_at TIMESTAMP WITH TIME ZONE NOT NULL,
			created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_refresh_tokens_user_id ON refresh_tokens(user_id)`,
		`CREATE INDEX IF NOT EXISTS idx_mfa_otps_user_id ON mfa_otps(user_id)`,
		`CREATE INDEX IF NOT EXISTS idx_email_otps_user_id ON email_otps(user_id)`,
		`CREATE INDEX IF NOT EXISTS idx_mobile_otps_user_id ON mobile_otps(user_id)`,
		`CREATE INDEX IF NOT EXISTS idx_password_reset_tokens_user_id ON password_reset_tokens(user_id)`,
		`CREATE INDEX IF NOT EXISTS idx_oauth_states_state ON oauth_states(state)`,
		
		// Alter users to support Lead Module fields
		`ALTER TABLE users ADD COLUMN IF NOT EXISTS name VARCHAR NOT NULL DEFAULT ''`,
		`ALTER TABLE users ADD COLUMN IF NOT EXISTS is_active BOOLEAN NOT NULL DEFAULT TRUE`,

		// Lead Module Master Stages
		`CREATE TABLE IF NOT EXISTS lead_stages (
			id INT PRIMARY KEY,
			name VARCHAR NOT NULL,
			status VARCHAR NOT NULL DEFAULT '',
			sort_order INT NOT NULL,
			is_active BOOLEAN NOT NULL DEFAULT TRUE
		)`,
		`ALTER TABLE lead_stages ADD COLUMN IF NOT EXISTS status VARCHAR NOT NULL DEFAULT ''`,

		// Lead ID Sequence
		`CREATE SEQUENCE IF NOT EXISTS lead_id_seq START WITH 1`,

		// Leads Table
		`CREATE TABLE IF NOT EXISTS leads (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			lead_id VARCHAR UNIQUE NOT NULL,
			company VARCHAR NOT NULL,
			project_name VARCHAR,
			contact VARCHAR,
			email VARCHAR UNIQUE,
			phone VARCHAR,
			office_phone VARCHAR,
			office_phone_country VARCHAR,
			owner VARCHAR,
			industry VARCHAR,
			size VARCHAR,
			region VARCHAR,
			source VARCHAR,
			stage VARCHAR,
			status VARCHAR,
			sentiment VARCHAR,
			priority VARCHAR,
			value NUMERIC(15, 2),
			lost_reason VARCHAR,
			created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
			deleted_at TIMESTAMP WITH TIME ZONE
		)`,
		`ALTER TABLE leads ADD COLUMN IF NOT EXISTS lost_reason VARCHAR`,

		// Leads Constraints / Indexes
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_leads_company_lower ON leads (LOWER(company))`,
		`CREATE INDEX IF NOT EXISTS idx_leads_deleted_at ON leads (deleted_at)`,

		// Activities Table
		`CREATE TABLE IF NOT EXISTS activities (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			lead_id VARCHAR REFERENCES leads(lead_id) ON DELETE CASCADE,
			type VARCHAR,
			"desc" TEXT,
			outcome TEXT,
			due_date TIMESTAMP WITH TIME ZONE,
			completed BOOLEAN NOT NULL DEFAULT FALSE,
			created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
		)`,

		// Seed Lead Stages (8 stages)
		`INSERT INTO lead_stages (id, name, status, sort_order, is_active) VALUES
			(1, 'Prospecting', 'Open', 1, true),
			(2, 'Qualification', 'New', 2, true),
			(3, 'Initial Discussion', 'Contacted', 3, true),
			(4, 'Needs Analysis', 'Analysis', 4, true),
			(5, 'Proposal', 'Interested', 5, true),
			(6, 'Negotiation', 'Negotiation', 6, true),
			(7, 'Closed Won', 'Won', 7, true),
			(8, 'Closed Lost', 'Lost', 8, true)
			ON CONFLICT (id) DO UPDATE SET
				name = EXCLUDED.name,
				status = EXCLUDED.status,
				sort_order = EXCLUDED.sort_order,
				is_active = EXCLUDED.is_active`,
	}

	for _, q := range queries {
		if _, err := pool.Exec(ctx, q); err != nil {
			return err
		}
	}
	return nil
}
