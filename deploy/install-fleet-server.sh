#!/usr/bin/env bash
#
# Fibratus Fleet Server — Complete Installer for Ubuntu 22.04/24.04
#
# Usage:
#   sudo bash install-fleet-server.sh
#
# Installs and configures:
#   - PostgreSQL (fleet state)
#   - ClickHouse (telemetry storage)
#   - Go toolchain (build server binary)
#   - Node.js (build dashboard)
#   - Nginx (reverse proxy + TLS + static files)
#   - Fleet server binary + systemd service
#   - Logrotate, firewall, auto-renewal
#
# The initial admin account has 2FA enforced — you must configure
# an authenticator app (Google Authenticator, Authy) on first login.
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
API_KEY="$(openssl rand -hex 32)"

CH_DB="fibratus"
CH_USER="fibratus"
CH_PASS="$(openssl rand -hex 16)"

GO_VERSION="1.26.1"
NODE_MAJOR="20"

REPO_URL="https://github.com/NovaSky0x1/fibratus.git"
REPO_BRANCH="${FIBRATUS_BRANCH:-feat/fleet-server}"

# ─── Colors ──────────────────────────────────────────────────────────────────

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
BOLD='\033[1m'
NC='\033[0m'

info()  { echo -e "${BLUE}[INFO]${NC}  $*"; }
ok()    { echo -e "${GREEN}[OK]${NC}    $*"; }
warn()  { echo -e "${YELLOW}[WARN]${NC}  $*"; }
err()   { echo -e "${RED}[ERROR]${NC} $*"; exit 1; }
step()  { echo -e "\n${CYAN}━━━ $* ━━━${NC}"; }

# ─── Pre-flight checks ──────────────────────────────────────────────────────

if [[ $EUID -ne 0 ]]; then
    err "This script must be run as root (use sudo)"
fi

if ! grep -qiE 'ubuntu|debian' /etc/os-release 2>/dev/null; then
    warn "This script is designed for Ubuntu/Debian. Other distros may not work."
fi

# Check minimum resources
MEM_MB=$(free -m | awk '/Mem:/{print $2}')
if [[ ${MEM_MB} -lt 1800 ]]; then
    warn "Less than 2GB RAM detected (${MEM_MB}MB). Recommended: 4GB+"
fi

echo ""
echo -e "${BOLD}╔════════════════════════════════════════════════════════════╗${NC}"
echo -e "${BOLD}║         Fibratus Fleet Server — Complete Installer        ║${NC}"
echo -e "${BOLD}╠════════════════════════════════════════════════════════════╣${NC}"
echo -e "${BOLD}║  PostgreSQL + ClickHouse + Nginx + Dashboard + 2FA       ║${NC}"
echo -e "${BOLD}╚════════════════════════════════════════════════════════════╝${NC}"
echo ""

# ─── Interactive prompts ─────────────────────────────────────────────────────

FLEET_DOMAIN=""
while [[ -z "${FLEET_DOMAIN}" ]] || [[ "${FLEET_DOMAIN}" != *"."* ]]; do
    echo -ne "  ${YELLOW}Enter the domain for the fleet server (e.g., edr.example.com)${NC}: "
    read FLEET_DOMAIN
done

ADMIN_EMAIL=""
while [[ -z "${ADMIN_EMAIL}" ]] || [[ "${ADMIN_EMAIL}" != *"@"* ]]; do
    echo -ne "  ${YELLOW}Enter admin email address${NC}: "
    read ADMIN_EMAIL
done

ADMIN_NAME=""
echo -ne "  ${YELLOW}Enter admin display name [Admin]${NC}: "
read ADMIN_NAME
ADMIN_NAME="${ADMIN_NAME:-Admin}"

ACCOUNT_NAME=""
echo -ne "  ${YELLOW}Enter account/company name [Default]${NC}: "
read ACCOUNT_NAME
ACCOUNT_NAME="${ACCOUNT_NAME:-Default}"

