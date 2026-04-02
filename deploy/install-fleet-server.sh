#!/usr/bin/env bash
#
# Fibratus Fleet Server — One-liner installer for Ubuntu 22.04/24.04
#
# Usage:
#   curl -sSL https://raw.githubusercontent.com/NovaSky0x1/fibratus/master/deploy/install-fleet-server.sh | sudo bash
#
# Or run locally:
#   sudo bash deploy/install-fleet-server.sh
#
# What this does:
#   1. Installs Go, Node.js 20, PostgreSQL 16
#   2. Creates the database and user
#   3. Builds the dashboard (React)
#   4. Builds the fleet-server binary
#   5. Generates a random API key
#   6. Runs database migrations
#   7. Installs as a systemd service
#   8. Opens firewall port 8443
#
set -euo pipefail

# ─── Configuration ───────────────────────────────────────────────────────────

INSTALL_DIR="/opt/fibratus-fleet"
CONFIG_DIR="/etc/fibratus"
DATA_DIR="/var/lib/fibratus-fleet"
LOG_DIR="/var/log/fibratus-fleet"
SERVICE_USER="fibratus"
LISTEN_PORT="8443"

DB_NAME="fibratus_fleet"
DB_USER="fibratus"
DB_PASS="$(openssl rand -hex 16)"
API_KEY="$(openssl rand -hex 32)"

GO_VERSION="1.26.1"
NODE_MAJOR="20"

REPO_URL="https://github.com/NovaSky0x1/fibratus.git"
REPO_BRANCH="${FIBRATUS_BRANCH:-feat/fleet-server}"

# ─── Colors ──────────────────────────────────────────────────────────────────

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
BOLD='\033[1m'
NC='\033[0m'

info()  { echo -e "${BLUE}[INFO]${NC}  $*"; }
ok()    { echo -e "${GREEN}[OK]${NC}    $*"; }
warn()  { echo -e "${YELLOW}[WARN]${NC}  $*"; }
err()   { echo -e "${RED}[ERROR]${NC} $*"; exit 1; }

# ─── Pre-flight checks ──────────────────────────────────────────────────────

if [[ $EUID -ne 0 ]]; then
    err "This script must be run as root (use sudo)"
fi

if ! grep -qiE 'ubuntu|debian' /etc/os-release 2>/dev/null; then
    warn "This script is designed for Ubuntu/Debian. Proceed with caution."
fi

echo ""
echo -e "${BOLD}╔══════════════════════════════════════════════════╗${NC}"
echo -e "${BOLD}║     Fibratus Fleet Server — Installer            ║${NC}"
echo -e "${BOLD}╚══════════════════════════════════════════════════╝${NC}"
echo ""

# ─── Step 1: System packages ────────────────────────────────────────────────

info "Updating system packages..."
export DEBIAN_FRONTEND=noninteractive
apt-get update -qq
apt-get install -y -qq git curl wget build-essential ca-certificates gnupg lsb-release ufw > /dev/null 2>&1
ok "System packages installed"

# ─── Step 2: Install Go ─────────────────────────────────────────────────────

if command -v go &>/dev/null && go version | grep -q "go${GO_VERSION}"; then
    ok "Go ${GO_VERSION} already installed"
else
    info "Installing Go ${GO_VERSION}..."
    wget -q "https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz" -O /tmp/go.tar.gz
    rm -rf /usr/local/go
    tar -C /usr/local -xzf /tmp/go.tar.gz
    rm /tmp/go.tar.gz
    ok "Go ${GO_VERSION} installed"
fi

export PATH="/usr/local/go/bin:$PATH"
export GOPATH="/tmp/gopath"
export GOCACHE="/tmp/gocache"

# ─── Step 3: Install Node.js ────────────────────────────────────────────────

if command -v node &>/dev/null && node --version | grep -qE "^v${NODE_MAJOR}"; then
    ok "Node.js ${NODE_MAJOR} already installed"
