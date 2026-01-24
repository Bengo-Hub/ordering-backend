#!/usr/bin/env bash
# =============================================================================
# Update devops-k8s values.yaml with current image tag
# =============================================================================
# This script automates the manual step of updating the image tag in
# devops-k8s/apps/ordering-backend/values.yaml
# 
# Usage: ./update-devops-values.sh [optional-git-token]
# 
# When run in CI/CD, this is part of build.sh
# When run locally, this can be used to update values without building image
# =============================================================================

set -euo pipefail

BLUE='\033[0;34m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

info() { echo -e "${BLUE}[INFO]${NC} $1"; }
success() { echo -e "${GREEN}[SUCCESS]${NC} $1"; }
warn() { echo -e "${YELLOW}[WARN]${NC} $1"; }
error() { echo -e "${RED}[ERROR]${NC} $1"; }

APP_NAME="ordering-backend"
DEVOPS_REPO=${DEVOPS_REPO:-"Bengo-Hub/devops-k8s"}
DEVOPS_DIR=${DEVOPS_DIR:-"${HOME}/devops-k8s"}
VALUES_FILE_PATH=${VALUES_FILE_PATH:-"apps/${APP_NAME}/values.yaml"}
GIT_EMAIL=${GIT_EMAIL:-"dev@bengobox.com"}
GIT_USER=${GIT_USER:-"Ordering Bot"}
REGISTRY_SERVER=${REGISTRY_SERVER:-docker.io}
REGISTRY_NAMESPACE=${REGISTRY_NAMESPACE:-codevertex}
IMAGE_REPO="${REGISTRY_SERVER}/${REGISTRY_NAMESPACE}/${APP_NAME}"

# Get current commit hash
if [[ -z ${GITHUB_SHA:-} ]]; then
  GIT_COMMIT_ID=$(git rev-parse --short=8 HEAD || echo "localbuild")
else
  GIT_COMMIT_ID=${GITHUB_SHA::8}
fi

info "Updating devops-k8s values.yaml"
info "App: ${APP_NAME}"
info "Current commit: ${GIT_COMMIT_ID}"
info "Image: ${IMAGE_REPO}:${GIT_COMMIT_ID}"

# Check for yq
if ! command -v yq &>/dev/null; then
  error "yq is required but not found. Install with: brew install yq or apt-get install yq"
  exit 1
fi

# Check for git
if ! command -v git &>/dev/null; then
  error "git is required"
  exit 1
fi

# Clone or update devops-k8s repo
TOKEN="${1:-${GH_PAT:-${GIT_SECRET:-${GITHUB_TOKEN:-}}}}"
CLONE_URL="https://github.com/${DEVOPS_REPO}.git"
[[ -n $TOKEN ]] && CLONE_URL="https://x-access-token:${TOKEN}@github.com/${DEVOPS_REPO}.git"

if [[ ! -d $DEVOPS_DIR ]]; then
  info "Cloning devops-k8s repository..."
  git clone "$CLONE_URL" "$DEVOPS_DIR" || { 
    error "Unable to clone devops repo from $CLONE_URL"
    error "You may need to provide a GitHub token: ./update-devops-values.sh <gh-token>"
    exit 1
  }
fi

pushd "$DEVOPS_DIR" >/dev/null || exit 1
info "Working in: $DEVOPS_DIR"

git config user.email "$GIT_EMAIL"
git config user.name "$GIT_USER"
git fetch origin main || true
git checkout main || git checkout -b main || true
git reset --hard origin/main || true

if [[ ! -f "$VALUES_FILE_PATH" ]]; then
  error "${VALUES_FILE_PATH} not found"
  popd >/dev/null || true
  exit 1
fi

info "Updating $VALUES_FILE_PATH with image tag: ${GIT_COMMIT_ID}"
IMAGE_REPO_ENV="$IMAGE_REPO" IMAGE_TAG_ENV="$GIT_COMMIT_ID" \
  yq e -i '.image.repository = strenv(IMAGE_REPO_ENV) | .image.tag = strenv(IMAGE_TAG_ENV)' "$VALUES_FILE_PATH"

# Verify the update
UPDATED_TAG=$(yq e '.image.tag' "$VALUES_FILE_PATH")
if [[ "$UPDATED_TAG" != "$GIT_COMMIT_ID" ]]; then
  error "Failed to update image tag. Expected ${GIT_COMMIT_ID}, got ${UPDATED_TAG}"
  popd >/dev/null || true
  exit 1
fi

success "Image tag updated to: ${UPDATED_TAG}"

git add "$VALUES_FILE_PATH"
if git diff --cached --quiet; then
  warn "No changes to commit - tag is already ${GIT_COMMIT_ID}"
  popd >/dev/null || true
  exit 0
fi

git commit -m "${APP_NAME}:${GIT_COMMIT_ID} released" || {
  warn "Commit failed - changes may already be committed"
}

if [[ -n $TOKEN ]]; then
  info "Pushing changes to origin/main..."
  git push origin HEAD:main || {
    error "Push failed - check your GitHub token permissions"
    popd >/dev/null || true
    exit 1
  }
  success "Changes pushed to origin/main"
else
  warn "No GitHub token provided - not pushing changes"
  warn "To push automatically, provide token: ./update-devops-values.sh <gh-token>"
  warn "Or manually push from: $DEVOPS_DIR"
fi

popd >/dev/null || true

info "=========================================="
success "Completed! devops-k8s values.yaml updated"
info "Image: ${IMAGE_REPO}:${GIT_COMMIT_ID}"
info "=========================================="
