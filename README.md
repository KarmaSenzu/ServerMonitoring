# 🖥️ VPS Monitoring Server

A comprehensive, production-ready VPS monitoring and management system with real-time metrics, Docker container management, PM2 process monitoring, automated deployments, and alerting capabilities.

![License](https://img.shields.io/badge/license-MIT-blue.svg)
![Go Version](https://img.shields.io/badge/go-1.21+-00ADD8?logo=go)
![React](https://img.shields.io/badge/react-18+-61DAFB?logo=react)
![PostgreSQL](https://img.shields.io/badge/postgresql-16+-336791?logo=postgresql)

## 📋 Table of Contents

- [Features](#-features)
- [Tech Stack](#-tech-stack)
- [Architecture](#-architecture)
- [Prerequisites](#-prerequisites)
- [Quick Start](#-quick-start)
- [Project Structure](#-project-structure)
- [Configuration](#-configuration)
- [Deployment](#-deployment)
- [Security](#-security)
- [API Documentation](#-api-documentation)
- [Troubleshooting](#-troubleshooting)
- [Contributing](#-contributing)

## ✨ Features

### Core Monitoring
- **Real-time System Metrics**: CPU, Memory, Disk, Network usage with historical data
- **Docker Container Management**: Start, stop, restart containers with live logs
- **PM2 Process Monitoring**: Track Node.js applications, view logs, restart processes
- **Health Checks**: Automated endpoint monitoring with configurable intervals
- **Event Logging**: Comprehensive audit trail of all system activities

### Management & Automation
- **Project Discovery**: Auto-detect Docker and PM2 projects on your VPS
- **One-Click Deployments**: Deploy projects with automated backup and rollback
- **Environment Management**: Manage environment variables across projects
- **Config Generator**: Generate Docker Compose and PM2 ecosystem configs

### Alerting & Notifications
- **Smart Alerts**: CPU, memory, disk space thresholds with configurable rules
- **Telegram Integration**: Receive notifications on critical events
- **Webhook Support**: Send alerts to custom endpoints
- **Alert History**: Track all triggered alerts with timestamps

### Infrastructure
- **PostgreSQL Cluster**: Master-slave replication for high availability
- **Cloudflare Tunnel**: Secure external access without exposing ports
- **Automated Backups**: Scheduled database backups with retention policies
- **Multi-user Support**: Role-based access control (Admin/Viewer)

## 🛠️ Tech Stack

### Backend
- **Go 1.21+**: High-performance API server
- **Chi Router**: Lightweight HTTP router
- **SQLite**: Embedded database for metrics and configurations
- **JWT Authentication**: Secure token-based auth
- **Server-Sent Events (SSE)**: Real-time updates

### Frontend
- **React 18**: Modern UI framework
- **Vite**: Lightning-fast build tool
- **CSS3**: Custom responsive design
- **Chart.js**: System metrics visualization

### Database & Infrastructure
- **PostgreSQL 16**: Master-slave cluster (optional)
- **Docker & Docker Compose**: Containerized deployment
- **Cloudflare Tunnel**: Secure tunneling solution
- **Nginx**: Reverse proxy and static file serving

### DevOps & Tooling
- **PM2**: Node.js process manager
- **Bash/Shell Scripts**: Deployment automation
- **Make**: Build automation

## 🏗️ Architecture

```
┌─────────────────────────────────────────────────┐
│              Cloudflare Tunnel                  │
│          (Secure External Access)               │
└──────────────────┬──────────────────────────────┘
                   │
┌──────────────────▼──────────────────────────────┐
│                 Nginx                           │
│         (Reverse Proxy / SSL)                   │
└──────┬─────────────────────────┬────────────────┘
       │                         │
┌──────▼────────┐       ┌────────▼─────────┐
│   Frontend    │       │   Backend (Go)   │
│  (React/Vite) │       │   API Server     │
│   Port 5173   │       │   Port 5050      │
└───────────────┘       └─────────┬────────┘
                                  │
                    ┌─────────────┼────────────┐
                    │             │            │
              ┌─────▼─────┐ ┌────▼────┐ ┌────▼─────┐
              │  SQLite   │ │ Docker  │ │   PM2    │
              │   (Data)  │ │  Engine │ │  Daemon  │
              └───────────┘ └─────────┘ └──────────┘
```

## 📦 Prerequisites

### Required
- **Operating System**: Linux (Ubuntu 20.04+ recommended) or macOS
- **Go**: 1.21 or higher
- **Node.js**: 18.x or higher
- **Docker**: 20.10+ with Docker Compose v2
- **Git**: For cloning the repository

### Optional
- **PM2**: For Node.js process management (`npm install -g pm2`)
- **PostgreSQL**: For database cluster setup
- **Cloudflare Account**: For tunnel setup

## 🚀 Quick Start

### 1. Clone the Repository

```bash
git clone https://github.com/KarmaSenzu/Monitoring-Server.git
cd Monitoring-Server
```

### 2. Setup VPS Dashboard

```bash
cd vps-dashboard
```

#### Configure Backend

```bash
cd backend-go
cp .env.example .env
```

Edit `.env` and configure:
```env
# Server Configuration
PORT=5050
HOST=0.0.0.0

# JWT Secret (CHANGE THIS!)
JWT_SECRET=your-super-secret-jwt-key-change-this

# Database
DB_PATH=./data/vps-dashboard.db

# Telegram Notifications (Optional)
TELEGRAM_BOT_TOKEN=your-bot-token
TELEGRAM_CHAT_ID=your-chat-id
```

#### Build and Run Backend

```bash
# Build
go build -o vps-dashboard-api ./cmd/api

# Run
./vps-dashboard-api
```

Backend will run on `http://localhost:5050`

#### Setup Frontend

```bash
cd ../frontend
npm install
cp .env.example .env
```

Edit `frontend/.env`:
```env
VITE_API_URL=http://localhost:5050
```

```bash
# Development
npm run dev

# Production build
npm run build
```

Frontend will run on `http://localhost:5173`

### 3. Access the Dashboard

1. Open browser: `http://localhost:5173`
2. Default credentials (first run creates admin):
   - **Username**: `admin`
   - **Password**: `admin123`
3. **⚠️ IMPORTANT**: Change password immediately after first login!

## 📁 Project Structure

```
Monitoring-Server/
├── vps-dashboard/              # Main application
│   ├── backend-go/             # Go API server
│   │   ├── cmd/api/            # Application entry point
│   │   ├── internal/           # Internal packages
│   │   │   ├── alerts/         # Alert system
│   │   │   ├── auth/           # Authentication
│   │   │   ├── backup/         # Backup management
│   │   │   ├── deploy/         # Deployment system
│   │   │   ├── docker/         # Docker integration
│   │   │   ├── healthcheck/    # Health monitoring
│   │   │   ├── httpx/          # HTTP handlers & middleware
│   │   │   ├── models/         # Data models
│   │   │   ├── notifier/       # Notification system
│   │   │   ├── pm2/            # PM2 integration
│   │   │   └── sysinfo/        # System metrics
│   │   └── data/               # SQLite database (gitignored)
│   ├── frontend/               # React frontend
│   │   ├── src/
│   │   │   ├── api/            # API client
│   │   │   ├── auth/           # Auth context & guards
│   │   │   ├── components/     # Reusable components
│   │   │   ├── pages/          # Page components
│   │   │   └── ui/             # UI utilities
│   │   └── dist/               # Build output (gitignored)
│   ├── cloudflared-config/     # Cloudflare Tunnel config
│   ├── scripts/                # Utility scripts
│   └── docker-compose.yml      # Docker deployment
├── database-cluster/           # PostgreSQL HA setup
│   ├── config/                 # PostgreSQL configurations
│   ├── init-scripts/           # Database initialization
│   └── docker-compose.yml      # Cluster deployment
└── README.md                   # This file
```

## ⚙️ Configuration

### Backend Environment Variables

Create `vps-dashboard/backend-go/.env`:

```env
# Server Configuration
PORT=5050                       # API server port
HOST=0.0.0.0                   # Bind address (0.0.0.0 for all interfaces)

# Security
JWT_SECRET=change-this-to-a-very-long-random-string-at-least-32-characters
JWT_EXPIRY_HOURS=24            # Token expiration (hours)

# Database
DB_PATH=./data/vps-dashboard.db  # SQLite database path

# System Monitoring
METRICS_INTERVAL=30              # Metrics collection interval (seconds)
METRICS_RETENTION_DAYS=30        # How long to keep historical data

# Alert Thresholds
CPU_THRESHOLD=80                 # CPU usage alert threshold (%)
MEMORY_THRESHOLD=85              # Memory usage alert threshold (%)
DISK_THRESHOLD=90                # Disk usage alert threshold (%)

# Telegram Notifications (Optional)
TELEGRAM_BOT_TOKEN=              # Get from @BotFather
TELEGRAM_CHAT_ID=                # Your chat ID or group ID

# Webhook Notifications (Optional)
WEBHOOK_URL=                     # POST alerts to this URL
WEBHOOK_ENABLED=false

# Backup Configuration
BACKUP_ENABLED=true
BACKUP_INTERVAL_HOURS=24
BACKUP_RETENTION_DAYS=7
BACKUP_PATH=./data/backups

# Health Check
HEALTHCHECK_ENABLED=true
HEALTHCHECK_INTERVAL=60          # Seconds between health checks

# Logging
LOG_LEVEL=info                   # debug, info, warn, error
LOG_FORMAT=json                  # json or text
```

### Frontend Environment Variables

Create `vps-dashboard/frontend/.env`:

```env
# API Configuration
VITE_API_URL=http://localhost:5050

# For production, use your domain:
# VITE_API_URL=https://monitor.yourdomain.com
```

### Docker Environment Variables

Create `vps-dashboard/.env.docker` for Docker deployment:

```env
# Backend
BACKEND_PORT=5050
JWT_SECRET=your-production-jwt-secret-here

# Frontend
FRONTEND_PORT=80

# Telegram (Optional)
TELEGRAM_BOT_TOKEN=
TELEGRAM_CHAT_ID=
```

## 🐳 Deployment

### Option 1: Docker Compose (Recommended)

#### 1. Prepare Configuration

```bash
cd vps-dashboard
cp .env.docker.example .env.docker
```

Edit `.env.docker` with your settings.

#### 2. Deploy with Docker

```bash
# Build and start all services
docker-compose up -d

# View logs
docker-compose logs -f

# Stop services
docker-compose down
```

Services will be available:
- **Frontend**: http://localhost:80
- **Backend API**: http://localhost:5050

#### 3. Setup Nginx (Optional - for production)

```bash
# Copy nginx config
sudo cp nginx-dashboard.conf /etc/nginx/sites-available/vps-dashboard

# Enable site
sudo ln -s /etc/nginx/sites-available/vps-dashboard /etc/nginx/sites-enabled/

# Test and reload nginx
sudo nginx -t
sudo systemctl reload nginx
```

### Option 2: Manual Deployment to VPS

#### 1. Prepare VPS

```bash
# Update system
sudo apt update && sudo apt upgrade -y

# Install dependencies
sudo apt install -y git docker.io docker-compose nodejs npm golang-go

# Install PM2 (if managing Node.js apps)
sudo npm install -g pm2
```

#### 2. Clone and Setup

```bash
# Clone repository
git clone https://github.com/KarmaSenzu/Monitoring-Server.git
cd Monitoring-Server/vps-dashboard

# Setup backend
cd backend-go
cp .env.example .env
# Edit .env with your settings
go build -o vps-dashboard-api ./cmd/api

# Setup frontend
cd ../frontend
npm install
cp .env.example .env
# Edit .env with your API URL
npm run build
```

#### 3. Run with PM2

```bash
# Start backend with PM2
pm2 start vps-dashboard-api --name "vps-monitor-api"

# Serve frontend with nginx or PM2
pm2 serve frontend/dist 80 --name "vps-monitor-frontend"

# Save PM2 config
pm2 save
pm2 startup
```

#### 4. Using Deployment Script

```bash
# Automated deployment
chmod +x deploy.sh
cp deploy.env.example deploy.env
# Edit deploy.env with VPS details

# Deploy to VPS
./deploy.sh
```

### Option 3: Cloudflare Tunnel Setup

Securely expose your dashboard without opening ports.

#### 1. Install cloudflared

```bash
# Download and install
wget https://github.com/cloudflare/cloudflared/releases/latest/download/cloudflared-linux-amd64.deb
sudo dpkg -i cloudflared-linux-amd64.deb
```

#### 2. Authenticate

```bash
cloudflared tunnel login
```

#### 3. Create Tunnel

```bash
# Create tunnel
cloudflared tunnel create vps-monitor

# Copy credentials
cp ~/.cloudflared/<TUNNEL-ID>.json cloudflared-config/credentials.json
```

#### 4. Configure Tunnel

Edit `cloudflared-config/config.yml`:

```yaml
tunnel: <TUNNEL-ID>
credentials-file: /etc/cloudflared/credentials.json

ingress:
  - hostname: monitor.yourdomain.com
    service: http://localhost:80
  - hostname: api.yourdomain.com
    service: http://localhost:5050
  - service: http_status:404
```

#### 5. Run Tunnel

```bash
# Test
cloudflared tunnel --config cloudflared-config/config.yml run vps-monitor

# Install as service
sudo cloudflared service install
sudo systemctl start cloudflared
sudo systemctl enable cloudflared
```

📖 **Detailed Guide**: See [CLOUDFLARE_TUNNEL_SETUP.md](vps-dashboard/CLOUDFLARE_TUNNEL_SETUP.md)

### PostgreSQL Cluster Setup (Optional)

For high-availability database cluster:

```bash
cd database-cluster
cp .env.example .env
# Edit .env with passwords

# Start cluster
docker-compose up -d

# Verify replication
docker-compose exec master psql -U postgres -c "SELECT * FROM pg_stat_replication;"
```

📖 **Detailed Guide**: See [database-cluster/README.md](database-cluster/README.md)

## 🔒 Security

### Essential Security Measures

#### 1. Change Default Credentials
```bash
# After first login, immediately change admin password
# Navigate to Users page > Edit Admin > Change Password
```

#### 2. Secure JWT Secret
```bash
# Generate a strong random secret
openssl rand -base64 32

# Add to .env
JWT_SECRET=<generated-secret-here>
```

#### 3. Configure Firewall
```bash
# Allow only necessary ports
sudo ufw allow 22/tcp      # SSH
sudo ufw allow 80/tcp      # HTTP
sudo ufw allow 443/tcp     # HTTPS
sudo ufw enable
```

#### 4. Use HTTPS in Production
```bash
# Install certbot for Let's Encrypt
sudo apt install certbot python3-certbot-nginx

# Get SSL certificate
sudo certbot --nginx -d monitor.yourdomain.com
```

#### 5. Secure Environment Variables
- **Never commit** `.env` files to git
- Use `.env.example` as templates
- Store sensitive data in environment variables or secrets management
- Rotate credentials regularly

#### 6. Network Security
- Use **Cloudflare Tunnel** instead of exposing ports directly
- Enable Cloudflare WAF and DDoS protection
- Whitelist IP addresses if possible
- Use VPN for administrative access

#### 7. Database Security
- Regular backups (automated daily)
- Encrypt sensitive data at rest
- Use strong passwords for database users
- Limit database access to localhost only

### Security Checklist

- [ ] Changed default admin password
- [ ] Set strong JWT secret (32+ characters)
- [ ] Configured firewall (UFW/iptables)
- [ ] Enabled HTTPS/SSL certificates
- [ ] Setup automated backups
- [ ] Configured Cloudflare Tunnel
- [ ] Enabled alert notifications
- [ ] Regular security updates scheduled
- [ ] Reviewed user permissions

📖 **More Details**: See [SECURITY.md](vps-dashboard/SECURITY.md)

## 📚 API Documentation

### Authentication

All API endpoints (except `/auth/login`) require JWT authentication.

```bash
# Login
curl -X POST http://localhost:5050/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"admin123"}'

# Response: {"token":"eyJhbGc...","user":{...}}

# Use token in subsequent requests
curl http://localhost:5050/api/system/metrics \
  -H "Authorization: Bearer YOUR_TOKEN"
```

### Main Endpoints

| Endpoint | Method | Description | Auth |
|----------|--------|-------------|------|
| `/auth/login` | POST | User login | No |
| `/auth/me` | GET | Get current user | Yes |
| `/api/system/metrics` | GET | System metrics | Yes |
| `/api/system/history` | GET | Historical metrics | Yes |
| `/api/docker/containers` | GET | List containers | Yes |
| `/api/docker/containers/:id/start` | POST | Start container | Yes |
| `/api/docker/containers/:id/stop` | POST | Stop container | Yes |
| `/api/docker/containers/:id/logs` | GET | Container logs | Yes |
| `/api/pm2/processes` | GET | List PM2 processes | Yes |
| `/api/pm2/processes/:id/restart` | POST | Restart process | Yes |
| `/api/projects` | GET | List projects | Yes |
| `/api/projects/:id/deploy` | POST | Deploy project | Yes |
| `/api/health` | GET | Health checks | Yes |
| `/api/alerts` | GET | Alert rules | Yes |
| `/api/events` | GET | System events | Yes |
| `/api/backups` | GET | List backups | Yes |

### Real-time Updates

The dashboard uses Server-Sent Events (SSE) for real-time updates:

```javascript
const eventSource = new EventSource('http://localhost:5050/api/system/stream');

eventSource.onmessage = (event) => {
  const metrics = JSON.parse(event.data);
  console.log('CPU:', metrics.cpu, 'Memory:', metrics.memory);
};
```

### Response Format

```json
{
  "success": true,
  "data": { ... },
  "message": "Operation completed",
  "timestamp": "2026-08-29T18:54:43Z"
}
```

## 🔧 Troubleshooting

### Common Issues

#### Backend Won't Start

**Problem**: `bind: address already in use`
```bash
# Find process using port 5050
sudo lsof -i :5050
# Kill the process
sudo kill -9 <PID>
```

**Problem**: `failed to open database`
```bash
# Check permissions
ls -la backend-go/data/
# Fix permissions
chmod 755 backend-go/data
chmod 644 backend-go/data/vps-dashboard.db
```

#### Docker Connection Failed

**Problem**: `Cannot connect to Docker daemon`
```bash
# Start Docker
sudo systemctl start docker

# Add user to docker group
sudo usermod -aG docker $USER
newgrp docker

# Verify
docker ps
```

#### Frontend Build Fails

**Problem**: `npm ERR! code ELIFECYCLE`
```bash
# Clear cache and reinstall
rm -rf node_modules package-lock.json
npm cache clean --force
npm install
```

#### Metrics Not Updating

**Problem**: Real-time metrics not showing
- Check if backend is running: `curl http://localhost:5050/health`
- Verify SSE connection in browser DevTools (Network tab)
- Check CORS settings if frontend on different domain
- Ensure firewall allows port 5050

#### PM2 Commands Not Working

**Problem**: `pm2: command not found`
```bash
# Install PM2 globally
npm install -g pm2

# Or use with npx
npx pm2 list
```

#### High CPU/Memory Usage

**Problem**: Dashboard consuming too many resources
- Increase `METRICS_INTERVAL` in `.env` (default: 30s)
- Reduce `METRICS_RETENTION_DAYS` to keep less history
- Disable unused features (health checks, backups)
- Check for Docker container issues

### Logs Location

```bash
# Backend logs
docker logs vps-dashboard-backend

# Frontend logs
docker logs vps-dashboard-frontend

# Cloudflared logs
sudo journalctl -u cloudflared -f

# Nginx logs
sudo tail -f /var/log/nginx/error.log
```

### Getting Help

- 📖 Check [Documentation](vps-dashboard/)
- 🐛 Report issues: [GitHub Issues](https://github.com/KarmaSenzu/Monitoring-Server/issues)
- 💬 Ask questions in [Discussions](https://github.com/KarmaSenzu/Monitoring-Server/discussions)

## 🤝 Contributing

Contributions are welcome! Here's how you can help:

### Reporting Bugs

1. Check if the bug already exists in [Issues](https://github.com/KarmaSenzu/Monitoring-Server/issues)
2. Create a new issue with:
   - Clear description of the problem
   - Steps to reproduce
   - Expected vs actual behavior
   - System information (OS, Go version, Docker version)
   - Relevant logs or screenshots

### Suggesting Features

1. Open a [Discussion](https://github.com/KarmaSenzu/Monitoring-Server/discussions) first
2. Describe the feature and use case
3. If approved, create a feature request issue

### Pull Requests

1. Fork the repository
2. Create a feature branch: `git checkout -b feature/amazing-feature`
3. Make your changes
4. Write/update tests if applicable
5. Ensure code passes linting: `go fmt`, `npm run lint`
6. Commit with clear messages: `git commit -m "Add amazing feature"`
7. Push to your fork: `git push origin feature/amazing-feature`
8. Open a Pull Request with description of changes

### Development Setup

```bash
# Clone your fork
git clone https://github.com/YOUR_USERNAME/Monitoring-Server.git
cd Monitoring-Server

# Backend development
cd vps-dashboard/backend-go
go run ./cmd/api

# Frontend development
cd ../frontend
npm install
npm run dev
```

## 📄 License

This project is licensed under the MIT License - see the LICENSE file for details.

## 🙏 Acknowledgments

- Built with [Go](https://golang.org/), [React](https://reactjs.org/), and [Docker](https://www.docker.com/)
- Icons from [Heroicons](https://heroicons.com/)
- Inspired by modern DevOps monitoring tools

## 📞 Contact

- **Repository**: [https://github.com/KarmaSenzu/Monitoring-Server](https://github.com/KarmaSenzu/Monitoring-Server)
- **Issues**: [https://github.com/KarmaSenzu/Monitoring-Server/issues](https://github.com/KarmaSenzu/Monitoring-Server/issues)

---

⭐ **Star this repository** if you find it helpful!