ORG_NAME=""
echo -ne "  ${YELLOW}Enter organization name [Production]${NC}: "
read ORG_NAME
ORG_NAME="${ORG_NAME:-Production}"

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
        echo -ne "  ${YELLOW}Enter email for Let's Encrypt notifications [${ADMIN_EMAIL}]${NC}: "
        read LE_EMAIL
        LE_EMAIL="${LE_EMAIL:-${ADMIN_EMAIL}}"
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
echo -e "  ${BOLD}Summary:${NC}"
info "Domain:       ${FLEET_DOMAIN}"
info "Admin email:  ${ADMIN_EMAIL}"
info "Account:      ${ACCOUNT_NAME}"
info "Organization: ${ORG_NAME}"
info "TLS mode:     ${TLS_MODE}"
echo ""
echo -ne "  ${YELLOW}Proceed with installation? [Y/n]${NC}: "
read CONFIRM
CONFIRM="${CONFIRM:-Y}"
if [[ ! "${CONFIRM}" =~ ^[Yy] ]]; then
    echo "Installation cancelled."
    exit 0
fi

# ═══════════════════════════════════════════════════════════════
# Installation
# ═══════════════════════════════════════════════════════════════

step "Step 1/14: System packages"

export DEBIAN_FRONTEND=noninteractive
apt-get update -qq
apt-get install -y -qq \
    git curl wget build-essential ca-certificates gnupg lsb-release \
    ufw nginx apt-transport-https software-properties-common > /dev/null 2>&1
ok "System packages installed"

# ─── Step 2: Install Go ─────────────────────────────────────────────────────

step "Step 2/14: Go ${GO_VERSION}"

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

step "Step 3/14: Node.js ${NODE_MAJOR}"

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

step "Step 4/14: PostgreSQL"

if command -v psql &>/dev/null; then
    ok "PostgreSQL already installed"
else
    info "Installing PostgreSQL..."
    apt-get install -y -qq postgresql postgresql-contrib > /dev/null 2>&1
    ok "PostgreSQL installed"
fi

systemctl enable postgresql 2>/dev/null || true
systemctl start postgresql 2>/dev/null || true

info "Setting up PostgreSQL database..."
sudo -u postgres psql -tc "SELECT 1 FROM pg_roles WHERE rolname='${DB_USER}'" | grep -q 1 || \
    sudo -u postgres psql -c "CREATE USER ${DB_USER} WITH PASSWORD '${DB_PASS}';" 2>/dev/null
sudo -u postgres psql -c "ALTER USER ${DB_USER} WITH PASSWORD '${DB_PASS}';" 2>/dev/null
sudo -u postgres psql -tc "SELECT 1 FROM pg_database WHERE datname='${DB_NAME}'" | grep -q 1 || \
    sudo -u postgres psql -c "CREATE DATABASE ${DB_NAME} OWNER ${DB_USER};" 2>/dev/null
sudo -u postgres psql -c "GRANT ALL PRIVILEGES ON DATABASE ${DB_NAME} TO ${DB_USER};" 2>/dev/null
ok "PostgreSQL database '${DB_NAME}' ready"

# ─── Step 5: Install ClickHouse ──────────────────────────────────────────────

step "Step 5/14: ClickHouse"

if command -v clickhouse-client &>/dev/null; then
    ok "ClickHouse already installed"
else
    info "Installing ClickHouse..."
    # Add ClickHouse official repository
    curl -fsSL https://packages.clickhouse.com/rpm/lts/repodata/repomd.xml.key 2>/dev/null | \
        gpg --dearmor -o /etc/apt/keyrings/clickhouse.gpg 2>/dev/null || true
    # Use the deb repo
    ARCH=$(dpkg --print-architecture)
    echo "deb [signed-by=/etc/apt/keyrings/clickhouse.gpg arch=${ARCH}] https://packages.clickhouse.com/deb stable main" > /etc/apt/sources.list.d/clickhouse.list
    apt-get update -qq
    DEBIAN_FRONTEND=noninteractive apt-get install -y -qq clickhouse-server clickhouse-client > /dev/null 2>&1
    ok "ClickHouse installed"
fi

systemctl enable clickhouse-server 2>/dev/null || true
systemctl start clickhouse-server 2>/dev/null || true

