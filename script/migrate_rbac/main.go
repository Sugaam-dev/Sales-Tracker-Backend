package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load("c:/Users/sahil/OneDrive/Documents/GitHub/Sales-Tracker-Backend/.env")
	host := os.Getenv("DB_HOST")
	port := os.Getenv("DB_PORT")
	user := os.Getenv("DB_USER")
	password := os.Getenv("DB_PASSWORD")
	dbname := os.Getenv("DB_NAME")
	sslmode := os.Getenv("DB_SSLMODE")

	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=%s", host, port, user, password, dbname, sslmode)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		log.Fatal("Connect failed: ", err)
	}
	defer conn.Close(context.Background())

	sqlBytes, err := os.ReadFile("c:/Users/sahil/OneDrive/Documents/GitHub/Sales-Tracker-Backend/script/migrations/001_rbac_and_row_level_security.sql")
	if err != nil {
		log.Fatal("Failed to read migration sql: ", err)
	}

	fmt.Println("Running RBAC migration...")
	_, err = conn.Exec(ctx, string(sqlBytes))
	if err != nil {
		log.Fatal("Migration failed: ", err)
	}
	fmt.Println("Migration successfully applied!")

	// Verify columns
	var leadCountWithOwnership int64
	err = conn.QueryRow(ctx, "SELECT count(*) FROM leads WHERE created_by IS NOT NULL AND assigned_to IS NOT NULL").Scan(&leadCountWithOwnership)
	if err != nil {
		log.Fatal(err)
	}
	var totalLeads int64
	_ = conn.QueryRow(ctx, "SELECT count(*) FROM leads").Scan(&totalLeads)
	fmt.Printf("Leads with ownership: %d / %d\n", leadCountWithOwnership, totalLeads)

	// Display roles in users table
	fmt.Println("\nCurrent roles in users table:")
	rows, err := conn.Query(ctx, "SELECT role, count(*) FROM users GROUP BY role")
	if err != nil {
		log.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var role string
		var cnt int64
		if err := rows.Scan(&role, &cnt); err == nil {
			fmt.Printf("  %s: %d users\n", role, cnt)
		}
	}
}
