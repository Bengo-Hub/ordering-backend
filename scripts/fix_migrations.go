//go:build ignore

// fix_migrations.go clears the ordering database and generates a fresh Atlas initial migration.
// Run from ordering-backend root: go run ./scripts/fix_migrations.go
// Requires: POSTGRES_URL (default: postgres://postgres:postgres@localhost:5432/ordering?sslmode=disable)
package main

import (
	"bufio"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// Files to remove after generating fresh initial (so we keep only the new initial_schema).
// Files to include in cleanup (placeholder will be regenerated if missing by script)
var filesToRemove = []string{
	"20260315163533_initial_schema.sql",
}

func main() {
	dbURL := os.Getenv("POSTGRES_URL")
	if dbURL == "" {
		dbURL = "postgres://postgres:postgres@localhost:5432/ordering?sslmode=disable"
	}

	cwd, err := os.Getwd()
	if err != nil {
		log.Fatalf("Failed to get cwd: %v", err)
	}
	migrationsDir := filepath.Join(cwd, "internal", "ent", "migrate", "migrations")

	// 1. Clear database: drop and recreate public + ent_dev (ent migrate main uses search_path=ent_dev)
	log.Println("Connecting to database...")
	db, err := sql.Open("pgx", dbURL)
	if err != nil {
		log.Fatalf("Failed to connect: %v", err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		log.Fatalf("Failed to ping database (is Postgres running?): %v", err)
	}

	log.Println("Dropping and recreating schemas (public, ent_dev)...")
	_, err = db.Exec(`
		DROP SCHEMA IF EXISTS public CASCADE;
		DROP SCHEMA IF EXISTS ent_dev CASCADE;
		CREATE SCHEMA public;
		CREATE SCHEMA ent_dev;
		GRANT ALL ON SCHEMA public TO postgres;
		GRANT ALL ON SCHEMA public TO public;
		GRANT ALL ON SCHEMA ent_dev TO postgres;
		GRANT ALL ON SCHEMA ent_dev TO public;
	`)
	if err != nil {
		log.Fatalf("Failed to clear database: %v", err)
	}
	log.Println("✓ Database cleared")

	// 2. Aggressively clean migration directory to avoid checksum mismatch
	log.Println("Cleaning migrations directory...")
	files, _ := os.ReadDir(migrationsDir)
	for _, f := range files {
		if !f.IsDir() && (strings.HasSuffix(f.Name(), ".sql") || f.Name() == "atlas.sum") {
			os.Remove(filepath.Join(migrationsDir, f.Name()))
		}
	}
	// Create a placeholder .sql file and atlas.sum so go:embed migrations/*.sql and atlas.sum doesn't fail
	os.WriteFile(filepath.Join(migrationsDir, "00000000000000_placeholder.sql"), []byte("-- placeholder\n"), 0644)
	os.WriteFile(filepath.Join(migrationsDir, "atlas.sum"), []byte("h1:dummy\n"), 0644)
	log.Println("✓ Migrations directory cleaned and placeholders created")

	// 3. Generate fresh initial migration
	log.Println("Generating fresh Atlas initial migration...")
	cmd := exec.Command("go", "run", "-mod=mod", "internal/ent/migrate/main.go", "initial_schema")
	cmd.Dir = cwd
	cmd.Env = append(os.Environ(), "POSTGRES_URL="+dbURL)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		log.Fatalf("Failed to generate migration: %v", err)
	}
	log.Println("✓ New initial migration generated")

	// 3. Remove old/placeholder migration files so only the new initial remains
	for _, name := range filesToRemove {
		p := filepath.Join(migrationsDir, name)
		if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
			log.Printf("Warning: could not remove %s: %v", name, err)
		} else if !os.IsNotExist(err) {
			log.Printf("  Removed %s", name)
		}
	}

	// 5. Rewrite atlas.sum to only include remaining migration files
	sumPath := filepath.Join(migrationsDir, "atlas.sum")
	entries, _ := os.ReadDir(migrationsDir)
	var remaining []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".sql") {
			remaining = append(remaining, e.Name())
		}
	}
	if len(remaining) > 0 {
		// Atlas sum format: "filename  h1:hash" per line. Re-hash by reading existing sum and keeping only lines for remaining files.
		var sumLines []string
		if f, err := os.Open(sumPath); err == nil {
			sc := bufio.NewScanner(f)
			for sc.Scan() {
				line := sc.Text()
				for _, r := range remaining {
					if strings.HasPrefix(line, r+" ") {
						sumLines = append(sumLines, line)
						break
					}
				}
			}
			f.Close()
		}
		if len(sumLines) > 0 {
			if err := os.WriteFile(sumPath, []byte(strings.Join(sumLines, "\n")+"\n"), 0644); err != nil {
				log.Printf("Warning: could not rewrite atlas.sum: %v", err)
			} else {
				log.Println("✓ atlas.sum updated")
			}
		}
	}

	fmt.Println("\nNext: run the app or 'go run ./cmd/migrate/main.go' to apply migrations, then 'go run ./cmd/seed/main.go' to seed.")
}

// writePlaceholderSum writes a valid atlas.sum for the placeholder file so Atlas Validate passes.
// Atlas format: first line "h1:<overall_sum>", then "<filename> h1:<base64(sha256(name+content))>" per file.
func writePlaceholderSum(migrationsDir string) {
	const placeholderName = "00000000000000_placeholder.sql"
	p := filepath.Join(migrationsDir, placeholderName)
	body, err := os.ReadFile(p)
	if err != nil {
		log.Printf("Warning: could not read placeholder to compute sum: %v", err)
		return
	}
	h := sha256.New()
	h.Write([]byte(placeholderName))
	h.Write(body)
	fileHash := base64.StdEncoding.EncodeToString(h.Sum(nil))
	overall := sha256.New()
	overall.Write([]byte(placeholderName))
	overall.Write([]byte(fileHash))
	sumLine := base64.StdEncoding.EncodeToString(overall.Sum(nil))
	content := fmt.Sprintf("h1:%s\n%s h1:%s\n", sumLine, placeholderName, fileHash)
	if err := os.WriteFile(filepath.Join(migrationsDir, "atlas.sum"), []byte(content), 0644); err != nil {
		log.Printf("Warning: could not write atlas.sum: %v", err)
	}
}
