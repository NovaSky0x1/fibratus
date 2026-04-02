#!/usr/bin/env bash
#
# Fibratus Fleet Server — One-liner installer for Ubuntu 22.04/24.04
#
# Usage:
#   sudo bash install-fleet-server.sh <domain>                  # Let's Encrypt (recommended)
#   sudo bash install-fleet-server.sh <domain> --self-signed    # Self-signed cert
#   sudo bash install-fleet-server.sh <domain> --cert /path/to/cert.pem --key /path/to/key.pem  # Own cert
#
# Examples:
#   sudo bash install-fleet-server.sh fleet.acme.com
#   sudo bash install-fleet-server.sh fleet.acme.com --self-signed
#   sudo bash install-fleet-server.sh fleet.acme.com --cert /etc/ssl/fleet.crt --key /etc/ssl/fleet.key
#
set -euo pipefail

# ─── Parse arguments ─────────────────────────────────────────────────────────

FLEET_DOMAIN=""
TLS_MODE="letsencrypt"   # letsencrypt | selfsigned | custom
CUSTOM_CERT=""
CUSTOM_KEY=""

while [[ $# -gt 0 ]]; do
    case "$1" in
        --self-signed)  TLS_MODE="selfsigned"; shift ;;
        --cert)         TLS_MODE="custom"; CUSTOM_CERT="$2"; shift 2 ;;
        --key)          CUSTOM_KEY="$2"; shift 2 ;;
        --help|-h)
            echo "Usage: sudo bash install-fleet-server.sh <domain> [options]"
            echo ""
            echo "Options:"
            echo "  --self-signed           Use a self-signed certificate"
            echo "  --cert <path> --key <path>  Use your own certificate files"
            echo ""
            echo "Default: Let's Encrypt (free, trusted, auto-renewing)"
            exit 0
            ;;
        -*)             echo "Unknown option: $1"; exit 1 ;;
        *)              FLEET_DOMAIN="$1"; shift ;;
    esac
done

FLEET_DOMAIN="${FLEET_DOMAIN:-${FLEET_DOMAIN_ENV:-}}"

# ─── Configuration ───────────────────────────────────────────────────────────

INSTALL_DIR="/opt/fibratus-fleet"
CONFIG_DIR="/etc/fibratus"
DATA_DIR="/var/lib/fibratus-fleet"
LOG_DIR="/var/log/fibratus-fleet"
SERVICE_USER="fibratus"
LISTEN_PORT="443"

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

if [[ -z "${FLEET_DOMAIN}" ]]; then
    echo ""
    echo -e "${BOLD}╔══════════════════════════════════════════════════╗${NC}"
    echo -e "${BOLD}║     Fibratus Fleet Server — Installer            ║${NC}"
    echo -e "${BOLD}╚══════════════════════════════════════════════════╝${NC}"
    echo ""
    echo -e "  ${BOLD}Usage:${NC}"
    echo -e "    sudo bash install-fleet-server.sh ${YELLOW}<your-domain>${NC}"
    echo ""
    echo -e "  ${BOLD}Examples:${NC}"
    echo -e "    sudo bash install-fleet-server.sh fleet.acme.com                 # Let's Encrypt (default)"
    echo -e "    sudo bash install-fleet-server.sh fleet.acme.com --self-signed   # Self-signed cert"
    echo -e "    sudo bash install-fleet-server.sh fleet.acme.com --cert /path/to/cert.pem --key /path/to/key.pem"
    echo ""
    echo -e "  Point a DNS A record to this server's public IP first."
    echo ""
    err "Domain name required as first argument"
fi

if [[ "${TLS_MODE}" == "custom" ]]; then
    if [[ -z "${CUSTOM_CERT}" || -z "${CUSTOM_KEY}" ]]; then
        err "Both --cert and --key must be provided for custom certificate"
    fi
    if [[ ! -f "${CUSTOM_CERT}" ]]; then
        err "Certificate file not found: ${CUSTOM_CERT}"
    fi
    if [[ ! -f "${CUSTOM_KEY}" ]]; then
        err "Key file not found: ${CUSTOM_KEY}"
    fi
fi

