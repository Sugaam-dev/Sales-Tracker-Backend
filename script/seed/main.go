package main

import (
	"context"
	"errors"
	"fmt"
	"log"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool" 

	"crm-auth-service/conf"
	"crm-auth-service/helpers"
)

type SeedUser struct {
	Email        string
	Password     string
	Role         string
	IsFirstLogin bool
}

func main() {
	// Load configuration
	cfg, err := conf.LoadConfig()
	if err != nil {
		log.Fatal(err)
	}

	// Logger
	logger := helpers.NewLogger(cfg.Server.Env)

	// Database connection
	db, err := conf.ConnectDB(cfg.DB, logger)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	users := []SeedUser{
		{
			Email:        "admin@pmrgsolution.com",
			Password:     "Welcome@123",
			Role:         "admin",
			IsFirstLogin: false,
		},
		{
			Email:        "manager@pmrgsolution.com",
			Password:     "Welcome@123",
			Role:         "manager",
			IsFirstLogin: false,
		},
		{
			Email:        "agent@pmrgsolution.com",
			Password:     "Welcome@123",
			Role:         "agent",
			IsFirstLogin: false,
		},
		{
			Email:        "newuser@pmrgsolution.com",
			Password:     "Welcome@123",
			Role:         "agent",
			IsFirstLogin: true,
		},
	}

	fmt.Println(" Creating Default CRM Users")

	for _, user := range users {
		createUser(db, user)
	}

	fmt.Println()
	
	fmt.Println(" User Seeding Completed")
}

func createUser(db *pgxpool.Pool, data SeedUser) {
	ctx := context.Background()
	var email string
	queryCheck := "SELECT email FROM users WHERE email = $1"
	err := db.QueryRow(ctx, queryCheck, data.Email).Scan(&email)

	if err == nil {
		fmt.Printf("✓ User already exists : %s\n", data.Email)
		return
	}

	if !errors.Is(err, pgx.ErrNoRows) {
		log.Fatalf("database error : %v", err)
	}

	passwordHash, err := helpers.HashPassword(data.Password)
	if err != nil {
		log.Fatalf("failed to hash password : %v", err)
	}

	queryInsert := `INSERT INTO users (email, password_hash, role, is_first_login, email_verified, mobile_verified, mfa_enabled, created_at, updated_at)
					VALUES ($1, $2, $3, $4, $5, $6, $7, NOW(), NOW())`
	if _, err := db.Exec(ctx, queryInsert, data.Email, passwordHash, data.Role, data.IsFirstLogin, true, true, false); err != nil {
		log.Fatalf("failed to create user : %v", err)
	}

	fmt.Println("----------------------------------------")
	fmt.Printf("✓ User Created\n")
	fmt.Printf("Email              : %s\n", data.Email)
	fmt.Printf("Temporary Password : %s\n", data.Password)
	fmt.Printf("Role               : %s\n", data.Role)
	fmt.Printf("First Login        : NO\n")
	fmt.Println("----------------------------------------")
}