#!/usr/bin/env bash
# add-tenant.sh — Add a new Postiz tenant
#
# Usage: ./scripts/add-tenant.sh <tenant-id> <domain> <admin-email> [admin-password]
#
# Prerequisites:
#   - postiz-postgres container running on postiz-shared network
#   - jq installed (sudo dnf install jq)
set -euo pipefail

TENANT_ID="${1:?Usage: $0 <tenant-id> <domain> <admin-email> [admin-password]}"
DOMAIN="${2:?Usage: $0 <tenant-id> <domain> <admin-email> [admin-password]}"
ADMIN_EMAIL="${3:?Usage: $0 <tenant-id> <domain> <admin-email> [admin-password]}"
ADMIN_PASSWORD="${4:-PostizTest2026!}"

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"
TENANTS_FILE="$PROJECT_DIR/go-backend/tenants.json"

# Config — change these if your PG/Redis setup differs
PG_CONTAINER="postiz-postgres"
PG_USER="postiz-user"
PG_PASSWORD="postiz-password"
REDIS_CONTAINER="postiz-redis"
DB_NAME="postiz-${TENANT_ID}"

echo "=== Adding tenant: $TENANT_ID ==="
echo "  Domain:  $DOMAIN"
echo "  Email:   $ADMIN_EMAIL"
echo "  DB:      $DB_NAME"
echo ""

# 1. Create database
echo "[1/5] Creating database $DB_NAME..."
docker exec "$PG_CONTAINER" psql -U "$PG_USER" -d postgres -c "CREATE DATABASE \"$DB_NAME\";" 2>/dev/null || {
    echo "  Database already exists, continuing..."
}

# 2. Copy schema from existing ferroquant database
echo "[2/5] Copying schema from postiz-db-local..."
docker exec "$PG_CONTAINER" pg_dump -U "$PG_USER" -d "postiz-db-local" --schema-only | \
    docker exec -i "$PG_CONTAINER" psql -U "$PG_USER" -d "$DB_NAME" 2>/dev/null
echo "  Schema copied"

# 3. Generate JWT secret
JWT_SECRET=$(openssl rand -hex 32)
echo "[3/5] Generated JWT secret"

# 4. Create admin user + org
echo "[4/5] Creating admin user + organization..."
ORG_ID=$(cat /proc/sys/kernel/random/uuid)
USER_ID=$(cat /proc/sys/kernel/random/uuid)
USER_ORG_ID=$(cat /proc/sys/kernel/random/uuid)
API_KEY=$(openssl rand -hex 24)

# Hash password using bcrypt via python3 (available on most systems)
PASSWORD_HASH=$(python3 -c "
import bcrypt
print(bcrypt.hashpw(b'${ADMIN_PASSWORD}', bcrypt.gensalt(rounds=10)).decode())
" 2>/dev/null || echo '\$2b\$10\$placeholder_hash_update_manually')

docker exec "$PG_CONTAINER" psql -U "$PG_USER" -d "$DB_NAME" -c "
INSERT INTO \"Organization\" (id, name, description, \"apiKey\", \"createdAt\", \"updatedAt\")
VALUES ('$ORG_ID', '$TENANT_ID', 'Auto-provisioned tenant', '$API_KEY', NOW(), NOW())
ON CONFLICT DO NOTHING;

INSERT INTO \"User\" (id, email, password, \"providerName\", \"timezone\", activated, \"createdAt\", \"updatedAt\")
VALUES ('$USER_ID', '$ADMIN_EMAIL', '$PASSWORD_HASH', 'LOCAL', 0, true, NOW(), NOW())
ON CONFLICT DO NOTHING;

INSERT INTO \"UserOrganization\" (id, \"userId\", \"organizationId\", disabled, role, \"createdAt\", \"updatedAt\")
VALUES ('$USER_ORG_ID', '$USER_ID', '$ORG_ID', false, 'SUPERADMIN', NOW(), NOW())
ON CONFLICT DO NOTHING;
"

# 5. Add to tenants.json
echo "[5/5] Adding to tenants.json..."
if [ ! -f "$TENANTS_FILE" ]; then
    echo "[]" > "$TENANTS_FILE"
fi

if ! command -v jq &>/dev/null; then
    echo "ERROR: jq is required. Install with: sudo dnf install jq"
    exit 1
fi

jq --arg id "$TENANT_ID" \
   --arg host "$DOMAIN" \
   --arg db "postgresql://$PG_USER:$PG_PASSWORD@$PG_CONTAINER:5432/$DB_NAME" \
   --arg redis "redis://$REDIS_CONTAINER:6379" \
   --arg jwt "$JWT_SECRET" \
   --arg frontend "https://$DOMAIN" \
   --arg uploads "/uploads/$TENANT_ID" \
   '. += [{
     id: $id,
     hosts: [$host],
     database_url: $db,
     redis_url: $redis,
     jwt_secret: $jwt,
     frontend_url: $frontend,
     upload_dir: $uploads,
     social: {
       x_api_key: "",
       x_api_secret: "",
       tiktok_client_id: "",
       tiktok_client_secret: "",
       youtube_client_id: "",
       youtube_client_secret: "",
       facebook_app_id: "",
       facebook_app_secret: ""
     }
   }]' "$TENANTS_FILE" > "$TENANTS_FILE.tmp" && mv "$TENANTS_FILE.tmp" "$TENANTS_FILE"

echo ""
echo "=== Tenant $TENANT_ID created ==="
echo ""
echo "Login: $ADMIN_EMAIL / $ADMIN_PASSWORD"
echo "API Key: $API_KEY"
echo ""
echo "Next steps:"
echo "  1. Add social keys to tenants.json (copy from existing tenant if shared apps)"
echo "  2. Add Cloudflare tunnel route:"
echo "     cloudflared tunnel route dns <TUNNEL_ID> $DOMAIN"
echo "     Add to tunnel config:"
echo "       - hostname: $DOMAIN"
echo "         service: http://localhost:5000"
echo "  3. Add upload volume to docker-compose.multitenant.yaml"
echo "  4. Restart: docker compose -f docker-compose.multitenant.yaml restart"
