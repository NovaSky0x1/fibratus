# Deployment

The Fleet Server runs on Linux and is deployed using an interactive install script that configures all components automatically.

## Requirements

### Server Requirements

| Component | Minimum | Recommended |
|-----------|---------|-------------|
| **OS** | Ubuntu 20.04+ / Debian 11+ | Ubuntu 22.04 LTS |
| **CPU** | 2 cores | 4+ cores |
| **RAM** | 4 GB | 8+ GB |
| **Disk** | 20 GB | 100+ GB (telemetry grows with fleet size) |
| **Network** | Public IP with ports 443 open | Static IP with DNS |
| **Go** | 1.26.x | Latest 1.26.x |
| **Node.js** | 18.x+ | 20.x LTS |

### Software Dependencies

The install script automatically installs:
- **PostgreSQL** — fleet state storage
- **ClickHouse** — telemetry event storage
- **Nginx** — reverse proxy and TLS termination
- **Certbot** — Let's Encrypt certificate management
- **Go** — server binary compilation
- **Node.js + npm** — dashboard build

## Installation

### Interactive Install

```bash
sudo bash deploy/install-fleet-server.sh
```

The script prompts for:
1. **Domain name** — the FQDN for TLS certificate provisioning (e.g., `edr.example.com`)
2. **TLS mode** — Let's Encrypt (recommended), self-signed, or custom certificate

### What the Install Script Does

1. **System packages**: Installs PostgreSQL, ClickHouse, Nginx, Certbot, Go, Node.js
2. **Database setup**: Creates `fibratus_fleet` PostgreSQL database with a generated password
3. **User creation**: Creates a `fibratus` system user for the service
4. **Directory structure**: Creates `/opt/fibratus-fleet/` with `src/`, `bin/`, `sigmahq/` subdirectories
5. **Git clone**: Clones the repository into `/opt/fibratus-fleet/src`
6. **Build server**: Compiles the Go backend binary
7. **Build dashboard**: Runs `npm install && npm run build` for the React dashboard
8. **TLS configuration**: Provisions certificates via Let's Encrypt or generates self-signed
9. **Nginx configuration**: Sets up reverse proxy for HTTP API, gRPC, and static dashboard files
10. **Systemd service**: Creates and enables the `fibratus-fleet` service
11. **Bootstrap**: Runs `fleet-server bootstrap` to create initial admin account and enrollment token
12. **Credentials**: Saves initial login credentials to `/etc/fibratus/install-credentials.txt`

## Server Architecture

### Directory Layout

```
/opt/fibratus-fleet/
├── src/                    # Git repository (feat/fleet-server branch)
│   ├── cmd/fleet-server/   # Server binary source
│   ├── web/dashboard/      # React dashboard source
│   │   └── dist/           # Built dashboard (served by Nginx)
│   ├── docs/               # Documentation (served by Nginx at /docs)
│   └── ...
├── bin/
│   └── fleet-server        # Compiled server binary
└── sigmahq/                # SigmaHQ community rules (optional)

/etc/fibratus/
├── fleet-server.yml        # Server configuration
└── install-credentials.txt # Initial admin credentials

/var/log/fibratus-fleet/
└── fleet-server.log        # Server application log
```

### Nginx Configuration

Nginx handles three types of traffic:

```nginx
server {
    listen 443 ssl http2;
    server_name edr.example.com;

    # TLS (Let's Encrypt)
    ssl_certificate /etc/letsencrypt/live/edr.example.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/edr.example.com/privkey.pem;

    # Dashboard static files
    location / {
        root /opt/fibratus-fleet/src/web/dashboard/dist;
        try_files $uri $uri/ /index.html;
    }

    # REST API proxy
    location /api/ {
        proxy_pass http://127.0.0.1:8443;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }

    # gRPC proxy (agent communication)
    location /fleet. {
        grpc_pass grpc://127.0.0.1:8444;
    }

    # Documentation
    location /docs {
        alias /opt/fibratus-fleet/src/docs;
        index index.html;
        try_files $uri $uri/ /docs/index.html;
    }
}
```

### Systemd Service

The `fibratus-fleet` service runs as the `fibratus` user with security hardening:

```ini
[Unit]
Description=Fibratus Fleet Server
After=network.target postgresql.service clickhouse-server.service

[Service]
Type=simple
User=fibratus
ExecStart=/opt/fibratus-fleet/bin/fleet-server serve --config /etc/fibratus/fleet-server.yml
Restart=on-failure
RestartSec=5

# Security hardening
ProtectSystem=strict
ReadWritePaths=/var/log/fibratus-fleet /opt/fibratus-fleet
NoNewPrivileges=true

[Install]
WantedBy=multi-user.target
```

### TLS Modes

| Mode | Description | Use Case |
|------|-------------|----------|
| **Let's Encrypt** | Automatic certificate provisioning and renewal via certbot | Production with public domain |
| **Self-signed** | Generated self-signed certificate | Development/testing |
| **Custom** | Bring your own certificate files | Enterprise with existing PKI |

Let's Encrypt certificates auto-renew via the certbot systemd timer.

## Server Commands

The fleet server binary supports several subcommands:

```bash
# Start the server
fleet-server serve --config /etc/fibratus/fleet-server.yml

# Run database migrations
fleet-server migrate --config /etc/fibratus/fleet-server.yml

# Bootstrap initial account, admin user, and enrollment token
fleet-server bootstrap --config /etc/fibratis/fleet-server.yml
```

## Updating the Server

To update after pushing new code:

```bash
cd /opt/fibratus-fleet/src

# Pull latest code
git fetch origin && git reset --hard origin/feat/fleet-server

# Rebuild server binary
go build -o /opt/fibratus-fleet/bin/fleet-server ./cmd/fleet-server

# Rebuild dashboard
cd web/dashboard && npm run build

# Restart service
systemctl restart fibratus-fleet
```

## Monitoring

### Server Logs

```bash
# Live log stream
tail -f /var/log/fibratus-fleet/fleet-server.log

# Recent entries
journalctl -u fibratus-fleet --since "1 hour ago"
```

### Service Status

```bash
systemctl status fibratus-fleet
```

### Database Health

```bash
# PostgreSQL
sudo -u postgres psql -d fibratus_fleet -c "SELECT count(*) FROM agents;"

# ClickHouse
clickhouse-client --query "SELECT count() FROM fibratus_fleet.telemetry_events"
```
