package main

import (
	"context"
	"database/sql"
	"log"
	"os"
	"regexp"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/bengobox/ordering-backend/internal/config"
	"github.com/bengobox/ordering-backend/internal/ent"
)

// maskPassword masks the password in a database URL for logging
func maskPassword(url string) string {
	re := regexp.MustCompile(`://([^:]+):([^@]+)@`)
	return re.ReplaceAllString(url, "://$1:****@")
}

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Use LoadDatabaseOnly to avoid OAuth validation failures
	dbCfg, err := config.LoadDatabaseOnly()
	if err != nil {
		log.Fatalf("load database config: %v", err)
	}

	log.Printf("connecting to database: %s", maskPassword(dbCfg.URL))

	db, err := sql.Open("pgx", dbCfg.URL)
	if err != nil {
		log.Fatalf("open ent driver: %v", err)
	}
	db.SetMaxIdleConns(dbCfg.MaxIdleConns)
	db.SetMaxOpenConns(dbCfg.MaxOpenConns)
	db.SetConnMaxIdleTime(5 * time.Minute)

	drv := entsql.OpenDB(dialect.Postgres, db)
	client := ent.NewClient(ent.Driver(drv))
	defer client.Close()

	if reset := os.Getenv("CAFE_RESET_DB"); reset == "true" {
		if _, err := db.ExecContext(ctx, "DROP SCHEMA IF EXISTS public CASCADE"); err != nil {
			log.Fatalf("reset schema: %v", err)
		}
		if _, err := db.ExecContext(ctx, "CREATE SCHEMA public"); err != nil {
			log.Fatalf("create schema: %v", err)
		}
	}

	if err := client.Schema.Create(ctx); err != nil {
		log.Fatalf("run migrations: %v", err)
	}

	log.Println("database migrations applied successfully")
}
