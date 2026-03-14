package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"

	"github.com/bengobox/ordering-backend/internal/config"
	"github.com/bengobox/ordering-backend/internal/ent"
	entsql "entgo.io/ent/dialect/sql"
	_ "github.com/jackc/pgx/v5/stdlib"
)

func main() {
	cfg, _ := config.Load()
	db, err := sql.Open("pgx", cfg.Postgres.URL)
	if err != nil {
		log.Fatalf("failed opening connection: %v", err)
	}
	drv := entsql.OpenDB("postgres", db)
	client := ent.NewClient(ent.Driver(drv))
	defer client.Close()

	ctx := context.Background()

	items, _ := client.MenuItem.Query().Limit(5).All(ctx)
	fmt.Println("--- Menu Items ---")
	for _, it := range items {
		fmt.Printf("Item: %s, Image: %s\n", it.Name, it.ImageURL)
	}

	cats, _ := client.MenuCategory.Query().Limit(5).All(ctx)
	fmt.Println("\n--- Categories ---")
	for _, c := range cats {
		fmt.Printf("Category: %s, Image: %s\n", c.Name, c.ImageURL)
	}
}
