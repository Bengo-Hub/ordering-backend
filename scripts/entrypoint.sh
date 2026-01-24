#!/bin/sh
# Entrypoint script for Ordering-Backend service
# Waits for database to be ready before starting the server

set -e

echo "=========================================="
echo "Ordering-Backend Service Startup"
echo "=========================================="

# Wait for database to be ready (with timeout)
echo "Waiting for database connection..."
# 60 retries * 5s = 5 minutes
MAX_RETRIES=60
RETRY_COUNT=0

# Use the ordering-migrate binary to check connection
# It will succeed if DB is ready, fail otherwise
until /usr/local/bin/ordering-migrate > /dev/null 2>&1 || [ $RETRY_COUNT -eq $MAX_RETRIES ]; do
  RETRY_COUNT=$((RETRY_COUNT+1))
  echo "Database not ready yet or migrations failing... (attempt $RETRY_COUNT/$MAX_RETRIES)"
  sleep 5
done

if [ $RETRY_COUNT -eq $MAX_RETRIES ]; then
  echo "Database connection timeout after $MAX_RETRIES attempts"
  echo "Proceeding to start server anyway (will fail if DB is critical)"
else
  echo "Database connected and migrations completed (attempt $RETRY_COUNT)"
fi

echo ""
echo "=========================================="
echo "Starting Ordering-Backend server"
echo "=========================================="
echo ""

exec /usr/local/bin/ordering-backend
