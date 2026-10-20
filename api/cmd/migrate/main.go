package main

import (
	"context"
	"database/sql"
	"log"
	"os"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

func main() {
	if len(os.Args) != 2 || (os.Args[1] != "up" && os.Args[1] != "status") {
		log.Fatal("usage: migrate [up|status]")
	}
	url := os.Getenv("DB_URL")
	if url == "" {
		log.Fatal("DB_URL is required")
	}
	db, err := sql.Open("pgx", url)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	if err := goose.SetDialect("postgres"); err != nil {
		log.Fatal(err)
	}
	if err := goose.RunContext(context.Background(), os.Args[1], db, "sql"); err != nil {
		log.Fatal(err)
	}
}
