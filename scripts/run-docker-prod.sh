#!/usr/bin/env bash
# Run mcp-banana in Docker production mode (public IP, TLS, OAuth).
# Usage: ./scripts/run-docker-prod.sh
#
# This script:
#   1. Reads MCP_DOMAIN from .env
#   2. Auto-populates OAUTH_BASE_URL and TLS paths in .env
#   3. Checks for TLS certificates — offers to generate them if missing
#   4. Starts the production Docker stack
#
# Prerequisites:
#   - Docker and Docker Compose installed
#   - .env file with MCP_DOMAIN set
#   - DNS A record for <MCP_DOMAIN> pointing to this server
#
# The server binds to 0.0.0.0:8847 (all interfaces) with TLS.

set -euo pipefail
cd "$(dirname "$0")/.."

if [ ! -f .env ]; then
  echo "ERROR: .env file not found. Copy .env.example and configure it:" >&2
  echo "  cp .env.example .env" >&2
  exit 1
fi

# Load .env to read MCP_DOMAIN
set -a
# shellcheck disable=SC1091
source .env
set +a

DOMAIN="${MCP_DOMAIN:-}"
if [ -z "$DOMAIN" ]; then
  echo "ERROR: MCP_DOMAIN is not set in .env" >&2
  echo "  Example: MCP_DOMAIN=mcp.yourdomain.com" >&2
  exit 1
fi

# Auto-update OAUTH_BASE_URL in .env from MCP_DOMAIN if empty
if grep -q "^OAUTH_BASE_URL=$" .env 2>/dev/null; then
  sed -i.bak "s|^OAUTH_BASE_URL=$|OAUTH_BASE_URL=https://${DOMAIN}:8847|" .env && rm -f .env.bak
  echo "OAUTH_BASE_URL set to https://${DOMAIN}:8847 in .env"
fi

# Auto-update MCP_TLS_CERT_FILE and MCP_TLS_KEY_FILE in .env if empty
if grep -q "^MCP_TLS_CERT_FILE=$" .env 2>/dev/null; then
  sed -i.bak "s|^MCP_TLS_CERT_FILE=$|MCP_TLS_CERT_FILE=/certs/fullchain.pem|" .env && rm -f .env.bak
  sed -i.bak "s|^MCP_TLS_KEY_FILE=$|MCP_TLS_KEY_FILE=/certs/privkey.pem|" .env && rm -f .env.bak
  echo "MCP_TLS_CERT_FILE and MCP_TLS_KEY_FILE set to /certs/ paths in .env"
fi

# Re-source .env after updates
set -a
# shellcheck disable=SC1091
source .env
set +a

if ! grep -q "^GEMINI_API_KEY=.\+" .env 2>/dev/null; then
  echo "NOTE: GEMINI_API_KEY is not set in .env. Clients must provide their own key via X-Gemini-API-Key header." >&2
fi

# Auto-generate MCP_AUTH_TOKEN if empty
if grep -q "^MCP_AUTH_TOKEN=$" .env 2>/dev/null; then
  GENERATED_TOKEN=$(openssl rand -hex 32)
  sed -i.bak "s|^MCP_AUTH_TOKEN=$|MCP_AUTH_TOKEN=${GENERATED_TOKEN}|" .env && rm -f .env.bak
  echo "MCP_AUTH_TOKEN auto-generated and saved to .env"
  echo "  Token: ${GENERATED_TOKEN}"
  echo "  Use this token in your Claude Code MCP config."
fi

# Check and generate TLS certificates
CERT_DIR="/etc/letsencrypt/live/${DOMAIN}"
if [ ! -d "$CERT_DIR" ]; then
  echo "" >&2
  echo "TLS certificates not found at ${CERT_DIR}" >&2
  echo "" >&2

  # Check if certbot is installed
  if ! command -v certbot &>/dev/null; then
    echo "certbot is not installed. Installing..." >&2
    if command -v apt-get &>/dev/null; then
      sudo apt-get update -qq && sudo apt-get install -y -qq certbot
    elif command -v yum &>/dev/null; then
      sudo yum install -y certbot
    elif command -v brew &>/dev/null; then
      brew install certbot
    else
      echo "ERROR: Cannot install certbot automatically. Install it manually:" >&2
      echo "  https://certbot.eff.org/instructions" >&2
      exit 1
    fi
  fi

  echo "Generating TLS certificate for ${DOMAIN}..." >&2
  echo "" >&2
  echo "Certbot will ask you to create a DNS TXT record." >&2
  echo "Add it in your domain registrar, wait 1-2 minutes, then press Enter." >&2
  echo "" >&2

  sudo certbot certonly --manual --preferred-challenges dns -d "${DOMAIN}"

  # Verify certs were created
  if [ ! -d "$CERT_DIR" ]; then
    echo "ERROR: Certificate generation failed. ${CERT_DIR} not found." >&2
    echo "  Fix the issue and re-run this script." >&2
    exit 1
  fi

  echo "" >&2
  echo "TLS certificates generated successfully at ${CERT_DIR}" >&2
fi

echo ""
echo "Building and starting mcp-banana (production mode, 0.0.0.0:8847, domain: ${DOMAIN})..."
docker compose -f docker-compose.yml -f docker-compose.prod.yml up -d --build

echo ""
echo "Server running at https://${DOMAIN}:8847"
echo "Health check: curl -k https://${DOMAIN}:8847/healthz"
echo "Logs: docker compose logs -f"
echo "Stop: docker compose -f docker-compose.yml -f docker-compose.prod.yml down"