else
    info "Installing Node.js ${NODE_MAJOR}..."
    mkdir -p /etc/apt/keyrings
    curl -fsSL https://deb.nodesource.com/gpgkey/nodesource-repo.gpg.key | gpg --dearmor -o /etc/apt/keyrings/nodesource.gpg 2>/dev/null
    echo "deb [signed-by=/etc/apt/keyrings/nodesource.gpg] https://deb.nodesource.com/node_${NODE_MAJOR}.x nodistro main" > /etc/apt/sources.list.d/nodesource.list
    apt-get update -qq
    apt-get install -y -qq nodejs > /dev/null 2>&1
    ok "Node.js $(node --version) installed"
fi

# ─── Step 4: Install PostgreSQL ──────────────────────────────────────────────

if command -v psql &>/dev/null; then
    ok "PostgreSQL already installed"
else
    info "Installing PostgreSQL..."
    apt-get install -y -qq postgresql postgresql-contrib > /dev/null 2>&1
    systemctl enable postgresql
    systemctl start postgresql
    ok "PostgreSQL installed and running"
fi

# Ensure PostgreSQL is running
systemctl start postgresql 2>/dev/null || true

# Create database and user
info "Setting up database..."
sudo -u postgres psql -tc "SELECT 1 FROM pg_roles WHERE rolname='${DB_USER}'" | grep -q 1 || \
    sudo -u postgres psql -c "CREATE USER ${DB_USER} WITH PASSWORD '${DB_PASS}';" 2>/dev/null
sudo -u postgres psql -tc "SELECT 1 FROM pg_database WHERE datname='${DB_NAME}'" | grep -q 1 || \
    sudo -u postgres psql -c "CREATE DATABASE ${DB_NAME} OWNER ${DB_USER};" 2>/dev/null
sudo -u postgres psql -c "GRANT ALL PRIVILEGES ON DATABASE ${DB_NAME} TO ${DB_USER};" 2>/dev/null
ok "Database '${DB_NAME}' ready"

# ─── Step 5: Clone / update repo ────────────────────────────────────────────

if [[ -d "${INSTALL_DIR}/src" ]]; then
    info "Updating existing source..."
    cd "${INSTALL_DIR}/src"
    git pull --ff-only origin ${REPO_BRANCH} 2>/dev/null || true
else
    info "Cloning repository..."
    mkdir -p "${INSTALL_DIR}"
    git clone --depth 1 --branch ${REPO_BRANCH} "${REPO_URL}" "${INSTALL_DIR}/src"
fi
cd "${INSTALL_DIR}/src"
ok "Source code ready at ${INSTALL_DIR}/src"

# ─── Step 6: Build dashboard ────────────────────────────────────────────────

info "Building dashboard (React + Vite)..."
cd "${INSTALL_DIR}/src/web/dashboard"
npm install --silent
npm run build
ok "Dashboard built"

# ─── Step 7: Build fleet-server binary ───────────────────────────────────────

info "Building fleet-server binary..."
cd "${INSTALL_DIR}/src"
mkdir -p "${INSTALL_DIR}/bin"

# Fetch dependencies
go mod tidy 2>/dev/null || go mod download

CGO_ENABLED=0 go build \
    -ldflags="-s -w -X github.com/rabbitstack/fibratus/cmd/fleet-server/app.version=$(git describe --tags 2>/dev/null || echo dev) -X github.com/rabbitstack/fibratus/cmd/fleet-server/app.commit=$(git rev-parse --short HEAD 2>/dev/null || echo unknown)" \
    -o "${INSTALL_DIR}/bin/fleet-server" \
    ./cmd/fleet-server/
chmod +x "${INSTALL_DIR}/bin/fleet-server"
ok "Binary built at ${INSTALL_DIR}/bin/fleet-server"

# ─── Step 8: Create service user ────────────────────────────────────────────

