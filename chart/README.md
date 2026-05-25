# walkthrough — Helm chart

Deploys the cloud-security-portfolio walkthrough as a per-link-token-gated
app with a password-protected admin portal. Backed by CNPG Postgres.

## Install

```bash
JWT_KEY=$(openssl rand -base64 32)
ADMIN_PW=$(openssl rand -hex 24)        # for the browser portal
ADMIN_TOKEN=$(openssl rand -hex 32)     # optional, for CLI scripts
IP_SALT=$(openssl rand -hex 16)

helm install walkthrough ./chart \
  --namespace walkthrough --create-namespace \
  --set secret.jwtPrivateKey="$JWT_KEY" \
  --set secret.adminPassword="$ADMIN_PW" \
  --set secret.adminToken="$ADMIN_TOKEN" \
  --set secret.ipSalt="$IP_SALT" \
  --set ingress.host=walkthrough.your.domain \
  --set publicUrl=https://walkthrough.your.domain
```

## Lock admin to corporate egress IPs (recommended)

```bash
helm upgrade walkthrough ./chart \
  --reuse-values \
  --set 'ingress.adminAllowCIDRs={203.0.113.0/24,198.51.100.42/32}'
```

This adds a second Ingress resource that whitelists `/admin/*` to those CIDRs.
The public deck path (`/`, `/api/event`) stays open so attendees can scan
from the conference Wi-Fi.

## Use the admin portal

Open `https://walkthrough.your.domain/admin`, enter the admin password. Fill
in the event details and click **Mint & show QR** — the page renders the
QR inline and offers a one-click download.

## CLI mint (optional)

```bash
curl -sS -X POST https://walkthrough.your.domain/admin/mint \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"event_id":"aws-summit-mtl-2026","nbf":"2026-05-25T08:00:00Z","exp":"2026-05-27T20:00:00Z"}'
```

## Without CNPG

`--set postgres.cnpg.enabled=false --set postgres.externalUrl='postgres://…'`
