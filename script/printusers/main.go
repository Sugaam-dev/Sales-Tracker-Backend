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
	_ = godotenv.Load()
	host := os.Getenv("DB_HOST")
	port := os.Getenv("DB_PORT")
	user := os.Getenv("DB_USER")
	password := os.Getenv("DB_PASSWORD")
	dbname := os.Getenv("DB_NAME")
	sslmode := os.Getenv("DB_SSLMODE")

	dsn := fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=%s", user, password, host, port, dbname, sslmode)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close(context.Background())

	fmt.Println("--- USERS IN DATABASE ---")
	rows, err := conn.Query(ctx, "SELECT id, name, email, role, is_active FROM users")
	if err != nil {
		log.Fatal(err)
	}
	defer rows.Close()

	for rows.Next() {
		var id string
		var name *string
		var email string
		var role string
		var isActive bool
		if err := rows.Scan(&id, &name, &email, &role, &isActive); err != nil {
			log.Fatal(err)
		}
		nameStr := "NULL"
		if name != nil {
			nameStr = *name
		}
		fmt.Printf("ID: %s | Name: %s | Email: %s | Role: %s | Active: %t\n", id, nameStr, email, role, isActive)
	}

	fmt.Println("\n--- STAGES IN DATABASE ---")
	rowsStages, err := conn.Query(ctx, "SELECT id, name, status FROM lead_stages")
	if err != nil {
		log.Fatal(err)
	}
	defer rowsStages.Close()
	for rowsStages.Next() {
		var id int
		var name string
		var status string
		if err := rowsStages.Scan(&id, &name, &status); err != nil {
			log.Fatal(err)
		}
		fmt.Printf("ID: %d | Name: %s | Status: %s\n", id, name, status)
	}
}
