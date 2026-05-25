# walkthrough — Helm chart

Deploys the cloud-security-portfolio walkthrough as a per-link-token-gated
Pages app, backed by a CNPG-managed Postgres in the same namespace.

## Install

```bash
# 1. Generate a JWT signing key (one-time)
JWT_KEY=$(openssl rand -base64 32)

# 2. Pick a strong admin token
ADMIN=$(openssl rand -hex 32)

helm install walkthrough ./chart \
  --namespace walkthrough --create-namespace \
  --set secret.jwtPrivateKey="$JWT_KEY" \
  --set secret.adminToken="$ADMIN" \
  --set ingress.host=walkthrough.your.domain \
  --set publicUrl=https://walkthrough.your.domain
```

## Mint a per-event token

```bash
curl -sS -X POST https://walkthrough.your.domain/admin/mint \
  -H "Authorization: Bearer $ADMIN" \
  -H "Content-Type: application/json" \
  -d '{
        "event_id":"aws-summit-mtl-2026",
        "event_name":"AWS Summit Montréal 2026",
        "presenter_id":"ajammes",
        "nbf":"2026-05-25T08:00:00Z",
        "exp":"2026-05-27T20:00:00Z"
      }' | jq
```

The response includes a `url` (with `?t=...`) — encode that into a QR.

## Without CNPG

Set `postgres.cnpg.enabled=false` and provide `postgres.externalUrl` pointing
at a managed Postgres.