echo ""
echo -e "${BOLD}╔══════════════════════════════════════════════════╗${NC}"
echo -e "${BOLD}║     Fibratus Fleet Server — Installer            ║${NC}"
echo -e "${BOLD}╚══════════════════════════════════════════════════╝${NC}"
echo ""
info "Domain:   ${FLEET_DOMAIN}"
info "TLS mode: ${TLS_MODE}"

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

# Create database and user (idempotent — safe to re-run)
info "Setting up database..."
sudo -u postgres psql -tc "SELECT 1 FROM pg_roles WHERE rolname='${DB_USER}'" | grep -q 1 || \
    sudo -u postgres psql -c "CREATE USER ${DB_USER} WITH PASSWORD '${DB_PASS}';" 2>/dev/null
# Always reset password to match the current install's generated value
sudo -u postgres psql -c "ALTER USER ${DB_USER} WITH PASSWORD '${DB_PASS}';" 2>/dev/null
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
# Allow binding to privileged ports (443) as non-root
setcap 'cap_net_bind_service=+ep' "${INSTALL_DIR}/bin/fleet-server"
ok "Binary built at ${INSTALL_DIR}/bin/fleet-server"

# ─── Step 8: Create service user ────────────────────────────────────────────

if id "${SERVICE_USER}" &>/dev/null; then
    ok "Service user '${SERVICE_USER}' exists"
else
    useradd --system --no-create-home --shell /usr/sbin/nologin "${SERVICE_USER}"
    ok "Service user '${SERVICE_USER}' created"
fi

# ─── Step 9: Write configuration ────────────────────────────────────────────

mkdir -p "${CONFIG_DIR}" "${DATA_DIR}" "${LOG_DIR}"
chown "${SERVICE_USER}:${SERVICE_USER}" "${DATA_DIR}" "${LOG_DIR}"

# ─── Step 9a: TLS certificate ────────────────────────────────────────────────

TLS_CERT=""
TLS_KEY=""
TLS_NOTE=""

case "${TLS_MODE}" in
    letsencrypt)
        info "Obtaining Let's Encrypt TLS certificate for ${FLEET_DOMAIN}..."

        # Install certbot
        if ! command -v certbot &>/dev/null; then
            apt-get install -y -qq certbot > /dev/null 2>&1
        fi

        # Stop anything on port 80/443 temporarily (certbot needs port 80)
        systemctl stop fibratus-fleet 2>/dev/null || true

        # Obtain certificate
        certbot certonly --standalone \
            --non-interactive \
            --agree-tos \
            --register-unsafely-without-email \
            --domain "${FLEET_DOMAIN}" \
            --preferred-challenges http

        TLS_CERT="/etc/letsencrypt/live/${FLEET_DOMAIN}/fullchain.pem"
        TLS_KEY="/etc/letsencrypt/live/${FLEET_DOMAIN}/privkey.pem"

        if [[ ! -f "${TLS_CERT}" ]]; then
            err "Let's Encrypt failed. Make sure DNS for ${FLEET_DOMAIN} points to this server and port 80 is open."
        fi

        # Auto-renewal with restart hook
        mkdir -p /etc/letsencrypt/renewal-hooks/deploy
        cat > /etc/letsencrypt/renewal-hooks/deploy/fibratus-fleet.sh <<'HOOK'
