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
	"entgo.io/ent/dialect/sql/schema"
	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/bengobox/ordering-backend/internal/config"
	"github.com/bengobox/ordering-backend/internal/ent"
	"github.com/bengobox/ordering-backend/internal/ent/migrate"
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
	if dbCfg.MigrateURL != "" {
		dbCfg.URL = dbCfg.MigrateURL
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

	if err := client.Schema.Create(ctx,
		schema.WithDir(migrate.Dir),
	); err != nil {
		log.Fatalf("run migrations: %v", err)
	}

	// Converge the order_number uniqueness to PER-TENANT. Declarative auto-migrate is additive and
	// cannot drop the legacy GLOBAL unique index (which made two tenants collide on the same day's
	// YYYYMMDD-NNNN). Both statements are idempotent and safe: every existing order_number was
	// globally unique, so it is necessarily unique per (tenant_id, order_number).
	if _, err := db.ExecContext(ctx, `CREATE UNIQUE INDEX IF NOT EXISTS order_tenant_id_order_number ON orders (tenant_id, order_number)`); err != nil {
		log.Printf("warning: ensure per-tenant order_number index: %v", err)
	}
	if _, err := db.ExecContext(ctx, `DROP INDEX IF EXISTS order_order_number`); err != nil {
		log.Printf("warning: drop legacy global order_number index: %v", err)
	}

	log.Println("database migrations applied successfully")
}
