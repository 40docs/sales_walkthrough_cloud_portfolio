# Cloud Security Portfolio Walkthrough

5-minute, seller-led, in-person presentation deck for the Fortinet cloud
security portfolio. Two deploy targets share one source file
(`cloud_portfolio.html`):

| Target               | Workflow            | Auth                  | Telemetry |
| -------------------- | ------------------- | --------------------- | --------- |
| GitHub Pages preview | `.github/pages.yml` | none                  | no-op     |
| Container (k8s)      | `.github/build.yml` | per-link signed token | yes       |

## Container shape

Single Go binary embedding the deck. Three surfaces, isolated from each other:

```
                ┌──────────────────────────────────────────────────────┐
                │                                                      │
  QR scan ────► │  GET  /             Verifies ?t=, sets wt_sess       │
                │  POST /api/event    Telemetry (write-only)           │
                │                                                      │
  Admin in   ─► │  GET  /admin        Portal (wt_admin cookie required)│
  browser       │  GET  /admin/login                                   │
                │  POST /admin/login  Password-gated, rate-limited     │
                │  POST /admin/logout                                  │
                │  POST /admin/mint   Mints event tokens               │
                │  GET  /admin/qr     Renders QR PNG                   │
                │                                                      │
  CLI / CI   ─► │  POST /admin/mint   Bearer ADMIN_TOKEN               │
                └──────────────────────────────────────────────────────┘
```

## Security model

The thing the user-facing deck can do: **post telemetry events** through a
session cookie minted from a valid event link. That's it.

### Isolation between surfaces

| Surface         | Cookie         | Path scope | Token `typ` / `aud`                  |
| --------------- | -------------- | ---------- | ------------------------------------ |
| Attendee deck   | `wt_sess`      | `/`        | `sess` / `walkthrough-session`       |
| Admin portal    | `wt_admin`     | `/admin`   | `admin` / `walkthrough-admin`        |
| CLI mint        | _none_         | _none_     | bearer `ADMIN_TOKEN` (env-injected)  |

Every JWT carries a `typ` and a JWT-validated `aud`. The admin parser only
accepts `typ=admin && aud=walkthrough-admin`; the session parser only accepts
`typ=sess && aud=walkthrough-session`. An attendee session cookie cannot pass
admin auth even if it were somehow presented to `/admin/*`.

### Hardening in this build

- **Stateless EdDSA-signed JWTs** for every auth path; one signing key,
  rotated by replacing the `JWT_PRIVATE_KEY` secret.
- **Admin password** verified with `crypto/subtle.ConstantTimeCompare`.
- **Per-IP rate limit** on `/admin/login` (5 failures / minute).
- **Per-session rate limit** on `/api/event` (max 200 events).
- **Input validation**: `/api/event` only accepts a known event enum and a
  scene-ID allowlist; `/admin/mint` caps field lengths and window length
  (≤ 30 days).
- **Security headers** on every response: strict CSP (same-origin + Google
  Fonts only), `Strict-Transport-Security`, `X-Frame-Options: DENY`,
  `X-Content-Type-Options: nosniff`, `Referrer-Policy: same-origin`,
  `Permissions-Policy` disabling camera/mic/geo.
- **Cookies**: `HttpOnly`, `Secure`, `SameSite=Strict` for admin /
  `SameSite=Lax` for the attendee session (required for the QR-driven
  navigation). Admin cookie path-scoped to `/admin`.
- **Audit logging** for admin login (success + failure with IP) and mint
  operations (event, presenter, window, IP).
- **Pod hardening**: distroless static image, non-root (UID 65532),
  read-only root filesystem, all Linux capabilities dropped,
  `seccompProfile: RuntimeDefault`, no service-account token mounted.
- **NetworkPolicy**: ingress from the nginx-ingress namespace only;
  egress to the CNPG Postgres pod, DNS, and outbound 443 only (so the
  pod can't initiate connections to arbitrary internal services).
- **Optional admin IP allowlist**: a second Ingress with
  `whitelist-source-range` over `/admin/*` so the portal is only reachable
  from corporate egress (the deck stays open for attendees).
- **PodDisruptionBudget** keeps at least one replica during drains.

### What the deck explicitly cannot do

- Reach `/admin/*` — its cookie fails `parseAdminSession` (typ/aud mismatch).
- Reach `/admin/mint` — neither cookie nor a bearer of `ADMIN_TOKEN` is
  present; both checks fail.
- Send arbitrary phase names — only the 4 deck scene IDs are accepted.
- Spam telemetry — rate-limited per session.
- Read its own session cookie — `HttpOnly` blocks JS access.

## Endpoints

| Path               | Auth                                | Purpose                                        |
| ------------------ | ----------------------------------- | ---------------------------------------------- |
| `GET /`            | URL `?t=<JWT>` or `wt_sess` cookie  | Verifies token, sets cookie, serves deck       |
| `POST /api/event`  | `wt_sess` cookie                    | `phase_enter` / `session_end` from `sendBeacon`|
| `GET /admin`       | `wt_admin` cookie                   | Portal UI                                      |
| `GET /admin/login` | none                                | Login form                                     |
| `POST /admin/login`| password form field                 | Authenticates, sets `wt_admin`                 |
| `POST /admin/logout`| `wt_admin` cookie                  | Clears cookie                                  |
| `POST /admin/mint` | `wt_admin` cookie OR bearer token   | Mints per-event URL token                      |
| `GET /admin/qr`    | `wt_admin` cookie                   | PNG QR of supplied URL                         |
| `GET /healthz`     | none                                | Liveness                                       |

## Deploy

See [`chart/README.md`](chart/README.md).

## Local dev

```bash
export DATABASE_URL='postgres://walkthrough@localhost:5432/walkthrough?sslmode=disable'
export JWT_PRIVATE_KEY=$(openssl rand -base64 32)
export ADMIN_PASSWORD='choose-a-password'
export ADMIN_TOKEN=$(openssl rand -hex 32)
go run .
```
