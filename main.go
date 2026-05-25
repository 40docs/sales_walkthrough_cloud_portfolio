package main

import (
	"context"
	"database/sql"
	"embed"
	"log"
	"net/http"
	"os"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

//go:embed cloud_portfolio.html
var deckHTML []byte

//go:embed migrations/*.sql
var migrationsFS embed.FS

var db *sql.DB

func main() {
	addr := envOr("LISTEN_ADDR", ":8080")
	must(loadKeys())

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatal("DATABASE_URL required")
	}
	if os.Getenv("ADMIN_TOKEN") == "" {
		log.Fatal("ADMIN_TOKEN required")
	}

	var err error
	db, err = sql.Open("pgx", dbURL)
	must(err)
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	must(db.PingContext(ctx))
	must(applyMigrations(db))

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", okHandler)
	mux.HandleFunc("/api/event", recordEvent)
	mux.HandleFunc("/admin/mint", adminMint)
	mux.HandleFunc("/", serveDeck)

	srv := &http.Server{
		Addr:              addr,
		Handler:           withLogging(mux),
		ReadHeaderTimeout: 5 * time.Second,
	}
	log.Printf("walkthrough listening on %s", addr)
	must(srv.ListenAndServe())
}
