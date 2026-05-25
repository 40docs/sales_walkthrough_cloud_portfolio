package main

import (
	"context"
	"database/sql"
	"embed"
	"html/template"
	"log"
	"net/http"
	"os"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

//go:embed cloud_portfolio.html
var deckHTML []byte

//go:embed admin/login.html admin/portal.html
var adminFS embed.FS

//go:embed migrations/*.sql
var migrationsFS embed.FS

var (
	db             *sql.DB
	adminLoginTpl  *template.Template
	adminPortalTpl *template.Template
)

func main() {
	addr := envOr("LISTEN_ADDR", ":8080")
	must(loadKeys())

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatal("DATABASE_URL required")
	}
	if os.Getenv("ADMIN_PASSWORD") == "" && os.Getenv("ADMIN_TOKEN") == "" {
		log.Fatal("ADMIN_PASSWORD or ADMIN_TOKEN required")
	}

	adminLoginTpl = template.Must(template.ParseFS(adminFS, "admin/login.html"))
	adminPortalTpl = template.Must(template.ParseFS(adminFS, "admin/portal.html"))

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

	// Attendee surface — write-only telemetry. No admin paths reachable.
	mux.HandleFunc("/api/event", recordEvent)

	// Admin surface — password login, separate cookie scoped to /admin.
	mux.HandleFunc("/admin", adminPortal)
	// Anything else under /admin that isn't a registered exact route is 404.
	// Longer registered paths (/admin/login, /admin/mint, ...) win over this.
	mux.HandleFunc("/admin/", http.NotFound)
	mux.HandleFunc("/admin/login", adminLogin)
	mux.HandleFunc("/admin/logout", adminLogout)
	mux.HandleFunc("/admin/mint", adminMint)
	mux.HandleFunc("/admin/qr", adminQR)

	// Deck served at /
	mux.HandleFunc("/", serveDeck)

	srv := &http.Server{
		Addr:              addr,
		Handler:           withLogging(withSecurityHeaders(mux)),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	log.Printf("walkthrough listening on %s", addr)
	must(srv.ListenAndServe())
}