if id "${SERVICE_USER}" &>/dev/null; then
    ok "Service user '${SERVICE_USER}' exists"
else
    useradd --system --no-create-home --shell /usr/sbin/nologin "${SERVICE_USER}"
    ok "Service user '${SERVICE_USER}' created"
fi

# ─── Step 9: Write configuration ────────────────────────────────────────────

mkdir -p "${CONFIG_DIR}" "${CONFIG_DIR}/tls" "${DATA_DIR}" "${LOG_DIR}"
chown "${SERVICE_USER}:${SERVICE_USER}" "${DATA_DIR}" "${LOG_DIR}"

# Generate self-signed TLS certificate for the server
info "Generating server TLS certificate..."
SERVER_IP=$(hostname -I | awk '{print $1}')
openssl req -x509 -newkey rsa:4096 -sha256 -days 3650 -nodes \
    -keyout "${CONFIG_DIR}/tls/server.key" \
    -out "${CONFIG_DIR}/tls/server.crt" \
    -subj "/CN=fibratus-fleet/O=Fibratus" \
    -addext "subjectAltName=DNS:localhost,IP:127.0.0.1,IP:${SERVER_IP}" 2>/dev/null
chmod 600 "${CONFIG_DIR}/tls/server.key"
chown -R "${SERVICE_USER}:${SERVICE_USER}" "${CONFIG_DIR}/tls"
ok "TLS certificate generated"

JWT_SECRET="$(openssl rand -hex 32)"

cat > "${CONFIG_DIR}/fleet-server.yml" <<YAML
server:
  listen: ":${LISTEN_PORT}"
  tls-cert: ${CONFIG_DIR}/tls/server.crt
  tls-key: ${CONFIG_DIR}/tls/server.key

database:
  host: localhost
  port: 5432
  name: ${DB_NAME}
  user: ${DB_USER}
  password: "${DB_PASS}"
  ssl-mode: disable
  max-connections: 50

elasticsearch:
  servers:
    - http://localhost:9200
  index-prefix: fibratus
  bulk-workers: 2
  flush-period: 1s

auth:
  jwt-secret: "${JWT_SECRET}"
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

chmod 600 "${CONFIG_DIR}/fleet-server.yml"
chown "${SERVICE_USER}:${SERVICE_USER}" "${CONFIG_DIR}/fleet-server.yml"
ok "Configuration written to ${CONFIG_DIR}/fleet-server.yml"

# ─── Step 10: Run migrations + bootstrap ────────────────────────────────────

info "Running database migrations..."
"${INSTALL_DIR}/bin/fleet-server" migrate --config "${CONFIG_DIR}/fleet-server.yml"
ok "Database schema created"

ADMIN_PASS="$(openssl rand -hex 8)"
info "Bootstrapping initial account and organization..."
BOOTSTRAP_OUTPUT=$("${INSTALL_DIR}/bin/fleet-server" bootstrap \
    --config "${CONFIG_DIR}/fleet-server.yml" \
    --account "Default" \
    --org "Production" \
    --email "admin@fibratus.local" \
    --name "Admin" \
    --password "${ADMIN_PASS}" 2>&1)
ORG_ID=$(echo "${BOOTSTRAP_OUTPUT}" | grep "Org ID:" | awk '{print $NF}')
ENROLL_TOKEN=$(echo "${BOOTSTRAP_OUTPUT}" | grep "Enrollment Token:" | awk '{print $NF}')
ok "Bootstrap complete"

# ─── Step 11: Install systemd service ────────────────────────────────────────

cat > /etc/systemd/system/fibratus-fleet.service <<SERVICE
[Unit]
Description=Fibratus Fleet Management Server
Documentation=https://www.fibratus.io
After=network-online.target postgresql.service
Wants=network-online.target
Requires=postgresql.service

