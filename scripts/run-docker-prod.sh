#!/usr/bin/env bash
# Run mcp-banana in Docker production mode (public IP, TLS, OAuth).
# Usage: ./scripts/run-docker-prod.sh
#
# This script:
#   1. Validates .env and MCP_DOMAIN
#   2. Auto-populates OAUTH_BASE_URL, TLS paths, and MCP_AUTH_TOKEN in .env
#   3. Generates TLS certificates if missing (via certbot)
#   4. Validates all required files exist before deployment
#   5. Starts the production Docker stack
#   6. Verifies the server is healthy before reporting success
#
# The script stops immediately if any step fails. Docker will not start
# unless all prerequisites are met.
#
# Compatible with:
#   - Docker Compose V2 plugin (docker compose)
#   - Docker Compose V1 standalone (docker-compose)
#   - Docker 19.03+
#
# Prerequisites:
#   - Docker and Docker Compose installed
#   - .env file with MCP_DOMAIN set
#   - DNS A record for <MCP_DOMAIN> pointing to this server

set -euo pipefail
cd "$(dirname "$0")/.."

echo "=== mcp-banana production deployment ==="
echo ""

# --- Detect docker compose command ---
# Docker Compose V2 (plugin): "docker compose"
# Docker Compose V1 (standalone): "docker-compose"
COMPOSE_CMD=""
if docker compose version &>/dev/null; then
  COMPOSE_CMD="docker compose"
elif command -v docker-compose &>/dev/null; then
  COMPOSE_CMD="docker-compose"
else
  echo "FAILED: Docker Compose is not installed." >&2
  echo "  Docker Compose V2: sudo apt-get install -y docker-compose-plugin" >&2
  echo "  Docker Compose V1: sudo apt-get install -y docker-compose" >&2
  exit 1
fi
echo "[ok] Using: ${COMPOSE_CMD} ($(${COMPOSE_CMD} version --short 2>/dev/null || ${COMPOSE_CMD} --version 2>/dev/null | grep -oP '[\d.]+' | head -1))"

# --- Step 1: Validate .env exists ---
if [ ! -f .env ]; then
  echo "FAILED: .env file not found." >&2
  echo "  Run: cp .env.example .env" >&2
  echo "  Then set MCP_DOMAIN in .env and re-run this script." >&2
  exit 1
fi
echo "[ok] .env file found"

# Load .env
set -a
# shellcheck disable=SC1091
source .env
set +a

# --- Step 2: Validate MCP_DOMAIN ---
DOMAIN="${MCP_DOMAIN:-}"
if [ -z "$DOMAIN" ]; then
  echo "FAILED: MCP_DOMAIN is not set in .env" >&2
  echo "  Example: MCP_DOMAIN=mcp.yourdomain.com" >&2
  exit 1
fi
echo "[ok] MCP_DOMAIN=${DOMAIN}"

# --- Step 3: Validate Docker is installed ---
if ! command -v docker &>/dev/null; then
  echo "FAILED: Docker is not installed." >&2
  echo "  Install: sudo apt-get install -y docker.io" >&2
  exit 1
fi
echo "[ok] Docker installed ($(docker --version | grep -oP '[\d.]+' | head -1))"

# --- Step 4: Auto-populate .env fields ---
UPDATED_ENV=false

if grep -q "^OAUTH_BASE_URL=$" .env 2>/dev/null; then
  sed -i.bak "s|^OAUTH_BASE_URL=$|OAUTH_BASE_URL=https://${DOMAIN}:8847|" .env && rm -f .env.bak
  echo "[auto] OAUTH_BASE_URL set to https://${DOMAIN}:8847"
  UPDATED_ENV=true
fi

if grep -q "^MCP_TLS_CERT_FILE=$" .env 2>/dev/null; then
  sed -i.bak "s|^MCP_TLS_CERT_FILE=$|MCP_TLS_CERT_FILE=/etc/letsencrypt/live/${DOMAIN}/fullchain.pem|" .env && rm -f .env.bak
  sed -i.bak "s|^MCP_TLS_KEY_FILE=$|MCP_TLS_KEY_FILE=/etc/letsencrypt/live/${DOMAIN}/privkey.pem|" .env && rm -f .env.bak
  echo "[auto] MCP_TLS_CERT_FILE and MCP_TLS_KEY_FILE set to /etc/letsencrypt/live/${DOMAIN}/ paths"
  UPDATED_ENV=true
fi

if grep -q "^MCP_AUTH_TOKEN=$" .env 2>/dev/null; then
  GENERATED_TOKEN=$(openssl rand -hex 32)
  sed -i.bak "s|^MCP_AUTH_TOKEN=$|MCP_AUTH_TOKEN=${GENERATED_TOKEN}|" .env && rm -f .env.bak
  echo "[auto] MCP_AUTH_TOKEN generated: ${GENERATED_TOKEN}"
  echo "       Save this token — you need it for your Claude Code MCP config."
  UPDATED_ENV=true
