# 🖥️ VPS Monitoring Server

A comprehensive **Infrastructure Monitoring & Management Platform** — from single-server monitoring to multi-host fleet management with SSH, Docker, terminals, and AI integration.

![License](https://img.shields.io/badge/license-MIT-blue.svg)
![Go Version](https://img.shields.io/badge/go-1.25+-00ADD8?logo=go)
![React](https://img.shields.io/badge/react-19+-61DAFB?logo=react)
![Docker](https://img.shields.io/badge/docker-ready-2496ED?logo=docker)

## 📋 Table of Contents

- [Overview](#-overview)
- [Architecture (12 Phases)](#-architecture-12-phases)
- [Tech Stack](#-tech-stack)
- [Prerequisites](#-prerequisites)
- [Quick Start (Docker)](#-quick-start-docker)
- [Local Development](#-local-development)
- [Configuration](#-configuration)
- [Environment Variables](#-environment-variables)
- [Project Structure](#-project-structure)
- [Features by Phase](#-features-by-phase)
- [API Reference](#-api-reference)
- [RBAC Roles](#-rbac-roles)
- [MCP/AI Integration](#-mcpai-integration)
- [Security](#-security)
- [Database Cluster (Optional)](#-database-cluster-optional)
- [Deployment](#-deployment)
- [Files to Exclude from GitHub](#-files-to-exclude-from-github)
- [Troubleshooting](#-troubleshooting)
- [License](#-license)

---

## 🌟 Overview

This project evolved from a **VPS Monitoring Dashboard** (local server monitoring) into a full **Infrastructure Platform** across 12 development phases:

```
Phase 0:  Stabilisasi baseline monitoring
Phase 1:  Server Registry (CRUD, tags, UI)
Phase 2:  SSH Engine (keys, test, command, TOFU host keys)
Phase 3:  Remote Monitoring (agentless metrics via SSH)
Phase 4:  Docker/Podman Fleet (multi-server containers)
Phase 5:  Terminal (WebSocket + xterm.js + SSH PTY)
Phase 6:  Multi-Host Commands (snippets, blast radius, parallel exec)
Phase 7:  File Manager (SFTP browse/upload/download/rename/delete)
Phase 8:  SSH Tunnels (local/remote/SOCKS5 forwarding)
Phase 9:  Cloud Discovery (provider abstraction, import to registry)
Phase 10: Infrastructure Search (Ctrl+K global search)
Phase 11: RBAC 3-level (ADMIN/OPERATOR/VIEWER)
Phase 12: MCP/AI (read-only Model Context Protocol for AI agents)
```

The core philosophy: `MONITOR → UNDERSTAND → INVESTIGATE → ACT → VERIFY`

---

## 🏗️ Architecture (12 Phases)

```
┌─────────────────────────────────────────────────────────┐
│                    Frontend (React + Vite)                │
│  Dashboard · Servers · Containers · Commands · Tunnels   │
│  Cloud · SSH Keys · Terminal (xterm.js) · Search (⌘K)  │
└──────────────────────┬──────────────────────────────────┘
                       │ HTTP + WebSocket
┌──────────────────────▼──────────────────────────────────┐
│              Backend (Go + Gin + SQLite)                │
│                                                         │
│  ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────┐  │
│  │  Server  │ │   SSH    │ │  Remote  │ │ Container│  │
│  │ Registry │ │  Engine  │ │ Monitor  │ │  Fleet   │  │
│  └────┬─────┘ └────┬─────┘ └────┬─────┘ └────┬─────┘  │
│       │            │            │            │         │
│  ┌────▼─────┐ ┌────▼─────┐ ┌────▼─────┐ ┌────▼─────┐   │
│  │ Terminal │ │ Commands │ │  Files   │ │ Tunnels  │   │
│  │ (PTY)    │ │(multi-h) │ │ (SFTP)   │ │(fwd/socks)│  │
│  └──────────┘ └──────────┘ └──────────┘ └──────────┘   │
│                                                         │
│  ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────┐  │
│  │  Cloud   │ │  Search  │ │   RBAC   │ │   MCP    │  │
│  │Discovery │ │(Ctrl+K)  │ │(3-level)│ │ (AI RO)  │  │
│  └──────────┘ └──────────┘ └──────────┘ └──────────┘  │
└─────────────────────────────────────────────────────────┘
                       │ SSH (agentless)
              ┌────────▼────────┐
              │  Remote Servers  │
              │  (Docker/Podman) │
              └─────────────────┘
```

---

## 🔧 Tech Stack

| Layer | Technology |
|---|---|
| **Backend** | Go 1.25, Gin web framework, SQLite (modernc.org/sqlite) |
| **SSH** | golang.org/x/crypto/ssh, github.com/pkg/sftp |
| **Frontend** | React 19, Vite, TanStack Query, Recharts, xterm.js |
| **Real-time** | SSE (metrics/events), WebSocket (terminal) |
| **Database** | SQLite (default), PostgreSQL cluster (optional) |
| **Container** | Docker + docker-compose |
| **Reverse Proxy** | Nginx (production), Vite proxy (dev) |

---

## 📋 Prerequisites

- **Docker** + **Docker Compose** (production deployment)
- **Go 1.25+** + **Node.js 20+** (local development)
- **Remote servers**: SSH access with key-based auth

---

## 🚀 Quick Start (Docker)

```bash
# 1. Clone the repo
git clone https://github.com/YOUR_USERNAME/Monitoring-Server.git
cd Monitoring-Server/vps-dashboard

# 2. Copy environment template
cp .env.docker.example .env.docker

# 3. Edit .env.docker — set JWT_SECRET and BOOTSTRAP_ADMIN_PASSWORD
nano .env.docker

# 4. Start everything
docker compose up -d --build

# 5. Access the dashboard
#    http://localhost (port 80 via nginx)
#    or http://localhost:3001 (direct backend)
```

**First login:** Use the bootstrap admin password you set in `.env.docker`.

---

## 💻 Local Development

### Backend (Go)

```bash
cd vps-dashboard/backend-go

# Copy env template
cp .env.example .env

# Set required variables
export JWT_SECRET="dev-secret-change-me"
export BOOTSTRAP_ADMIN_PASSWORD="admin123"

# Install dependencies
go mod tidy

# Run the backend
go run ./cmd/api/

# Backend runs on :3001
```

### Frontend (React)

```bash
cd vps-dashboard/frontend

# Install dependencies
npm install

# Start dev server (proxies to backend on :3001)
npm run dev

# Frontend runs on :5173
```

### Run Tests

```bash
# Backend tests
cd vps-dashboard/backend-go
go test ./... -count=1

# Frontend lint + build
cd vps-dashboard/frontend
npm run lint
npm run build
```

---

## ⚙️ Configuration

### Core Environment Variables

| Variable | Default | Description |
|---|---|---|
| `JWT_SECRET` | *(required)* | JWT signing secret |
| `BOOTSTRAP_ADMIN_PASSWORD` | *(required)* | Initial admin password |
| `HTTP_ADDR` | `:3001` | Backend listen address |
| `DB_PATH` | `./data/vps-dashboard.db` | SQLite database path |
| `CORS_ORIGINS` | `http://localhost:5173` | Allowed CORS origins |

### SSH Engine (Phase 2)

| Variable | Default | Description |
|---|---|---|
| `SSH_KEYS_DIR` | `./data/ssh-keys` | Private key store (0600 files) |

### Remote Monitoring (Phase 3)

| Variable | Default | Description |
|---|---|---|
| `REMOTE_POLL_INTERVAL` | `60s` | Metrics collection interval |
| `REMOTE_MAX_PARALLEL` | `4` | Max concurrent SSH probes |
| `REMOTE_COMMAND_TIMEOUT` | `15s` | Timeout per metrics command |
| `REMOTE_RETENTION` | `24h` | Metrics data retention |

### MCP/AI (Phase 12)

| Variable | Default | Description |
|---|---|---|
| `MCP_API_KEY` | *(empty = disabled)* | API key for MCP endpoint |
| `MCP_AUDIT_PATH` | `./data/mcp-audit.jsonl` | JSONL audit log path |

### Database Cluster (Optional)

| Variable | Default | Description |
|---|---|---|
| `POSTGRES_REPLICATION_PASSWORD` | `changeme_repl` | Replication password |
| `POSTGRES_ADMIN_PASSWORD` | `changeme_admin` | Admin password |

---

## 📁 Project Structure

```
Monitoring-Server/
├── README.md                         # This file
├── .gitignore
├── database-cluster/                 # PostgreSQL master-slave cluster (optional)
│   ├── docker-compose.yml
│   ├── .env.example
│   ├── config/
│   │   ├── master-postgresql.conf
│   │   └── slave-postgresql.conf
│   ├── init-scripts/
│   │   └── 01-init-master.sql
│   ├── pgadmin/
│   │   └── servers.json
│   └── scripts/
│       └── setup-slave.sh
├── vps-dashboard/                    # Main application
│   ├── .gitignore
│   ├── docker-compose.yml            # Production deployment
│   ├── .env.docker.example           # Docker env template
│   ├── Makefile
│   ├── nginx-dashboard.conf          # Nginx config for production
│   ├── deploy.env.example            # Deploy scripts env template
│   │
│   ├── backend-go/                  # Go backend (Phase 0-12)
│   │   ├── .gitignore
│   │   ├── .env.example
│   │   ├── Dockerfile
│   │   ├── go.mod
│   │   ├── go.sum
│   │   ├── cmd/
│   │   │   └── api/
│   │   │       └── main.go           # Entry point
│   │   └── internal/
│   │       ├── app/                  # DI container
│   │       ├── auth/                 # JWT authentication
│   │       ├── config/              # Env-based config
│   │       ├── db/                  # SQLite + migrations
│   │       ├── models/              # Data models + repos
│   │       ├── httpx/
│   │       │   ├── server.go         # Route registration
│   │       │   ├── handlers/        # HTTP handlers
│   │       │   └── middleware/      # Auth, CORS, RBAC
│   │       ├── alerts/              # Alert engine
│   │       ├── backup/              # Backup scheduler
│   │       ├── cloud/              # Cloud discovery (Phase 9)
│   │       ├── commands/           # Multi-host command engine (Phase 6)
│   │       ├── containers/         # Docker/Podman fleet (Phase 4)
│   │       ├── deploy/             # Deployment service
│   │       ├── discovery/          # Project discovery
│   │       ├── docker/             # Local Docker management
│   │       ├── files/              # SFTP file manager (Phase 7)
│   │       ├── generator/          # Command generator
│   │       ├── healthcheck/        # Health check engine
│   │       ├── maintenance/        # Purger (data retention)
│   │       ├── mcp/                # MCP server for AI (Phase 12)
│   │       ├── notifier/           # Telegram + webhook notifications
│   │       ├── pm2/                # PM2 process management
│   │       ├── remote/             # Remote monitoring (Phase 3)
│   │       ├── safeexec/           # Safe command execution
│   │       ├── search/             # Infrastructure search (Phase 10)
│   │       ├── ssh/                # SSH engine + tunnels (Phase 2, 5, 8)
│   │       ├── sysinfo/            # Local system metrics
│   │       └── tunnel/             # Cloudflare tunnel management
│   │
│   ├── frontend/                    # React frontend
│   │   ├── .gitignore
│   │   ├── Dockerfile
│   │   ├── package.json
│   │   ├── vite.config.js
│   │   ├── nginx.conf
│   │   └── src/
│   │       ├── App.jsx              # Routes
│   │       ├── api/                 # API client
│   │       ├── auth/                # Auth provider + RBAC helpers
│   │       ├── components/          # Shared components
│   │       ├── pages/              # Page components
│   │       └── ui/                  # Layout, modals, search
│   │
│   ├── cloudflared-config/          # Cloudflare tunnel config
│   ├── legacy/                      # Legacy Node.js backend (reference only)
│   └── scripts/                     # Utility scripts
│
└── PROJECT ARCHITECTURE.md          # Full architecture document (2406 lines)
```

---

## 🎯 Features by Phase

### Phase 1: Server Registry
- CRUD for server entries (name, hostname, IP, SSH port/username, credential type)
- Tags with normalized `tags` + `server_tags` tables
- Status model: `online`, `degraded`, `offline`, `unknown`
- Environment classification: `development`, `staging`, `production`

### Phase 2: SSH Engine
- SSH key management (generate Ed25519, upload PEM, fingerprint, 0600 storage)
- SSH connectivity test (latency, host key fingerprint, server version)
- Command execution (timeout 30s, output cap 1MB, exit code)
- TOFU host key verification (trust on first use, MITM detection)
- Credential types: `ssh_key`, `agent`, `password` (via env var)

### Phase 3: Remote Monitoring
- Agentless metrics collection via SSH (CPU, RAM, disk, load, network, uptime)
- Background poller with bounded concurrency (MaxParallel=4)
- Metrics stored in `server_metrics` table (retention configurable)
- Auto status updates (online/offline) on each sweep
- Per-server metrics modal with charts (recharts)

### Phase 4: Docker/Podman Fleet
- Auto-detect Docker or Podman on remote servers (sentinel pattern)
- Container listing across all servers (fleet overview)
- Start/Stop/Restart containers on any server
- Container logs (tail, streaming)
- Uniform `Container` shape regardless of engine

### Phase 5: Terminal
- Interactive SSH terminal via WebSocket (xterm.js)
- PTY session with resize support (window-change)
- Cookie-based auth (browser sends JWT cookie automatically)
- Session lifecycle management (clean close on disconnect)

### Phase 6: Multi-Host Commands
- Reusable command snippets (name, description, command, variables)
- Blast-radius preview (danger classification: safe/caution/dangerous)
- Parallel execution with per-host independence (failure ≠ abort)
- Per-host results (exit code, stdout, stderr, duration)
- Audit trail in `command_runs` table

### Phase 7: File Manager
- SFTP-based file operations (browse, upload, download, rename, delete, mkdir)
- SafePath anti-traversal sanitizer (prevents `..` escape)
- Streaming upload/download
- File metadata (size, mode, mod_time)

### Phase 8: SSH Tunnels
- Local forward (-L): `local:port → remote:port` via SSH
- Remote forward (-R): `SSH server:port → local:port`
- SOCKS5 (-D): dynamic proxy with full SOCKS5 protocol
- Persistent tunnel definitions with connect/disconnect
- Auto-start on backend boot (optional)

### Phase 9: Cloud Discovery
- Provider interface: `ListInstances()`, `GetInstance()`
- ManualProvider (default, for on-premise/testing)
- Import discovered instances into Server Registry (status: unknown)
- Discovery ≠ authorization (user must configure SSH separately)

### Phase 10: Infrastructure Search
- Global search with Ctrl+K / ⌘K shortcut
- Search across: servers, commands, tunnels, tags
- Results grouped by resource kind
- Keyboard navigation (arrow keys + Enter)

### Phase 11: RBAC
- **VIEWER**: read everything, no mutations
- **OPERATOR**: + restart containers, run commands, deploy, backup, SSH terminal/files
- **ADMIN**: + manage users, credentials, providers, tunnels, snippets, config

### Phase 12: MCP/AI
- Read-only MCP server for AI agents
- 6 tools: `list_servers`, `get_server`, `list_events`, `search_infrastructure`, `list_tunnels`, `list_snippets`
- JSON-RPC 2.0 protocol (`initialize`, `tools/list`, `tools/call`)
- API key authentication (separate from JWT cookie)
- JSONL audit log for every MCP call

---

## 📡 API Reference

### Authentication
```
POST /auth/login          # Login (returns JWT cookie)
POST /auth/logout         # Logout
GET  /auth/me             # Current user info
```

### Server Registry (Phase 1)
```
GET    /servers           # List servers (filter: q, tag, environment, status)
POST   /servers           # Create server (admin)
GET    /servers/:id       # Get server
PUT    /servers/:id       # Update server (admin)
PATCH  /servers/:id       # Patch server (admin)
DELETE /servers/:id       # Delete server (admin)
GET    /servers/tags      # List all tags
GET    /servers/:id/metrics    # Latest metric (Phase 3)
GET    /servers/:id/history    # Metric history (Phase 3)
```

### SSH Engine (Phase 2)
```
GET    /ssh/keys              # List key metadata (admin)
POST   /ssh/keys              # Upload private key (admin)
POST   /ssh/keys/generate     # Generate Ed25519 key (admin)
GET    /ssh/keys/:name/public # Get public key line (admin)
DELETE /ssh/keys/:name        # Delete key (admin)
POST   /ssh/test/:id          # Test SSH connection (operator+)
POST   /ssh/command/:id       # Run command (operator+)
```

### Terminal (Phase 5)
```
GET    /servers/:id/terminal  # WebSocket upgrade (operator+)
```

### Containers (Phase 4)
```
GET    /containers              # Fleet overview (all servers)
GET    /servers/:id/containers  # Per-server containers
POST   /servers/:id/containers/:name/start   # Start (operator+)
POST   /servers/:id/containers/:name/stop    # Stop (operator+)
POST   /servers/:id/containers/:name/restart # Restart (operator+)
```

### Commands (Phase 6)
```
GET    /commands/snippets      # List snippets
POST   /commands/snippets      # Create snippet (admin)
PUT    /commands/snippets/:id  # Update (admin)
DELETE /commands/snippets/:id  # Delete (admin)
POST   /commands/preview       # Blast-radius preview (operator+)
POST   /commands/execute       # Execute multi-host (operator+)
GET    /commands/history       # Execution history
```

### Files (Phase 7)
```
GET    /servers/:id/files         # Browse directory
GET    /servers/:id/files/*path   # Stat or download (?action=download|stat)
POST   /servers/:id/files/mkdir   # Create directory (operator+)
POST   /servers/:id/files/upload  # Upload file (operator+)
POST   /servers/:id/files/rename  # Rename (operator+)
DELETE /servers/:id/files/*path   # Delete (operator+)
```

### Tunnels (Phase 8)
```
GET    /tunnels              # List tunnels
POST   /tunnels              # Create (admin)
PUT    /tunnels/:id          # Update (admin)
DELETE /tunnels/:id          # Delete (admin)
POST   /tunnels/:id/connect    # Connect (admin)
POST   /tunnels/:id/disconnect # Disconnect (admin)
```

### Cloud (Phase 9)
```
GET    /cloud/providers       # List providers
GET    /cloud/instances       # Discover instances
GET    /cloud/instances/:p/:id  # Get instance
POST   /cloud/import          # Import to registry (admin)
```

### Search (Phase 10)
```
GET    /search?q=...         # Global infrastructure search
```

### MCP (Phase 12)
```
POST   /mcp                  # JSON-RPC 2.0 (API key auth)
GET    /mcp/tools            # List tools (API key auth)
```

---

## 🔐 RBAC Roles

| Capability | VIEWER | OPERATOR | ADMIN |
|---|:---:|:---:|:---:|
| View servers/metrics/containers | ✅ | ✅ | ✅ |
| View events/search | ✅ | ✅ | ✅ |
| Restart containers | ❌ | ✅ | ✅ |
| Run SSH commands | ❌ | ✅ | ✅ |
| Open terminal | ❌ | ✅ | ✅ |
| Upload/delete files | ❌ | ✅ | ✅ |
| Execute multi-host commands | ❌ | ✅ | ✅ |
| Deploy/backup | ❌ | ✅ | ✅ |
| Manage users | ❌ | ❌ | ✅ |
| Manage SSH keys | ❌ | ❌ | ✅ |
| Manage tunnels/snippets | ❌ | ❌ | ✅ |
| Manage server registry | ❌ | ❌ | ✅ |
| Configure cloud providers | ❌ | ❌ | ✅ |

---

## 🤖 MCP/AI Integration

Enable the MCP server by setting `MCP_API_KEY` in your environment:

```bash
# In .env.docker
MCP_API_KEY=your-secret-api-key
MCP_AUDIT_PATH=/data/mcp-audit.jsonl
```

### Usage with AI agents:

```bash
# List available tools
curl -H "X-API-Key: your-secret-api-key" \
  http://localhost:3001/mcp/tools

# Call a tool (JSON-RPC 2.0)
curl -X POST -H "X-API-Key: your-secret-api-key" \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","method":"tools/call","params":{"name":"list_servers","arguments":{}},"id":1}' \
  http://localhost:3001/mcp
```

**Mode: READ ONLY** — AI agents can query but not mutate infrastructure.

---

## 🛡️ Security

- **JWT cookie auth** for web UI (httpOnly, secure)
- **API key auth** for MCP (separate from JWT)
- **RBAC 3-level**: admin/operator/viewer
- **SSH TOFU**: host key verification (MITM detection)
- **SafePath**: anti-traversal for SFTP
- **Danger classification**: safe/caution/dangerous command patterns
- **Audit events**: every infrastructure-changing operation logged
- **Private keys**: stored 0600, never returned via API
- **Passphrase-protected keys**: rejected (clear error)

---

## 🗄️ Database Cluster (Optional)

A PostgreSQL 16 master-slave cluster with TimescaleDB + pgAdmin is available in `database-cluster/`:

```bash
cd database-cluster
cp .env.example .env  # Set passwords
docker compose up -d
```

The application defaults to SQLite. PostgreSQL is for larger deployments.

---

## 🚢 Deployment

### Docker Compose (Recommended)

```bash
cd vps-dashboard
cp .env.docker.example .env.docker
# Edit .env.docker — set JWT_SECRET, BOOTSTRAP_ADMIN_PASSWORD
docker compose up -d --build
```

### Manual Deployment

See `DEPLOY_DOCKER.md` for manual deployment instructions.

---

## 📋 Files to Exclude from GitHub

The `.gitignore` files are pre-configured to exclude:

### Never upload (auto-excluded by .gitignore):
- `**/node_modules/` — npm dependencies
- `frontend/dist/` — build artifacts
- `**/*.db` — SQLite databases
- `**/data/` — runtime data (DB, backups, SSH keys, MCP audit)
- `.env` / `.env.docker` / `deploy.env` — real env files with secrets
- `cloudflared-config/credentials.json` — tunnel credentials
- `backend-go/vps-dashboard-api` — compiled binary
- `.DS_Store`, `.vscode/`, `.idea/` — editor/OS metadata

### Scripts you may want to remove before uploading:
These are personal/utility scripts that may not be relevant for public repos:

| File | Purpose | Recommendation |
|---|---|---|
| `JALANKAN_INI.sh` | Quick-start script (Indonesian) | Remove or keep |
| `DEPLOY_UPDATE.sh` | Deploy + update combo | Keep |
| `FIX_NGINX.sh` | Nginx fix script | Remove (one-time fix) |
| `FIX_TUNNEL.sh` | Tunnel fix script | Remove (one-time fix) |
| `deploy-manual.sh` | Manual deploy | Keep |
| `deploy-setup.sh` | Initial deploy setup | Keep |
| `setup-tunnel-cli.sh` | Cloudflare tunnel CLI setup | Keep |
| `verify.sh` | Post-deploy verification | Keep |
| `*.command` (5 files) | macOS double-click scripts | Remove (personal) |
| `start.ps1` | PowerShell start script | Remove if not on Windows |

### Documentation files to review:
| File | Purpose | Recommendation |
|---|---|---|
| `RUN.md` | Local run instructions | Merge into README, remove |
| `FASE2_CLOUDFLARE_SETUP.md` | Cloudflare setup (Indonesian) | Keep or merge |
| `CLOUDFLARE_TUNNEL_SETUP.md` | Tunnel setup | Keep |
| `SETUP_ROUTES_CLOUDFLARE.md` | Route setup | Merge into CLOUDFLARE doc |
| `DEPLOY_DOCKER.md` | Docker deploy guide | Keep |
| `SECURITY.md` | Security notes | Keep |
| `Dockerfile.README.md` | Dockerfile explanation | Remove (self-documenting) |

### Legacy code:
| Path | Recommendation |
|---|---|
| `legacy/backend-node/` | Remove — superseded by Go backend |

---

## 🔧 Troubleshooting

### Backend won't start
```bash
# Check if JWT_SECRET is set
echo $JWT_SECRET

# Check if data directory exists and is writable
mkdir -p data
ls -la data/

# Check migrations
go run ./cmd/api/ 2>&1 | grep -i migrate
```

### Frontend can't connect to backend
```bash
# Ensure backend is running on :3001
curl http://localhost:3001/auth/me

# Check vite proxy config
cat frontend/vite.config.js | grep -A5 proxy
```

### SSH operations fail
```bash
# Verify SSH key is stored
ls -la data/ssh-keys/

# Check key permissions (must be 0600)
chmod 600 data/ssh-keys/*

# Test SSH manually
ssh -i data/ssh-keys/your-key deploy@server-ip
```

### Docker containers not showing
```bash
# Ensure Docker socket proxy is running
docker compose ps

# Check container handler logs
docker compose logs backend | grep -i container
```

---

## 📄 License

MIT License — see individual files for details.

---

## 🙏 Acknowledgments

- **Purple** (Rust TUI) — inspiration for agentless SSH, fleet management, and MCP patterns
- **golang.org/x/crypto/ssh** — Go SSH library
- **github.com/pkg/sftp** — SFTP protocol implementation
- **xterm.js** — Terminal frontend
- **Recharts** — React charting library
