#!/usr/bin/env bash
# Environment secret setup script for Ordering Backend (Go)
# Retrieves DB credentials from existing Helm releases and creates app env secret

set -euo pipefail
set +H

# Inherit logging functions from parent script or define minimal ones
log_info() { echo -e "\033[0;34m[INFO]\033[0m $1"; }
log_success() { echo -e "\033[0;32m[SUCCESS]\033[0m $1"; }
log_warning() { echo -e "\033[0;33m[WARNING]\033[0m $1"; }
log_error() { echo -e "\033[0;31m[ERROR]\033[0m $1"; }
log_step() { echo -e "\033[0;35m[STEP]\033[0m $1"; }

# Required environment variables
NAMESPACE=${NAMESPACE:-ordering}
ENV_SECRET_NAME=${ENV_SECRET_NAME:-ordering-backend-env}
SERVICE_DB_NAME=${SERVICE_DB_NAME:-ordering}
SERVICE_DB_USER=${SERVICE_DB_USER:-ordering_user}

log_step "Setting up environment secrets for Ordering Backend..."

# Ensure kubectl is available
if ! command -v kubectl &> /dev/null; then
    log_error "kubectl is required"
    exit 1
fi

# Get PostgreSQL password - Use master password for service-specific user
# CRITICAL: Service users now use POSTGRES_PASSWORD (master password)
# Get it from the PostgreSQL secret in infra namespace
if kubectl -n infra get secret postgresql >/dev/null 2>&1; then
    # Get admin password (master password used for all service users)
    EXISTING_PG_PASS=$(kubectl -n infra get secret postgresql -o jsonpath='{.data.admin-user-password}' 2>/dev/null | base64 -d || true)

    if [[ -z "$EXISTING_PG_PASS" ]]; then
        # Fallback to postgres-password if admin-user-password not found
        EXISTING_PG_PASS=$(kubectl -n infra get secret postgresql -o jsonpath='{.data.postgres-password}' 2>/dev/null | base64 -d || true)
    fi

    if [[ -n "$EXISTING_PG_PASS" ]]; then
        log_info "Retrieved PostgreSQL master password from database secret"
        log_info "Using service-specific user: ${SERVICE_DB_USER}"
        APP_DB_PASS="$EXISTING_PG_PASS"
    else
        log_error "Could not retrieve PostgreSQL password from Kubernetes secret"
        exit 1
    fi
else
    log_error "PostgreSQL secret not found in Kubernetes"
    log_error "Ensure PostgreSQL is installed: kubectl get secret postgresql -n infra"
    exit 1
fi

log_info "Database password retrieved and verified (length: ${#APP_DB_PASS} chars)"

# Get Redis password - ALWAYS use the password from the live database
# CRITICAL: The database password is the source of truth
# Get it from the Redis secret (where Helm stores it) in infra namespace
if kubectl -n infra get secret redis >/dev/null 2>&1; then
    REDIS_PASS=$(kubectl -n infra get secret redis -o jsonpath='{.data.redis-password}' 2>/dev/null | base64 -d || true)
    if [[ -n "$REDIS_PASS" ]]; then
        log_info "Retrieved Redis password from database secret (source of truth)"
    else
        log_error "Could not retrieve Redis password from Kubernetes secret"
        exit 1
    fi
else
    log_error "Redis secret not found in Kubernetes"
    log_error "Ensure Redis is installed: kubectl get secret redis -n infra"
    exit 1
fi

log_info "Redis password retrieved and verified (length: ${#REDIS_PASS} chars)"

log_info "Database credentials retrieved: user=${SERVICE_DB_USER}, db=${SERVICE_DB_NAME}"

# Build database URLs
POSTGRES_URL="postgresql://${SERVICE_DB_USER}:${APP_DB_PASS}@postgresql.infra.svc.cluster.local:5432/${SERVICE_DB_NAME}?sslmode=disable"
REDIS_ADDR="redis-master.infra.svc.cluster.local:6379"

# Create or update the secret
log_step "Creating/updating Kubernetes secret..."

# Delete existing secret if it exists (to ensure clean recreation)
kubectl -n "$NAMESPACE" delete secret "$ENV_SECRET_NAME" --ignore-not-found >/dev/null 2>&1

# Create the secret with proper database credentials
kubectl -n "$NAMESPACE" create secret generic "$ENV_SECRET_NAME" \
  --from-literal=ORDERING_APP_ENV="production" \
  --from-literal=ORDERING_POSTGRES_URL="${POSTGRES_URL}" \
  --from-literal=ORDERING_REDIS_ADDR="${REDIS_ADDR}" \
  --from-literal=ORDERING_REDIS_PASSWORD="${REDIS_PASS}" \
  --from-literal=ORDERING_NATS_URL="nats://nats.messaging.svc.cluster.local:4222"

log_success "Environment secret created/updated with production configuration"
log_info "PostgreSQL URL configured for service user: ${SERVICE_DB_USER}"
log_info "Redis configured with password authentication"

# Verify secret was created
kubectl -n "$NAMESPACE" get secret "$ENV_SECRET_NAME" -o jsonpath='{.data.ORDERING_POSTGRES_URL}' | base64 -d | head -c 50 && echo "..."

log_success "Ordering Backend environment secrets configured successfully"