fi

if [ "$UPDATED_ENV" = true ]; then
  # Re-source .env after updates
  set -a
  # shellcheck disable=SC1091
  source .env
  set +a
fi

if ! grep -q "^GEMINI_API_KEY=.\+" .env 2>/dev/null; then
  echo "[note] GEMINI_API_KEY is not set — clients must send X-Gemini-API-Key header"
fi

# --- Step 5: Check and generate TLS certificates ---
# Note: /etc/letsencrypt is owned by root (mode 700), so all checks use sudo.
CERT_DIR="/etc/letsencrypt/live/${DOMAIN}"
if ! sudo test -d "$CERT_DIR"; then
  echo ""
  echo "TLS certificates not found at ${CERT_DIR}"
  echo ""

  # Check if certbot is installed, install if not
  if ! command -v certbot &>/dev/null; then
    echo "Installing certbot..."
    if command -v apt-get &>/dev/null; then
      sudo apt-get update -qq && sudo apt-get install -y -qq certbot
    elif command -v yum &>/dev/null; then
      sudo yum install -y certbot
    elif command -v brew &>/dev/null; then
      brew install certbot
    else
      echo "FAILED: Cannot install certbot automatically." >&2
      echo "  Install manually: https://certbot.eff.org/instructions" >&2
      exit 1
    fi
    echo "[ok] certbot installed"
  fi

  echo "Generating TLS certificate for ${DOMAIN}..."
  echo ""
  echo "Certbot will ask you to create a DNS TXT record."
  echo "Add it in your domain registrar, wait 1-2 minutes, then press Enter."
  echo ""

  sudo certbot certonly --manual --preferred-challenges dns -d "${DOMAIN}"

  # Verify certs were created
  if ! sudo test -d "$CERT_DIR"; then
    echo "FAILED: Certificate generation failed. ${CERT_DIR} not found." >&2
    echo "  Fix the issue and re-run this script." >&2
    exit 1
  fi

  echo "[ok] TLS certificates generated at ${CERT_DIR}"
else
  echo "[ok] TLS certificates found at ${CERT_DIR}"
fi

# --- Step 6: Validate cert files exist ---
FULLCHAIN="${CERT_DIR}/fullchain.pem"
PRIVKEY="${CERT_DIR}/privkey.pem"
if ! sudo test -f "$FULLCHAIN"; then
  echo "FAILED: ${FULLCHAIN} not found." >&2
  exit 1
fi
if ! sudo test -f "$PRIVKEY"; then
  echo "FAILED: ${PRIVKEY} not found." >&2
  exit 1
fi
echo "[ok] fullchain.pem and privkey.pem exist"

# --- Step 7: Stop any existing container on the same port ---
echo ""
# Stop dev or previous prod container to free port 8847
${COMPOSE_CMD} down 2>/dev/null || true
${COMPOSE_CMD} -f docker-compose.yml -f docker-compose.prod.yml down 2>/dev/null || true

# --- Step 8: Build and start Docker ---
echo "Building and starting mcp-banana (production, 0.0.0.0:8847, ${DOMAIN})..."
export MCP_BIND_ADDRESS=0.0.0.0
${COMPOSE_CMD} -f docker-compose.yml -f docker-compose.prod.yml up -d --build

# --- Step 8: Wait for health check ---
echo ""
echo "Waiting for server to become healthy..."
MAX_RETRIES=15
RETRY_INTERVAL=2
for attempt in $(seq 1 $MAX_RETRIES); do
  if curl -sk "https://${DOMAIN}:8847/healthz" 2>/dev/null | grep -q '"status":"ok"'; then
    echo "[ok] Server is healthy"
    break
  fi
  if [ "$attempt" -eq "$MAX_RETRIES" ]; then
    echo "FAILED: Server did not become healthy after $((MAX_RETRIES * RETRY_INTERVAL)) seconds." >&2
    echo "  Check logs: ${COMPOSE_CMD} -f docker-compose.yml -f docker-compose.prod.yml logs" >&2
    exit 1
  fi
  sleep $RETRY_INTERVAL
done

# --- Done ---
echo ""
echo "=== Deployment successful ==="
echo ""
echo "  Server:       https://${DOMAIN}:8847"
echo "  Health check: curl -k https://${DOMAIN}:8847/healthz"
echo "  Logs:         ${COMPOSE_CMD} -f docker-compose.yml -f docker-compose.prod.yml logs -f"
echo "  Stop:         ${COMPOSE_CMD} -f docker-compose.yml -f docker-compose.prod.yml down"
