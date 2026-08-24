#!/bin/sh
# Entrypoint script for Ordering-Backend service
# Waits for database to be ready before starting the server

set -e

# Use direct PostgreSQL URL for migrate/seed to bypass PgBouncer transaction mode.
MIGRATE_URL="${POSTGRES_MIGRATE_URL:-$POSTGRES_URL}"

echo "=========================================="
echo "Ordering-Backend Service Startup"
echo "=========================================="

# Sync media assets to persistent volume if mounted
if [ -d "/media" ] && [ -d "/app/media" ]; then
  echo "Synchronizing media assets to persistent volume..."
  cp -rn /app/media/* /media/ 2>/dev/null || true
  echo "Media synchronization complete"
fi

echo "Waiting for database and running migrations..."
MAX_RETRIES=60
RETRY_COUNT=0

# Captured (not swallowed) so a real migration failure is visible on every attempt -- the
# liveness probe usually kills this container long before MAX_RETRIES is ever reached.
until MIGRATE_OUTPUT=$(POSTGRES_URL="$MIGRATE_URL" /usr/local/bin/ordering-migrate 2>&1) || [ $RETRY_COUNT -eq $MAX_RETRIES ]; do
  RETRY_COUNT=$((RETRY_COUNT+1))
  echo "Migration attempt $RETRY_COUNT/$MAX_RETRIES failed:"
  echo "$MIGRATE_OUTPUT"
  sleep 5
done

if [ $RETRY_COUNT -eq $MAX_RETRIES ]; then
  echo "Migration failed after $MAX_RETRIES attempts. Last error:"
  echo "$MIGRATE_OUTPUT"
  exit 1
fi

echo "Migrations applied successfully"

echo ""
echo "=========================================="
echo "Running seed (idempotent)"
echo "=========================================="
POSTGRES_URL="$MIGRATE_URL" /usr/local/bin/ordering-seed || echo "Seed completed with warnings (non-fatal)"

echo ""
echo "=========================================="
echo "Starting Ordering-Backend server"
echo "=========================================="
echo ""

exec /usr/local/bin/ordering-backend
