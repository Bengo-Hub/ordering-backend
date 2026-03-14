package main

import (
	"database/sql"
	"log"
	_ "github.com/jackc/pgx/v5/stdlib"
)

func main() {
	db, err := sql.Open("pgx", "postgres://postgres:postgres@localhost:5432/ordering?sslmode=disable")
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer db.Close()

	_, err = db.Exec("CREATE SCHEMA IF NOT EXISTS ent_dev;")
	if err != nil {
		log.Fatalf("create schema: %v", err)
	}
	log.Println("Schema ent_dev created successfully")
}