# Wait for ClickHouse to be ready
for i in $(seq 1 15); do
    if clickhouse-client --query "SELECT 1" &>/dev/null; then
        break
    fi
    sleep 1
done

info "Setting up ClickHouse database..."
clickhouse-client --query "CREATE DATABASE IF NOT EXISTS ${CH_DB}" 2>/dev/null || true
# Create ClickHouse user (skip if default user works)
clickhouse-client --query "CREATE USER IF NOT EXISTS ${CH_USER} IDENTIFIED WITH plaintext_password BY '${CH_PASS}'" 2>/dev/null || true
clickhouse-client --query "GRANT ALL ON ${CH_DB}.* TO ${CH_USER}" 2>/dev/null || true
ok "ClickHouse database '${CH_DB}' ready"

# ─── Step 6: Clone / update repo ────────────────────────────────────────────

step "Step 6/14: Source code"

if [[ -d "${INSTALL_DIR}/src" ]]; then
    info "Updating existing source..."
    cd "${INSTALL_DIR}/src"
    git fetch origin ${REPO_BRANCH} 2>/dev/null || true
    git reset --hard origin/${REPO_BRANCH} 2>/dev/null || true
else
    info "Cloning repository..."
    mkdir -p "${INSTALL_DIR}"
    git clone --depth 1 --branch ${REPO_BRANCH} "${REPO_URL}" "${INSTALL_DIR}/src"
fi
cd "${INSTALL_DIR}/src"
ok "Source code ready at ${INSTALL_DIR}/src"

# ─── Step 7: Build dashboard ────────────────────────────────────────────────

step "Step 7/14: Dashboard (React + Vite)"

cd "${INSTALL_DIR}/src/web/dashboard"
npm install --silent 2>/dev/null
npm run build 2>&1 | tail -5
ok "Dashboard built"

# ─── Step 8: Build fleet-server binary ───────────────────────────────────────

step "Step 8/14: Fleet server binary"

cd "${INSTALL_DIR}/src"
mkdir -p "${INSTALL_DIR}/bin"

go mod tidy 2>/dev/null || go mod download

CGO_ENABLED=0 go build \
    -ldflags="-s -w -X github.com/rabbitstack/fibratus/cmd/fleet-server/app.version=$(git describe --tags 2>/dev/null || echo dev) -X github.com/rabbitstack/fibratus/cmd/fleet-server/app.commit=$(git rev-parse --short HEAD 2>/dev/null || echo unknown)" \
    -o "${INSTALL_DIR}/bin/fleet-server" \
    ./cmd/fleet-server/
chmod +x "${INSTALL_DIR}/bin/fleet-server"
ok "Binary built at ${INSTALL_DIR}/bin/fleet-server"

# ─── Step 8b: Clone SigmaHQ repository ─────────────────────────────────────

step "Step 8b/14: SigmaHQ community rules repository"

SIGMAHQ_DIR="${INSTALL_DIR}/sigmahq"
if [ -d "${SIGMAHQ_DIR}/.git" ]; then
    info "SigmaHQ repo exists, pulling latest..."
    git -C "${SIGMAHQ_DIR}" pull --ff-only 2>/dev/null || warn "SigmaHQ pull failed (non-fatal)"
    ok "SigmaHQ repository updated"
else
    info "Cloning SigmaHQ repository (shallow clone)..."
    git clone --depth=1 https://github.com/SigmaHQ/sigma.git "${SIGMAHQ_DIR}" 2>/dev/null
    ok "SigmaHQ repository cloned to ${SIGMAHQ_DIR}"
fi
SIGMAHQ_RULE_COUNT=$(find "${SIGMAHQ_DIR}/rules/windows" -name "*.yml" 2>/dev/null | wc -l)
info "SigmaHQ: ${SIGMAHQ_RULE_COUNT} Windows rules available for conversion"

# ─── Step 9: Create service user ────────────────────────────────────────────

step "Step 9/14: Service user"

if id "${SERVICE_USER}" &>/dev/null; then
    ok "Service user '${SERVICE_USER}' exists"
else
    useradd --system --no-create-home --shell /usr/sbin/nologin "${SERVICE_USER}"
    ok "Service user '${SERVICE_USER}' created"
