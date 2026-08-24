package main

import (
	"context"
	"fmt"
	"log"

	"crm-auth-service/conf"
	"crm-auth-service/helpers"
)

func main() {
	cfg, err := conf.LoadConfig()
	if err != nil {
		log.Fatal(err)
	}
	logger := helpers.NewLogger(cfg.Server.Env)
	db, err := conf.ConnectDB(cfg.DB, logger)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	ctx := context.Background()
	email := "newuser@pmrgsolution.com"
	password := "Welcome@123"

	var storedHash string
	err = db.QueryRow(ctx, "SELECT password_hash FROM users WHERE email = $1", email).Scan(&storedHash)
	if err != nil {
		log.Fatalf("User not found or DB error: %v", err)
	}

	fmt.Printf("User found: %s\n", email)
	fmt.Printf("Stored hash (first 30 chars): %s...\n", storedHash[:30])

	match := helpers.ComparePassword(password, storedHash)
	fmt.Printf("Password 'Welcome@123' matches: %v\n", match)

	// Also try admin
	email2 := "admin@pmrgsolution.com"
	var storedHash2 string
	err = db.QueryRow(ctx, "SELECT password_hash FROM users WHERE email = $1", email2).Scan(&storedHash2)
	if err != nil {
		log.Fatalf("Admin not found: %v", err)
	}
	match2 := helpers.ComparePassword(password, storedHash2)
	fmt.Printf("\nAdmin '%s' Password matches: %v\n", email2, match2)
}