#!/bin/bash
systemctl restart fibratus-fleet
HOOK
        chmod +x /etc/letsencrypt/renewal-hooks/deploy/fibratus-fleet.sh
        systemctl enable certbot.timer 2>/dev/null || true
        systemctl start certbot.timer 2>/dev/null || true

        # Ensure service user can read the cert files
        chmod 0755 /etc/letsencrypt/live /etc/letsencrypt/archive

        ok "Let's Encrypt certificate issued (auto-renewal enabled)"
        TLS_NOTE="Let's Encrypt (auto-renew via certbot)"
        ;;

    selfsigned)
        info "Generating self-signed TLS certificate..."
        mkdir -p "${CONFIG_DIR}/tls"
        openssl req -x509 -newkey rsa:4096 -sha256 -days 3650 -nodes \
            -keyout "${CONFIG_DIR}/tls/server.key" \
            -out "${CONFIG_DIR}/tls/server.crt" \
            -subj "/CN=${FLEET_DOMAIN}/O=Fibratus" \
            -addext "subjectAltName=DNS:${FLEET_DOMAIN},DNS:localhost" 2>/dev/null

        TLS_CERT="${CONFIG_DIR}/tls/server.crt"
        TLS_KEY="${CONFIG_DIR}/tls/server.key"
        chmod 600 "${TLS_KEY}"

        ok "Self-signed certificate generated"
        TLS_NOTE="Self-signed (agents need --insecure flag)"
        ;;

    custom)
        info "Using custom TLS certificate..."
        TLS_CERT="${CUSTOM_CERT}"
        TLS_KEY="${CUSTOM_KEY}"

        ok "Custom certificate: ${TLS_CERT}"
        TLS_NOTE="Custom certificate"
        ;;
esac

JWT_SECRET="$(openssl rand -hex 32)"

cat > "${CONFIG_DIR}/fleet-server.yml" <<YAML
server:
  listen: ":${LISTEN_PORT}"
  tls-cert: ${TLS_CERT}
  tls-key: ${TLS_KEY}

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

# Allow binding to port 443 as non-root
AmbientCapabilities=CAP_NET_BIND_SERVICE

# Hardening
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=${DATA_DIR} ${LOG_DIR} /etc/letsencrypt
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
    ufw allow 80/tcp comment "Let's Encrypt HTTP challenge" 2>/dev/null || true
    ufw allow 443/tcp comment "Fibratus Fleet Server" 2>/dev/null || true
    ok "Firewall ports 80 (certbot) and 443 (fleet) opened"
fi

# ─── Done ────────────────────────────────────────────────────────────────────

sleep 2

# Check if server is running
if systemctl is-active --quiet fibratus-fleet; then
    STATUS="${GREEN}RUNNING${NC}"
else
    STATUS="${RED}NOT RUNNING${NC} (check: journalctl -u fibratus-fleet)"
fi

SERVER_URL="https://${FLEET_DOMAIN}"
ENROLL_CMD="fibratus enroll --token ${ENROLL_TOKEN} --server ${SERVER_URL}"
if [[ "${TLS_MODE}" == "selfsigned" ]]; then
    ENROLL_CMD="${ENROLL_CMD} --insecure"
fi

echo ""
echo -e "${BOLD}╔══════════════════════════════════════════════════════════════╗${NC}"
echo -e "${BOLD}║              Fibratus Fleet Server — Installed!              ║${NC}"
echo -e "${BOLD}╚══════════════════════════════════════════════════════════════╝${NC}"
echo ""
echo -e "  ${BOLD}Status:${NC}       ${STATUS}"
echo -e "  ${BOLD}Dashboard:${NC}    ${GREEN}${SERVER_URL}${NC}"
echo ""
echo -e "  ${BOLD}Dashboard login:${NC}"
echo -e "    Email:      admin@fibratus.local"
echo -e "    Password:   ${YELLOW}${ADMIN_PASS}${NC}"
echo ""
echo -e "  ─────────────────────────────────────────────────────────────"
echo ""
echo -e "  ${BOLD}${GREEN}Enroll agents — run this on each Windows endpoint:${NC}"
echo ""
echo -e "    ${YELLOW}${ENROLL_CMD}${NC}"
echo ""
echo -e "  Then start the agent:"
echo -e "    ${YELLOW}fibratus service start${NC}"
echo ""
echo -e "  ─────────────────────────────────────────────────────────────"
echo ""
echo -e "  ${BOLD}Enrollment Token:${NC}  ${ENROLL_TOKEN}"
echo -e "  ${BOLD}Org ID:${NC}            ${ORG_ID}"
echo -e "  ${BOLD}Config:${NC}            ${CONFIG_DIR}/fleet-server.yml"
echo -e "  ${BOLD}TLS:${NC}               ${TLS_NOTE}"
echo -e "  ${BOLD}Logs:${NC}              journalctl -u fibratus-fleet -f"
echo ""
echo -e "  Token valid for 1 year / 1000 agents. Create more in dashboard Settings."
echo ""