fi

# ─── Step 10: TLS certificate ───────────────────────────────────────────────

step "Step 10/14: TLS certificate"

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

        # Post-renewal hook to reload nginx
        mkdir -p /etc/letsencrypt/renewal-hooks/post
        cat > /etc/letsencrypt/renewal-hooks/post/nginx-reload.sh <<'HOOK'
#!/bin/bash
systemctl reload nginx
HOOK
        chmod +x /etc/letsencrypt/renewal-hooks/post/nginx-reload.sh

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

# ─── Step 11: Write configuration ───────────────────────────────────────────

step "Step 11/14: Configuration"

mkdir -p "${CONFIG_DIR}" "${DATA_DIR}" "${LOG_DIR}"
chown "${SERVICE_USER}:${SERVICE_USER}" "${DATA_DIR}" "${LOG_DIR}"

SERVER_URL="https://${FLEET_DOMAIN}"

cat > "${CONFIG_DIR}/fleet-server.yml" <<YAML
server:
  listen: "127.0.0.1:${BACKEND_PORT}"
  external-url: "${SERVER_URL}"

database:
  host: localhost
  port: 5432
  name: ${DB_NAME}
  user: ${DB_USER}
  password: "${DB_PASS}"
  ssl-mode: disable
  max-connections: 50

clickhouse:
  enabled: true
  host: localhost
  port: 9000
  database: ${CH_DB}
  user: ${CH_USER}
  password: "${CH_PASS}"
  max-open-conns: 20
  max-idle-conns: 10
  conn-max-lifetime-secs: 3600

auth:
  jwt-secret: "${JWT_SECRET}"
  api-keys:
    - name: "agent-key"
      key: "${API_KEY}"

agent:
  heartbeat-timeout: 90s
  offline-threshold: 5m

deployment:
  agent-binary-path: ""
  agent-config-dir: ""
  install-dir: 'C:\Program Files\Fibratus'

logging:
  level: info

dashboard:
  enabled: false
YAML

chmod 600 "${CONFIG_DIR}/fleet-server.yml"
chown "${SERVICE_USER}:${SERVICE_USER}" "${CONFIG_DIR}/fleet-server.yml"
ok "Configuration written to ${CONFIG_DIR}/fleet-server.yml"

# ─── Step 12: Configure Nginx reverse proxy ─────────────────────────────────

step "Step 12/14: Nginx reverse proxy"

cat > /etc/nginx/sites-available/fibratus-fleet <<NGINX
# Fibratus Fleet Server — Nginx reverse proxy
# Auto-generated by install-fleet-server.sh

# Redirect HTTP → HTTPS
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
    ssl_ciphers         ECDHE-ECDSA-AES128-GCM-SHA256:ECDHE-RSA-AES128-GCM-SHA256:ECDHE-ECDSA-AES256-GCM-SHA384:ECDHE-RSA-AES256-GCM-SHA384;
    ssl_prefer_server_ciphers on;
    ssl_session_cache shared:SSL:10m;
    ssl_session_timeout 10m;

    # Security headers
    add_header X-Frame-Options DENY;
    add_header X-Content-Type-Options nosniff;
    add_header X-XSS-Protection "1; mode=block";
    add_header Strict-Transport-Security "max-age=31536000; includeSubDomains" always;

    # Dashboard — serve static files directly from Nginx
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

    # Install endpoint (agent deployment)
    location /install/ {
        proxy_pass http://127.0.0.1:${BACKEND_PORT};
        proxy_set_header Host \$host;
        proxy_set_header X-Real-IP \$remote_addr;
        proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto \$scheme;
    }

    # Health check
    location /health {
        proxy_pass http://127.0.0.1:${BACKEND_PORT};
    }

    # SPA fallback — serves index.html for client-side routing
    location / {
        try_files \$uri \$uri/ /index.html;
    }

    # Deny access to hidden files
    location ~ /\. {
        deny all;
    }
}
NGINX