[Service]
Type=simple
User=${SERVICE_USER}
Group=${SERVICE_USER}
ExecStart=${INSTALL_DIR}/bin/fleet-server serve --config ${CONFIG_DIR}/fleet-server.yml
Restart=always
RestartSec=5
LimitNOFILE=65536

StandardOutput=append:${LOG_DIR}/fleet-server.log
StandardError=append:${LOG_DIR}/fleet-server.log

# Hardening
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=${DATA_DIR} ${LOG_DIR}
PrivateTmp=true

[Install]
WantedBy=multi-user.target
SERVICE

systemctl daemon-reload
systemctl enable fibratus-fleet
systemctl start fibratus-fleet
ok "Systemd service installed and started"

# ─── Step 12: Firewall ──────────────────────────────────────────────────────

if command -v ufw &>/dev/null; then
    ufw allow ${LISTEN_PORT}/tcp comment "Fibratus Fleet Server" 2>/dev/null || true
    ok "Firewall port ${LISTEN_PORT} opened"
fi

# ─── Done ────────────────────────────────────────────────────────────────────

sleep 2

# Check if server is running
if systemctl is-active --quiet fibratus-fleet; then
    STATUS="${GREEN}RUNNING${NC}"
else
    STATUS="${RED}NOT RUNNING${NC} (check: journalctl -u fibratus-fleet)"
fi

SERVER_IP=$(hostname -I | awk '{print $1}')

echo ""
echo -e "${BOLD}╔══════════════════════════════════════════════════════════════╗${NC}"
echo -e "${BOLD}║              Fibratus Fleet Server — Installed!              ║${NC}"
echo -e "${BOLD}╚══════════════════════════════════════════════════════════════╝${NC}"
echo ""
echo -e "  ${BOLD}Status:${NC}      ${STATUS}"
echo -e "  ${BOLD}Dashboard:${NC}   https://${SERVER_IP}:${LISTEN_PORT}"
echo -e "  ${BOLD}API:${NC}         https://${SERVER_IP}:${LISTEN_PORT}/api/v1/"
echo -e "  ${BOLD}Health:${NC}      https://${SERVER_IP}:${LISTEN_PORT}/health"
echo ""
echo ""
echo -e "  ${BOLD}Dashboard:${NC}"
echo -e "    URL:      https://${SERVER_IP}:${LISTEN_PORT}"
echo -e "    Email:    admin@fibratus.local"
echo -e "    Password: ${YELLOW}${ADMIN_PASS}${NC}"
echo ""
echo -e "  ─────────────────────────────────────────────────────────────"
echo ""
echo -e "  ${BOLD}${GREEN}Enroll agents — run this on each Windows endpoint:${NC}"
echo ""
echo -e "    ${YELLOW}fibratus enroll \\${NC}"
echo -e "    ${YELLOW}  --token ${ENROLL_TOKEN} \\${NC}"
echo -e "    ${YELLOW}  --server https://${SERVER_IP}:${LISTEN_PORT} \\${NC}"
echo -e "    ${YELLOW}  --insecure${NC}"
echo ""
echo -e "  Then start the agent:"
echo -e "    ${YELLOW}fibratus service start${NC}"
echo ""
echo -e "  ─────────────────────────────────────────────────────────────"
echo ""
echo -e "  ${BOLD}Enrollment Token:${NC}  ${ENROLL_TOKEN}"
echo -e "  ${BOLD}Org ID:${NC}            ${ORG_ID}"
echo -e "  ${BOLD}Config:${NC}            ${CONFIG_DIR}/fleet-server.yml"
echo -e "  ${BOLD}Logs:${NC}              journalctl -u fibratus-fleet -f"
echo ""
echo -e "  The enrollment token is valid for 1 year / 1000 agents."
echo -e "  Create more tokens in the dashboard under Settings."
echo ""
echo -e "  ${BOLD}Note:${NC} --insecure is needed because the server uses a self-signed"
echo -e "  TLS certificate. For production, replace with a CA-signed cert"
echo -e "  and remove the flag."
echo ""
