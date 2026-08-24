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

	emails := []string{
		"admin@pmrgsolution.com",
		"manager@pmrgsolution.com",
		"agent@pmrgsolution.com",
		"newuser@pmrgsolution.com",
	}

	fmt.Println("Deleting existing CRM users...")
	for _, email := range emails {
		_, err := db.Exec(ctx, "DELETE FROM users WHERE email = $1", email)
		if err != nil {
			log.Fatalf("Failed to delete %s: %v", email, err)
		}
		fmt.Printf("✓ Deleted: %s\n", email)
	}
	fmt.Println("\nAll users deleted. Now run: go run script/seed.go")
}