# Enable site, remove default
rm -f /etc/nginx/sites-enabled/default
ln -sf /etc/nginx/sites-available/fibratus-fleet /etc/nginx/sites-enabled/fibratus-fleet
nginx -t 2>/dev/null || err "Nginx configuration test failed"
ok "Nginx configured"

# ─── Step 13: Migrations + bootstrap ─────────────────────────────────────────

step "Step 13/14: Database migrations & bootstrap"

info "Running database migrations..."
"${INSTALL_DIR}/bin/fleet-server" migrate --config "${CONFIG_DIR}/fleet-server.yml"
ok "Database schema created"

info "Bootstrapping initial account, org, and root admin..."
BOOTSTRAP_OUTPUT=$("${INSTALL_DIR}/bin/fleet-server" bootstrap \
    --config "${CONFIG_DIR}/fleet-server.yml" \
    --account "${ACCOUNT_NAME}" \
    --org "${ORG_NAME}" \
    --email "${ADMIN_EMAIL}" \
    --name "${ADMIN_NAME}" 2>&1)
ADMIN_PASS=$(echo "${BOOTSTRAP_OUTPUT}" | grep "Password:" | awk '{print $NF}')
ORG_ID=$(echo "${BOOTSTRAP_OUTPUT}" | grep "Org ID:" | awk '{print $NF}')
ENROLL_TOKEN=$(echo "${BOOTSTRAP_OUTPUT}" | grep "Enrollment Token:" | awk '{print $NF}')
ok "Bootstrap complete (2FA enforcement enabled)"

# ─── Step 14: Systemd service + logrotate ────────────────────────────────────

step "Step 14/14: Systemd service, logrotate, firewall"

# Systemd service with full hardening
cat > /etc/systemd/system/fibratus-fleet.service <<SERVICE
[Unit]
Description=Fibratus Fleet Management Server
Documentation=https://www.fibratus.io
After=network-online.target postgresql.service clickhouse-server.service
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

# Security hardening
NoNewPrivileges=true
ProtectHome=true
PrivateTmp=true
ProtectSystem=strict
ReadWritePaths=${DATA_DIR} ${LOG_DIR}
ProtectKernelTunables=true
ProtectKernelModules=true
ProtectControlGroups=true

[Install]
WantedBy=multi-user.target
SERVICE

# Logrotate configuration
cat > /etc/logrotate.d/fibratus-fleet <<LOGROTATE
${LOG_DIR}/fleet-server.log {
    daily
    rotate 14
    compress
    delaycompress
    missingok
    notifempty
    create 0640 ${SERVICE_USER} ${SERVICE_USER}
    postrotate
        systemctl reload fibratus-fleet 2>/dev/null || true
    endscript
}
LOGROTATE

systemctl daemon-reload
systemctl enable fibratus-fleet
systemctl start fibratus-fleet
ok "Fleet server service started"

# Start Nginx
systemctl enable nginx
systemctl restart nginx
ok "Nginx started"

# Firewall
if command -v ufw &>/dev/null; then
    ufw allow 80/tcp comment "HTTP (Let's Encrypt renewal + redirect)" 2>/dev/null || true
    ufw allow 443/tcp comment "HTTPS (Fibratus Fleet)" 2>/dev/null || true
    ufw allow 22/tcp comment "SSH" 2>/dev/null || true
    ok "Firewall ports 22, 80, 443 opened"
fi

# ═══════════════════════════════════════════════════════════════
# Summary
# ═══════════════════════════════════════════════════════════════

sleep 2

if systemctl is-active --quiet fibratus-fleet && systemctl is-active --quiet nginx; then
    STATUS="${GREEN}RUNNING${NC}"
else
    STATUS="${RED}CHECK LOGS${NC} (journalctl -u fibratus-fleet -u nginx)"
fi

ENROLL_CMD="fibratus enroll --token ${ENROLL_TOKEN} --server ${SERVER_URL}"
if [[ "${TLS_MODE}" == "selfsigned" ]]; then
    ENROLL_CMD="${ENROLL_CMD} --insecure"
fi

INSTALL_CMD="irm ${SERVER_URL}/install/${ENROLL_TOKEN} | iex"

