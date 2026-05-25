package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	cookieName = "wt_sess"
	tokenParam = "t"
)

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func must(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

func okHandler(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok")) }

func withLogging(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		h.ServeHTTP(w, r)
		log.Printf("%s %s %s %s", r.Method, r.URL.Path, r.RemoteAddr, time.Since(start))
	})
}

func clientIPHash(r *http.Request) string {
	ip := r.Header.Get("X-Forwarded-For")
	if i := strings.Index(ip, ","); i > 0 {
		ip = ip[:i]
	}
	ip = strings.TrimSpace(ip)
	if ip == "" {
		host, _, _ := net.SplitHostPort(r.RemoteAddr)
		ip = host
	}
	salt := envOr("IP_SALT", "wt")
	sum := sha256.Sum256([]byte(salt + ":" + ip))
	return hex.EncodeToString(sum[:])[:16]
}

func serveDeck(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	if t := r.URL.Query().Get(tokenParam); t != "" {
		urlC, err := parseURLToken(t)
		if err != nil {
			http.Error(w, "this link is invalid or expired", http.StatusUnauthorized)
			return
		}
		sid := uuid.NewString()
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()
		if _, err := db.ExecContext(ctx,
			`INSERT INTO sessions(id, event_id, presenter_id, ua, ip_hash, started_at)
			 VALUES ($1,$2,$3,$4,$5,now())`,
			sid, urlC.EventID, urlC.PresenterID, r.UserAgent(), clientIPHash(r)); err != nil {
			log.Printf("session insert: %v", err)
			http.Error(w, "server error", http.StatusInternalServerError)
			return
		}
		exp := urlC.ExpiresAt.Time
		tok, err := mintSessionToken(sid, urlC.EventID, urlC.PresenterID, exp)
		if err != nil {
			http.Error(w, "server error", http.StatusInternalServerError)
			return
		}
		http.SetCookie(w, &http.Cookie{
			Name: cookieName, Value: tok, Path: "/",
			Expires: exp, HttpOnly: true, Secure: true, SameSite: http.SameSiteLaxMode,
		})
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}

	c, err := r.Cookie(cookieName)
	if err != nil {
		http.Error(w, "this walkthrough requires a valid link from the event QR code", http.StatusUnauthorized)
		return
	}
	if _, err := parseSessionToken(c.Value); err != nil {
		http.Error(w, "your session has expired — re-scan the event QR code", http.StatusUnauthorized)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = w.Write(deckHTML)
}

type eventPayload struct {
	Event   string `json:"event"`
	Phase   string `json:"phase,omitempty"`
	DwellMs int    `json:"dwell_ms,omitempty"`
}

func recordEvent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	c, err := r.Cookie(cookieName)
	if err != nil {
		http.Error(w, "no session", http.StatusUnauthorized)
		return
	}
	sess, err := parseSessionToken(c.Value)
	if err != nil {
		http.Error(w, "invalid session", http.StatusUnauthorized)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 4096))
	if err != nil {
		http.Error(w, "bad body", http.StatusBadRequest)
		return
	}
	var p eventPayload
	if err := json.Unmarshal(body, &p); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	switch p.Event {
	case "phase_enter":
		_, err = db.ExecContext(ctx,
			`INSERT INTO phase_events(session_id, phase, entered_at, dwell_ms)
			 VALUES ($1,$2,now(),$3)`,
			sess.SessionID, p.Phase, p.DwellMs)
	case "session_end":
		_, err = db.ExecContext(ctx,
			`UPDATE sessions SET ended_at = now() WHERE id = $1 AND ended_at IS NULL`,
			sess.SessionID)
	default:
		http.Error(w, "unknown event", http.StatusBadRequest)
		return
	}
	if err != nil {
		log.Printf("event insert: %v", err)
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type mintRequest struct {
	EventID     string `json:"event_id"`
	EventName   string `json:"event_name"`
	PresenterID string `json:"presenter_id"`
	NotBefore   string `json:"nbf"`
	Expires     string `json:"exp"`
	MaxUses     int    `json:"max_uses"`
}

type mintResponse struct {
	Token string `json:"token"`
	URL   string `json:"url,omitempty"`
}

func adminMint(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	auth := r.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "Bearer ") || strings.TrimPrefix(auth, "Bearer ") != os.Getenv("ADMIN_TOKEN") {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	var req mintRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&req); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}
	if req.EventID == "" {
		http.Error(w, "event_id required", http.StatusBadRequest)
		return
	}
	nbf := time.Now()
	exp := time.Now().Add(48 * time.Hour)
	if req.NotBefore != "" {
		t, err := time.Parse(time.RFC3339, req.NotBefore)
		if err != nil {
			http.Error(w, "bad nbf", http.StatusBadRequest)
			return
		}
		nbf = t
	}
	if req.Expires != "" {
		t, err := time.Parse(time.RFC3339, req.Expires)
		if err != nil {
			http.Error(w, "bad exp", http.StatusBadRequest)
			return
		}
		exp = t
	}
	tok, err := mintURLToken(req.EventID, req.EventName, req.PresenterID, nbf, exp, req.MaxUses)
	if err != nil {
		http.Error(w, "mint failed", http.StatusInternalServerError)
		return
	}
	resp := mintResponse{Token: tok}
	if base := envOr("PUBLIC_URL", ""); base != "" {
		resp.URL = strings.TrimRight(base, "/") + "/?t=" + tok
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}
