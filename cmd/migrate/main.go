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

	// Every replica runs this binary on startup (see scripts/entrypoint.sh) — without
	// coordination, N pods launching together each run their own Schema.Create concurrently
	// against the same tables. This exact class of bug took down pos-api in production on
	// 2026-07-26 (concurrent migrate attempts from multiple starting replicas corrupted a
	// nullable-then-backfill migration on the live pos_orders table; see
	// .claude/memory/feedback_ent_atlas_migrations.md). A single physical connection
	// (MaxOpenConns=1) + a session-level Postgres advisory lock ensures only ONE pod across the
	// whole cluster ever executes the migration at a time; the rest block here until it
	// finishes, then find nothing pending and return immediately. Lock key 727271005 is
	// ordering-backend's own — distinct from pos-api's 727271001, inventory-api's 727271002,
	// treasury-api's 727271003, and hospital-api's 727271004 (all share one physical Postgres
	// instance, so reusing a key would make unrelated services block on each other for no reason).
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(5 * time.Minute)

	const migrationLockKey = 727271005
	if _, err := db.ExecContext(ctx, "SELECT pg_advisory_lock($1)", migrationLockKey); err != nil {
		log.Fatalf("acquire migration lock: %v", err)
	}
	defer func() {
		if _, err := db.ExecContext(context.Background(), "SELECT pg_advisory_unlock($1)", migrationLockKey); err != nil {
			log.Printf("release migration lock: %v", err)
		}
	}()

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
