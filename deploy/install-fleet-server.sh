#!/usr/bin/env bash
#
# Fibratus Fleet Server — Installer for Ubuntu 22.04/24.04
#
# Usage:
#   sudo bash install-fleet-server.sh
#
# The script will interactively prompt for:
#   - Domain name (e.g., edr.acme.com)
#   - Email for Let's Encrypt
#   - TLS mode: Let's Encrypt (default), self-signed, or custom cert
#
set -euo pipefail

# ─── Configuration ───────────────────────────────────────────────────────────

INSTALL_DIR="/opt/fibratus-fleet"
CONFIG_DIR="/etc/fibratus"
DATA_DIR="/var/lib/fibratus-fleet"
LOG_DIR="/var/log/fibratus-fleet"
SERVICE_USER="fibratus"
BACKEND_PORT="8443"  # Internal only — Nginx proxies to this

DB_NAME="fibratus_fleet"
DB_USER="fibratus"
DB_PASS="$(openssl rand -hex 16)"
JWT_SECRET="$(openssl rand -hex 32)"

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
    warn "This script is designed for Ubuntu/Debian. Other distros may not work."
fi

echo ""
echo -e "${BOLD}╔══════════════════════════════════════════════════╗${NC}"
echo -e "${BOLD}║     Fibratus Fleet Server — Installer            ║${NC}"
echo -e "${BOLD}╚══════════════════════════════════════════════════╝${NC}"
echo ""

# ─── Interactive prompts ─────────────────────────────────────────────────────

FLEET_DOMAIN=""
while [[ $FLEET_DOMAIN != *[.]*[.]* ]]; do
    echo -ne "  ${YELLOW}Enter the domain for the fleet server (e.g., edr.example.com)${NC}: "
    read FLEET_DOMAIN
done

echo ""
echo -e "  ${BOLD}TLS Certificate Options:${NC}"
echo -e "    1) Let's Encrypt (${GREEN}recommended${NC} — free, trusted, auto-renewing)"
echo -e "    2) Self-signed certificate"
echo -e "    3) Provide your own certificate files"
echo ""
echo -ne "  ${YELLOW}Select TLS option [1]${NC}: "
read TLS_CHOICE
TLS_CHOICE="${TLS_CHOICE:-1}"

TLS_MODE="letsencrypt"
CUSTOM_CERT=""
CUSTOM_KEY=""
LE_EMAIL=""

case "${TLS_CHOICE}" in
    1)
        TLS_MODE="letsencrypt"
        echo ""
        echo -ne "  ${YELLOW}Enter email for Let's Encrypt notifications${NC}: "
        read LE_EMAIL
        if [[ -z "${LE_EMAIL}" ]]; then
            err "Email is required for Let's Encrypt"
        fi
        ;;
    2)
        TLS_MODE="selfsigned"
        ;;
    3)
        TLS_MODE="custom"
        echo ""
        echo -ne "  ${YELLOW}Path to certificate file (fullchain.pem)${NC}: "
        read CUSTOM_CERT
        echo -ne "  ${YELLOW}Path to private key file (privkey.pem)${NC}: "
        read CUSTOM_KEY
        [[ -f "${CUSTOM_CERT}" ]] || err "Certificate file not found: ${CUSTOM_CERT}"
        [[ -f "${CUSTOM_KEY}" ]] || err "Key file not found: ${CUSTOM_KEY}"
        ;;
    *)
        err "Invalid choice: ${TLS_CHOICE}"
        ;;
esac

echo ""
info "Domain:   ${FLEET_DOMAIN}"
info "TLS mode: ${TLS_MODE}"
echo ""

# ─── Step 1: System packages ────────────────────────────────────────────────

info "Updating system packages..."
export DEBIAN_FRONTEND=noninteractive
apt-get update -qq
apt-get install -y -qq git curl wget build-essential ca-certificates gnupg lsb-release ufw nginx > /dev/null 2>&1
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

systemctl start postgresql 2>/dev/null || true

info "Setting up database..."
sudo -u postgres psql -tc "SELECT 1 FROM pg_roles WHERE rolname='${DB_USER}'" | grep -q 1 || \
    sudo -u postgres psql -c "CREATE USER ${DB_USER} WITH PASSWORD '${DB_PASS}';" 2>/dev/null
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

# ─── Step 9: TLS certificate ────────────────────────────────────────────────

TLS_CERT=""
TLS_KEY=""
TLS_NOTE=""

case "${TLS_MODE}" in
    letsencrypt)
        info "Obtaining Let's Encrypt TLS certificate for ${FLEET_DOMAIN}..."

        if ! command -v certbot &>/dev/null; then
            apt-get install -y -qq certbot python3-certbot-nginx > /dev/null 2>&1
        fi

        # Stop nginx temporarily for standalone challenge
        systemctl stop nginx 2>/dev/null || true

        certbot certonly --standalone \
            --non-interactive \
            --agree-tos \
            --email "${LE_EMAIL}" \
            --domain "${FLEET_DOMAIN}" \
            --preferred-challenges http

        TLS_CERT="/etc/letsencrypt/live/${FLEET_DOMAIN}/fullchain.pem"
        TLS_KEY="/etc/letsencrypt/live/${FLEET_DOMAIN}/privkey.pem"

        if [[ ! -f "${TLS_CERT}" ]]; then
            err "Let's Encrypt failed. Make sure DNS for ${FLEET_DOMAIN} points to this server and port 80 is open."
        fi

        # Auto-renewal via certbot timer (reloads nginx automatically)
        systemctl enable certbot.timer 2>/dev/null || true
        systemctl start certbot.timer 2>/dev/null || true

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

        ok "Self-signed certificate generated"
        TLS_NOTE="Self-signed (agents need --insecure flag)"
        ;;

    custom)
        TLS_CERT="${CUSTOM_CERT}"
        TLS_KEY="${CUSTOM_KEY}"
        ok "Using custom certificate: ${TLS_CERT}"
        TLS_NOTE="Custom certificate"
        ;;
