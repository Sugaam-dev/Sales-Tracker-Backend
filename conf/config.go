// Package conf owns environment configuration and database pool lifecycle.
package conf

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
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
	SMTP   SMTPConfig
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

// SMTPConfig holds SMTP server credentials for email delivery.
type SMTPConfig struct {
	Host     string
	Port     string
	Username string
	Password string
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
		SMTP: SMTPConfig{
			Host:     getEnv("SMTP_HOST", ""),
			Port:     getEnv("SMTP_PORT", "587"),
			Username: getEnv("SMTP_USERNAME", ""),
			Password: getEnv("SMTP_PASSWORD", ""),
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
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
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
		// Alter users to support Lead Module fields and temporary password
		`ALTER TABLE users ADD COLUMN IF NOT EXISTS name VARCHAR NOT NULL DEFAULT ''`,
		`ALTER TABLE users ADD COLUMN IF NOT EXISTS is_active BOOLEAN NOT NULL DEFAULT TRUE`,
		`ALTER TABLE users ADD COLUMN IF NOT EXISTS password_change_required BOOLEAN NOT NULL DEFAULT TRUE`,
		`ALTER TABLE users ADD COLUMN IF NOT EXISTS manager_id UUID REFERENCES users(id) ON DELETE SET NULL`,
		`CREATE INDEX IF NOT EXISTS idx_users_manager_id ON users(manager_id)`,

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
			phone_country VARCHAR,
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
			request_details TEXT,
			request_type VARCHAR,
			created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
			deleted_at TIMESTAMP WITH TIME ZONE
		)`,
		`ALTER TABLE leads ADD COLUMN IF NOT EXISTS lost_reason VARCHAR`,
		`ALTER TABLE leads DROP COLUMN IF EXISTS product_service`,
		`ALTER TABLE leads ADD COLUMN IF NOT EXISTS request_details TEXT`,
		`ALTER TABLE leads ADD COLUMN IF NOT EXISTS request_type VARCHAR`,
		`ALTER TABLE leads ADD COLUMN IF NOT EXISTS phone_country VARCHAR`,

		// Leads Constraints / Indexes
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_leads_company_lower ON leads (LOWER(company))`,
		`CREATE INDEX IF NOT EXISTS idx_leads_deleted_at ON leads (deleted_at)`,

		// Drop activities table if id column is UUID type
		`DO $$
		BEGIN
			IF EXISTS (
				SELECT 1 
				FROM information_schema.columns 
				WHERE table_name = 'activities' AND column_name = 'id' AND data_type = 'uuid'
			) THEN
				DROP TABLE activities CASCADE;
			END IF;
		END $$;`,

		// Activities Table
		`CREATE TABLE IF NOT EXISTS activities (
			id SERIAL PRIMARY KEY,
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
		// Additional Lead Columns
		`ALTER TABLE leads ADD COLUMN IF NOT EXISTS best_time VARCHAR`,
		`ALTER TABLE leads ADD COLUMN IF NOT EXISTS lifecycle_template VARCHAR`,
		`ALTER TABLE leads ADD COLUMN IF NOT EXISTS kam_name VARCHAR`,
		`ALTER TABLE leads ADD COLUMN IF NOT EXISTS designation VARCHAR`,
		`ALTER TABLE leads ADD COLUMN IF NOT EXISTS best_time_to_connect VARCHAR`,
		`ALTER TABLE leads ADD COLUMN IF NOT EXISTS alternate_phone VARCHAR`,
		`ALTER TABLE leads ADD COLUMN IF NOT EXISTS alternate_phone_country VARCHAR`,
		`ALTER TABLE leads ADD COLUMN IF NOT EXISTS linkedin_profile_url VARCHAR`,
		`ALTER TABLE leads ADD COLUMN IF NOT EXISTS linkedin_company_page_url VARCHAR`,
		`ALTER TABLE leads ADD COLUMN IF NOT EXISTS estimated_requirement_date DATE`,
		`ALTER TABLE leads ADD COLUMN IF NOT EXISTS last_contact_date TIMESTAMP WITH TIME ZONE`,
		`ALTER TABLE leads ADD COLUMN IF NOT EXISTS next_follow_up TIMESTAMP WITH TIME ZONE`,
		`ALTER TABLE leads ADD COLUMN IF NOT EXISTS basic_requirements TEXT`,
		`ALTER TABLE leads ADD COLUMN IF NOT EXISTS notes TEXT`,

		// 000009_expand_activities_table
		`ALTER TABLE activities ADD COLUMN IF NOT EXISTS rep UUID REFERENCES users(id) ON DELETE SET NULL`,
		`CREATE INDEX IF NOT EXISTS idx_activities_type ON activities(type)`,
		`CREATE INDEX IF NOT EXISTS idx_activities_created_at ON activities(created_at)`,
		`CREATE INDEX IF NOT EXISTS idx_activities_rep ON activities(rep)`,

		// Commercial Estimations
		`CREATE TABLE IF NOT EXISTS commercial_estimations (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			lead_id VARCHAR UNIQUE NOT NULL REFERENCES leads(lead_id) ON DELETE CASCADE,
			currency VARCHAR(3) NOT NULL DEFAULT 'USD',
			billing_type VARCHAR(50) NOT NULL DEFAULT 'T&M',
			start_date DATE NOT NULL DEFAULT CURRENT_DATE,
			estimated_duration_months INT NOT NULL DEFAULT 1,
			estimated_end_date DATE NOT NULL DEFAULT (CURRENT_DATE + INTERVAL '1 month'),
			markup_percent NUMERIC(5,2) NOT NULL DEFAULT 0.00,
			discount_percent NUMERIC(5,2) NOT NULL DEFAULT 0.00,
			manual_selling_price NUMERIC(15,2),
			status VARCHAR(30) NOT NULL DEFAULT 'DRAFT',
			created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_comm_est_lead ON commercial_estimations(lead_id)`,

		// Commercial Resources
		`CREATE TABLE IF NOT EXISTS commercial_resources (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			commercial_estimation_id UUID NOT NULL REFERENCES commercial_estimations(id) ON DELETE CASCADE,
			role VARCHAR(100) NOT NULL,
			grade VARCHAR(50) NOT NULL,
			onsite_days INT NOT NULL DEFAULT 0,
			offshore_days INT NOT NULL DEFAULT 0,
			daily_cost NUMERIC(15,2) NOT NULL DEFAULT 0.00,
			billing_rate NUMERIC(15,2) NOT NULL DEFAULT 0.00,
			total_cost NUMERIC(15,2) NOT NULL DEFAULT 0.00,
			total_revenue NUMERIC(15,2) NOT NULL DEFAULT 0.00,
			created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
		)`,
		`ALTER TABLE commercial_resources ADD COLUMN IF NOT EXISTS total_cost NUMERIC(15,2) NOT NULL DEFAULT 0.00`,
		`ALTER TABLE commercial_resources ADD COLUMN IF NOT EXISTS total_revenue NUMERIC(15,2) NOT NULL DEFAULT 0.00`,
		`CREATE INDEX IF NOT EXISTS idx_comm_res_est ON commercial_resources(commercial_estimation_id)`,

		// Commercial Expenses
		`CREATE TABLE IF NOT EXISTS commercial_expenses (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			commercial_estimation_id UUID NOT NULL REFERENCES commercial_estimations(id) ON DELETE CASCADE,
			expense_type VARCHAR(100) NOT NULL,
			cost NUMERIC(15,2) NOT NULL DEFAULT 0.00,
			remarks TEXT,
			created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_comm_exp_est ON commercial_expenses(commercial_estimation_id)`,

		// SDLC Allocations
		`CREATE TABLE IF NOT EXISTS sdlc_allocations (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			commercial_estimation_id UUID NOT NULL REFERENCES commercial_estimations(id) ON DELETE CASCADE,
			phase VARCHAR(100) NOT NULL,
			man_days INT NOT NULL DEFAULT 0,
			created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_sdlc_est ON sdlc_allocations(commercial_estimation_id)`,
		// Tasks Table
		`CREATE TABLE IF NOT EXISTS tasks (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			text TEXT NOT NULL,
			due_date DATE,
			priority VARCHAR(20) NOT NULL CHECK (priority IN ('Low', 'Medium', 'High')),
			completed BOOLEAN NOT NULL DEFAULT FALSE,
			created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_tasks_user_id ON tasks(user_id)`,
	}

	script := strings.Join(queries, ";\n")
	if _, err := pool.Exec(ctx, script); err != nil {
		// Fallback to sequential execution if a batch error occurs
		for _, q := range queries {
			if _, err := pool.Exec(ctx, q); err != nil {
				return err
			}
		}
	}
	return nil
}
