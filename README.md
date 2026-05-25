# Sales Cloud Assessment (k8s-hosted)

Containerized, auth-gated, telemetry-collecting deploy of the 4-scenario
**Security Maturity Assessment** plus a password-protected admin portal
for minting per-event QR tokens.

The assessment HTML files (`security-maturity-assessment-browser.html`,
`security-maturity-assessment-mobile.html`) are kept as the source of truth
in upstream [`40docs/sales_cloud-assessment`](https://github.com/40docs/sales_cloud-assessment).
This repo embeds them at build time and serves the right variant per device.

## Container shape

Single Go binary embedding both variants. Three surfaces, isolated from each other:

```
                ┌──────────────────────────────────────────────────────┐
  QR scan ────► │  GET  /             Verifies ?t=, sets wt_sess,      │
                │                     picks browser/mobile variant     │
                │  POST /api/event    phase_enter / pick / submit /    │
                │                     session_end (write-only)         │
                │                                                      │
  Admin in   ─► │  GET  /admin        Mint UI                          │
  browser       │  GET  /admin/login                                   │
                │  POST /admin/login  Password-gated, rate-limited     │
                │  POST /admin/mint   Issues per-event URL tokens      │
                │  GET  /admin/qr     Renders QR PNG                   │
                │  POST /admin/logout                                  │
                │                                                      │
  CLI / CI   ─► │  POST /admin/mint   Bearer ADMIN_TOKEN               │
                └──────────────────────────────────────────────────────┘
```

## Device variant selection

Server-side, no client-side router:

1. `Sec-CH-UA-Mobile: ?1` client hint → serve mobile.
2. Otherwise, UA string contains `Mobile|Android|iPhone|iPad|iPod|Mobi|Opera Mini|IEMobile` → serve mobile.
3. Else → serve browser (desktop).

Response sets `Vary: User-Agent, Sec-CH-UA-Mobile` so caches store both variants correctly.

## Telemetry captured per session

| Event          | Trigger                                      | Stored in       |
| -------------- | -------------------------------------------- | --------------- |
| `phase_enter`  | Scene transitions (intro / scenario / results) | `phase_events` |
| `pick`         | Each scenario outcome selected               | `picks`         |
| `submit`       | Email submission on results screen           | `submissions`   |
| `session_end`  | Tab/window close                             | updates `sessions.ended_at` |

The assessment HTML is patched with a small `<script>` block that wraps
`goTo()`, `pick()`, and `sendReport()` and emits these via `navigator.sendBeacon`.
The patch is reapplied on every sync from upstream.

## Security model

| Surface         | Cookie         | Path scope | Token `typ` / `aud`                  |
| --------------- | -------------- | ---------- | ------------------------------------ |
| Attendee deck   | `wt_sess`      | `/`        | `sess` / `walkthrough-session`       |
| Admin portal    | `wt_admin`     | `/admin`   | `admin` / `walkthrough-admin`        |
| CLI mint        | _none_         | _none_     | bearer `ADMIN_TOKEN`                 |

The deck cookie cannot satisfy admin auth (token parser checks `aud` + `typ`).
The admin cookie is path-scoped to `/admin` and `SameSite=Strict`. CLI bearer
is verified with `crypto/subtle.ConstantTimeCompare`.

### Other defenses

- **Strict CSP** + HSTS + `X-Frame-Options: DENY` + `nosniff` + Referrer-Policy + Permissions-Policy on every response.
- **Input validation** on `/api/event`: phase allowlist, scenario allowlist (`network/app/cnapp/sspm`), `outcome_idx 0..2`, `score 0..2`, color in `{red,yellow,green}`, email regex, `scores` jsonb ≤ 4 KB.
- **Rate limits**: per-IP login limiter (5/min), per-session event cap (400 events).
- **Pod**: distroless static, non-root, read-only rootfs, all caps dropped, `seccompProfile: RuntimeDefault`, no service-account token mounted.
- **NetworkPolicy**: ingress from `ingress-nginx` ns only; egress to CNPG Postgres pod, DNS, and 443 only.
- **Optional admin IP allowlist** via a separate Ingress with `whitelist-source-range` over `/admin/*`.
- **PodDisruptionBudget**: minAvailable=1.
- **Audit log** for admin login (success + fail with IP) and every mint.

## Endpoints

| Path                | Auth                              | Purpose                                            |
| ------------------- | --------------------------------- | -------------------------------------------------- |
| `GET /`             | URL `?t=` or `wt_sess`            | Mint session + serve device-appropriate assessment |
| `POST /api/event`   | `wt_sess`                         | Telemetry — phase/pick/submit/session-end          |
| `GET /admin`        | `wt_admin`                        | Portal UI                                          |
| `GET /admin/login`  | none                              | Login form                                         |
| `POST /admin/login` | password (form field)             | Sets `wt_admin`                                    |
| `POST /admin/logout`| `wt_admin`                        | Clears cookie                                      |
| `POST /admin/mint`  | `wt_admin` OR bearer token        | Mints per-event URL token                          |
| `GET /admin/qr`     | `wt_admin`                        | PNG QR of supplied URL                             |
| `GET /healthz`      | none                              | Liveness                                           |

## Deploy

See [`chart/README.md`](chart/README.md) for `helm install` instructions.

## Sync from upstream

```bash
UP=https://raw.githubusercontent.com/40docs/sales_cloud-assessment/main
curl -sSL $UP/security-maturity-assessment-browser.html > security-maturity-assessment-browser.html
curl -sSL $UP/security-maturity-assessment-mobile.html  > security-maturity-assessment-mobile.html
# Re-apply telemetry script (see git log for the snippet) and commit.
```

## Local dev

```bash
export DATABASE_URL='postgres://walkthrough@localhost:5432/walkthrough?sslmode=disable'
export JWT_PRIVATE_KEY=$(openssl rand -base64 32)
export ADMIN_PASSWORD='choose-a-password'
export ADMIN_TOKEN=$(openssl rand -hex 32)
go run .
```