esac

# ─── Step 10: Write configuration ───────────────────────────────────────────

mkdir -p "${CONFIG_DIR}" "${DATA_DIR}" "${LOG_DIR}"
chown "${SERVICE_USER}:${SERVICE_USER}" "${DATA_DIR}" "${LOG_DIR}"

# Fleet server config — listens on localhost only, Nginx handles TLS
cat > "${CONFIG_DIR}/fleet-server.yml" <<YAML
server:
  listen: "127.0.0.1:${BACKEND_PORT}"

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
  api-keys: []

agent:
  heartbeat-timeout: 90s
  offline-threshold: 5m

logging:
  level: info

dashboard:
  enabled: false
YAML

chmod 600 "${CONFIG_DIR}/fleet-server.yml"
chown "${SERVICE_USER}:${SERVICE_USER}" "${CONFIG_DIR}/fleet-server.yml"
ok "Configuration written to ${CONFIG_DIR}/fleet-server.yml"

# ─── Step 11: Configure Nginx reverse proxy ─────────────────────────────────

info "Configuring Nginx..."

cat > /etc/nginx/sites-available/fibratus-fleet <<NGINX
server {
    listen 80;
    server_name ${FLEET_DOMAIN};
    return 301 https://\$host\$request_uri;
}

server {
    listen 443 ssl http2;
    server_name ${FLEET_DOMAIN};

    ssl_certificate     ${TLS_CERT};
    ssl_certificate_key ${TLS_KEY};
    ssl_protocols       TLSv1.2 TLSv1.3;
    ssl_ciphers         HIGH:!aNULL:!MD5;
    ssl_prefer_server_ciphers on;

    # Dashboard — serve static files directly from Nginx (fast)
    root ${INSTALL_DIR}/src/web/dashboard/dist;
    index index.html;

    # API routes — proxy to Go backend
    location /api/ {
        proxy_pass http://127.0.0.1:${BACKEND_PORT};
        proxy_http_version 1.1;
        proxy_set_header Host \$host;
        proxy_set_header X-Real-IP \$remote_addr;
        proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto \$scheme;
        proxy_read_timeout 300s;
        proxy_send_timeout 300s;
        client_max_body_size 50m;
    }

    # Health check
    location /health {
        proxy_pass http://127.0.0.1:${BACKEND_PORT};
    }

    # SPA fallback — serves index.html for client-side routing
    location / {
        try_files \$uri \$uri/ /index.html;
    }
}
NGINX

# Enable site, remove default
rm -f /etc/nginx/sites-enabled/default
ln -sf /etc/nginx/sites-available/fibratus-fleet /etc/nginx/sites-enabled/fibratus-fleet
nginx -t 2>/dev/null || err "Nginx configuration test failed"
ok "Nginx configured as reverse proxy"

# ─── Step 12: Run migrations + bootstrap ─────────────────────────────────────

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

# ─── Step 13: Install systemd service ────────────────────────────────────────

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

NoNewPrivileges=true
ProtectHome=true
PrivateTmp=true

[Install]
WantedBy=multi-user.target
SERVICE

systemctl daemon-reload
systemctl enable fibratus-fleet
systemctl start fibratus-fleet
ok "Fleet server service started"

# Start Nginx
systemctl enable nginx
systemctl restart nginx
ok "Nginx started"

# ─── Step 14: Firewall ──────────────────────────────────────────────────────

if command -v ufw &>/dev/null; then
    ufw allow 80/tcp comment "HTTP (Let's Encrypt renewal)" 2>/dev/null || true
    ufw allow 443/tcp comment "HTTPS (Fibratus Fleet)" 2>/dev/null || true
    ok "Firewall ports 80 and 443 opened"
fi

# ─── Done ────────────────────────────────────────────────────────────────────

sleep 2

if systemctl is-active --quiet fibratus-fleet && systemctl is-active --quiet nginx; then
    STATUS="${GREEN}RUNNING${NC}"
else
    STATUS="${RED}CHECK LOGS${NC} (journalctl -u fibratus-fleet -u nginx)"
fi

SERVER_URL="https://${FLEET_DOMAIN}"
ENROLL_CMD="fibratus enroll --token ${ENROLL_TOKEN} --server ${SERVER_URL}"
if [[ "${TLS_MODE}" == "selfsigned" ]]; then
    ENROLL_CMD="${ENROLL_CMD} --insecure"
fi

echo ""
echo -e "${BOLD}╔══════════════════════════════════════════════════════════════╗${NC}"
echo -e "${BOLD}║           Fibratus Fleet Server — Installed!                 ║${NC}"
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
echo -e "  ${BOLD}TLS:${NC}               ${TLS_NOTE}"
echo -e "  ${BOLD}Config:${NC}            ${CONFIG_DIR}/fleet-server.yml"
echo -e "  ${BOLD}Logs:${NC}              journalctl -u fibratus-fleet -f"
echo -e "  ${BOLD}Nginx logs:${NC}        /var/log/nginx/access.log"
echo ""
echo -e "  Token valid for 1 year / 1000 agents. Create more in dashboard Settings."
echo ""
