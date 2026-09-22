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

	tables := []string{"commercial_estimations", "commercial_resources", "commercial_expenses", "sdlc_allocations"}
	for _, t := range tables {
		fmt.Printf("=== INDEXES ON %s ===\n", t)
		rows, err := db.Query(ctx, "SELECT indexname, indexdef FROM pg_indexes WHERE tablename = $1", t)
		if err != nil {
			log.Fatal(err)
		}
		for rows.Next() {
			var name, def string
			rows.Scan(&name, &def)
			fmt.Printf(" - %s: %s\n", name, def)
		}
		rows.Close()
	}

	// Also inspect EXPLAIN ANALYZE for the estimation query
	fmt.Println("\n=== EXPLAIN FOR commercial_estimations ===")
	explainRow := db.QueryRow(ctx, "EXPLAIN SELECT * FROM commercial_estimations WHERE lead_id = 'L-7001'")
	var plan string
	explainRow.Scan(&plan)
	fmt.Println(plan)
}
