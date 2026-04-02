#!/usr/bin/env bash
#
# Fibratus Fleet Server — Quick Start with Docker
#
# Requirements: Docker + Docker Compose
#
# Usage:
#   curl -sSL https://raw.githubusercontent.com/NovaSky0x1/fibratus/master/deploy/quickstart.sh | bash
#
# Or locally:
#   bash deploy/quickstart.sh
#
set -euo pipefail

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
BOLD='\033[1m'
NC='\033[0m'

info()  { echo -e "${BLUE}[INFO]${NC}  $*"; }
ok()    { echo -e "${GREEN}[OK]${NC}    $*"; }
err()   { echo -e "${RED}[ERROR]${NC} $*"; exit 1; }

echo ""
echo -e "${BOLD}Fibratus Fleet Server — Docker Quick Start${NC}"
echo ""

# Check Docker
command -v docker &>/dev/null || err "Docker is not installed. Install it first: https://docs.docker.com/get-docker/"
docker compose version &>/dev/null || err "Docker Compose (v2) is required. Update Docker or install compose plugin."

# Generate API key
API_KEY="$(openssl rand -hex 32 2>/dev/null || head -c 64 /dev/urandom | od -A n -t x1 | tr -d ' \n')"

# Clone if not in repo
if [[ ! -f "deploy/docker-compose.yml" ]]; then
    info "Cloning repository..."
    TMPDIR="$(mktemp -d)"
    git clone --depth 1 https://github.com/NovaSky0x1/fibratus.git "$TMPDIR/fibratus"
    cd "$TMPDIR/fibratus"
else
    cd "$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
fi

# Write config with generated API key
info "Generating configuration with fresh API key..."
cat > deploy/fleet-server-config.yml <<YAML
server:
  listen: ":8443"

database:
  host: postgres
  port: 5432
  name: fibratus_fleet
  user: fibratus
  password: "$(openssl rand -hex 16)"
  ssl-mode: disable
  max-connections: 50

elasticsearch:
  servers:
    - http://elasticsearch:9200
  index-prefix: fibratus
  bulk-workers: 2
  flush-period: 1s

auth:
  api-keys:
    - name: "default"
      key: "${API_KEY}"

agent:
  heartbeat-timeout: 90s
  offline-threshold: 5m

logging:
  level: info

dashboard:
  enabled: true
YAML

# Also update the docker-compose postgres password to match
DB_PASS=$(grep 'password:' deploy/fleet-server-config.yml | head -1 | awk -F'"' '{print $2}')
sed -i "s/fibratus-fleet-db-pass/${DB_PASS}/g" deploy/docker-compose.yml

# Build and start
info "Building and starting containers (this takes 2-5 minutes on first run)..."
cd deploy
docker compose up -d --build

# Wait for health
info "Waiting for server to be ready..."
for i in $(seq 1 30); do
    if curl -sf http://localhost:8443/health > /dev/null 2>&1; then
        break
    fi
    sleep 2
done

SERVER_IP=$(hostname -I 2>/dev/null | awk '{print $1}' || echo "localhost")

echo ""
echo -e "${BOLD}╔══════════════════════════════════════════════════════════════╗${NC}"
echo -e "${BOLD}║           Fibratus Fleet Server — Running!                   ║${NC}"
echo -e "${BOLD}╚══════════════════════════════════════════════════════════════╝${NC}"
echo ""
echo -e "  ${BOLD}Dashboard:${NC}   ${GREEN}http://${SERVER_IP}:8443${NC}"
echo -e "  ${BOLD}API:${NC}         http://${SERVER_IP}:8443/api/v1/"
echo -e "  ${BOLD}Health:${NC}      http://${SERVER_IP}:8443/health"
echo ""
echo -e "  ${BOLD}API Key:${NC}     ${YELLOW}${API_KEY}${NC}"
echo ""
echo -e "  ${BOLD}Agent config (add to fibratus.yml on each Windows endpoint):${NC}"
echo ""
echo -e "    ${YELLOW}fleet:${NC}"
echo -e "    ${YELLOW}  enabled: true${NC}"
echo -e "    ${YELLOW}  server-url: \"http://${SERVER_IP}:8443\"${NC}"
echo -e "    ${YELLOW}  api-key: \"${API_KEY}\"${NC}"
echo -e "    ${YELLOW}  agent-group: \"default\"${NC}"
echo ""
echo -e "  ${BOLD}Manage:${NC}"
echo -e "    docker compose -f deploy/docker-compose.yml logs -f"
echo -e "    docker compose -f deploy/docker-compose.yml down"
echo -e "    docker compose -f deploy/docker-compose.yml up -d"
echo ""
