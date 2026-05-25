# Cloud Security Portfolio Walkthrough

A 5-minute, seller-led, in-person presentation deck for the Fortinet cloud
security portfolio. Two deploy targets share one source file
(`cloud_portfolio.html`):

| Target                  | Workflow            | Auth        | Telemetry |
| ----------------------- | ------------------- | ----------- | --------- |
| GitHub Pages preview    | `.github/pages.yml` | none        | no-op     |
| Container (k8s)         | `.github/build.yml` | per-link JWT | yes      |

## Container shape

Single Go binary (`main.go`, `handlers.go`, `token.go`, `store.go`) that
embeds the deck and serves:

- `GET  /`             — verifies `?t=<JWT>` from the QR, sets a session
                         cookie, serves the deck. Requires a valid token
                         or session cookie.
- `POST /api/event`    — phase-enter and session-end events from the deck
                         (sent via `navigator.sendBeacon`).
- `POST /admin/mint`   — bearer-auth'd; mints a per-event URL token.
- `GET  /healthz`

Data lives in Postgres (CNPG operator-managed inside the cluster by default):

```sql
sessions(id, event_id, presenter_id, ua, ip_hash, started_at, ended_at)
phase_events(session_id, phase, entered_at, dwell_ms)
```

## Auth model — per-link signed tokens

Tokens are EdDSA-signed JWTs. Each event gets one token, baked into a QR;
multiple attendees scan the same QR. Token claims include `event_id`,
`event_name`, optional `presenter_id`, `nbf` and `exp`. Outside the
`nbf`/`exp` window the link is dead — blast radius = the conference window.

Mint a token (after `helm install`):

```bash
curl -sS -X POST https://walkthrough.your.domain/admin/mint \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
        "event_id":"aws-summit-mtl-2026",
        "event_name":"AWS Summit Montréal 2026",
        "presenter_id":"ajammes",
        "nbf":"2026-05-25T08:00:00Z",
        "exp":"2026-05-27T20:00:00Z"
      }'
# → { "token": "...", "url": "https://walkthrough.your.domain/?t=..." }
```

Drop that `url` into a QR generator (e.g. `qrencode`) and print.

## Deploy

See [`chart/README.md`](chart/README.md) for `helm install` instructions.

## Local dev

```bash
export DATABASE_URL='postgres://walkthrough@localhost:5432/walkthrough?sslmode=disable'
export JWT_PRIVATE_KEY=$(openssl rand -base64 32)
export ADMIN_TOKEN=$(openssl rand -hex 32)
go run .
```