echo ""
echo -e "${BOLD}╔════════════════════════════════════════════════════════════════╗${NC}"
echo -e "${BOLD}║           Fibratus Fleet Server — Installation Complete        ║${NC}"
echo -e "${BOLD}╚════════════════════════════════════════════════════════════════╝${NC}"
echo ""
echo -e "  ${BOLD}Status:${NC}       ${STATUS}"
echo -e "  ${BOLD}Dashboard:${NC}    ${GREEN}${SERVER_URL}${NC}"
echo ""
echo -e "  ─────────────────────────────────────────────────────────────"
echo ""
echo -e "  ${BOLD}Dashboard login:${NC}"
echo -e "    Email:      ${CYAN}${ADMIN_EMAIL}${NC}"
echo -e "    Password:   ${YELLOW}${ADMIN_PASS}${NC}"
echo -e "    Role:       root"
echo ""
echo -e "  ${RED}${BOLD}IMPORTANT:${NC} 2FA is enforced on this account."
echo -e "  On first login you will be prompted to set up an"
echo -e "  authenticator app (Google Authenticator, Authy, etc)."
echo ""
echo -e "  ─────────────────────────────────────────────────────────────"
echo ""
echo -e "  ${BOLD}${GREEN}Deploy agents — PowerShell one-liner (run as Administrator):${NC}"
echo ""
echo -e "    ${YELLOW}${INSTALL_CMD}${NC}"
echo ""
echo -e "  Or manually:"
echo -e "    ${YELLOW}${ENROLL_CMD}${NC}"
echo -e "    ${YELLOW}fibratus service start${NC}"
echo ""
echo -e "  ─────────────────────────────────────────────────────────────"
echo ""
echo -e "  ${BOLD}Infrastructure:${NC}"
echo -e "    PostgreSQL:   localhost:5432 / ${DB_NAME}"
echo -e "    ClickHouse:   localhost:9000 / ${CH_DB}"
echo -e "    Backend:      127.0.0.1:${BACKEND_PORT}"
echo -e "    Nginx:        ${FLEET_DOMAIN}:443 → backend"
echo ""
echo -e "  ${BOLD}Credentials:${NC}"
echo -e "    Enrollment:   ${ENROLL_TOKEN}"
echo -e "    API Key:      ${API_KEY}"
echo -e "    Org ID:       ${ORG_ID}"
echo -e "    TLS:          ${TLS_NOTE}"
echo ""
echo -e "  ${BOLD}Files:${NC}"
echo -e "    Config:       ${CONFIG_DIR}/fleet-server.yml"
echo -e "    Logs:         journalctl -u fibratus-fleet -f"
echo -e "    Nginx:        /var/log/nginx/access.log"
echo -e "    Source:       ${INSTALL_DIR}/src"
echo ""
echo -e "  Token valid for 1 year / 1000 agents. Create more in dashboard Settings."
echo ""

# Save credentials to a file for reference (owned by root, 600 perms)
CREDS_FILE="${CONFIG_DIR}/install-credentials.txt"
cat > "${CREDS_FILE}" <<CREDS
# Fibratus Fleet Server — Installation Credentials
# Generated: $(date -Iseconds)
# Domain: ${FLEET_DOMAIN}
#
# !! STORE SECURELY AND DELETE THIS FILE AFTER SAVING CREDENTIALS !!

Dashboard URL: ${SERVER_URL}
Admin Email:   ${ADMIN_EMAIL}
Admin Pass:    ${ADMIN_PASS}
Admin Role:    root

PostgreSQL:    ${DB_USER}:${DB_PASS}@localhost:5432/${DB_NAME}
ClickHouse:    ${CH_USER}:${CH_PASS}@localhost:9000/${CH_DB}
JWT Secret:    ${JWT_SECRET}
API Key:       ${API_KEY}
Enrollment:    ${ENROLL_TOKEN}
Org ID:        ${ORG_ID}
CREDS
chmod 600 "${CREDS_FILE}"
echo -e "  ${YELLOW}Credentials saved to ${CREDS_FILE}${NC}"
echo -e "  ${RED}Delete this file after saving credentials securely!${NC}"
echo ""
