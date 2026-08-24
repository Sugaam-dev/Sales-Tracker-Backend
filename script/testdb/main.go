package main

import (
	"context"

	"fmt"

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

	dsn := fmt.Sprintf(

		"postgres://%s:%s@%s:%s/%s?sslmode=%s",

		user,

		password,

		host,

		port,

		dbname,

		sslmode,
	)

	fmt.Println("Testing PostgreSQL connection...")

	fmt.Println("Host:", host)

	fmt.Println("Port:", port)

	fmt.Println("User:", user)

	fmt.Println("Database:", dbname)

	fmt.Println("SSL Mode:", sslmode)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)

	defer cancel()

	conn, err := pgx.Connect(ctx, dsn)

	if err != nil {

		fmt.Println("CONNECTION FAILED:")

		fmt.Println(err)

		return

	}

	defer conn.Close(context.Background())

	if err := conn.Ping(ctx); err != nil {

		fmt.Println("PING FAILED:")

		fmt.Println(err)

		return

	}

	fmt.Println("DATABASE CONNECTION SUCCESSFUL")

}
