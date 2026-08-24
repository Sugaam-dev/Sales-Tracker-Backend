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

	newPassword := "Welcome@123"
	hash, err := helpers.HashPassword(newPassword)
	if err != nil {
		log.Fatalf("Failed to hash: %v", err)
	}

	// Update newuser's password and reset is_first_login to true
	_, err = db.Exec(ctx,
		"UPDATE users SET password_hash = $1, is_first_login = true WHERE email = $2",
		hash, "newuser@pmrgsolution.com",
	)
	if err != nil {
		log.Fatalf("Failed to update: %v", err)
	}

	fmt.Println("✅ newuser@pmrgsolution.com password reset to: Welcome@123")
	fmt.Println("✅ is_first_login set to: true")
	fmt.Println("\nAb Postman me login karo:")
	fmt.Println(`  identifier: newuser@pmrgsolution.com`)
	fmt.Println(`  password:   Welcome@123`)
}
