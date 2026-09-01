# PROJECT ARCHITECTURE
# Infrastructure Monitoring & Management Platform

> **Architecture Specification for Monitoring-Server**
>
> This document defines the target architecture for transforming the existing VPS Monitoring Server into a centralized **Infrastructure Monitoring & Management Platform**, inspired by the infrastructure-management concepts of [Purple](https://github.com/erickochen/purple) while preserving and extending the existing Monitoring-Server capabilities.

---

# 1. Project Vision

The project is evolving from:

> **VPS Monitoring Server**

into:

> **Infrastructure Monitoring & Management Platform**

The platform is not intended to be only a monitoring dashboard.

It should become a centralized control center where users can:

- Monitor infrastructure
- Manage servers
- Connect through SSH
- Manage Docker/Podman containers
- Manage PM2 applications
- Execute commands
- Manage files
- Manage SSH tunnels
- Deploy applications
- Manage backups
- Discover infrastructure from cloud providers
- Configure alerts
- Audit infrastructure operations
- Eventually interact with infrastructure through AI/MCP

The core philosophy is:

```text
MONITOR
   ↓
UNDERSTAND
   ↓
INVESTIGATE
   ↓
ACT
   ↓
VERIFY
```

---

# 2. Reference Projects

## 2.1 Existing Project

Primary codebase:

```text
https://github.com/KarmaSenzu/Monitoring-Server
```

The existing project already provides:

- Real-time CPU, memory, disk and network monitoring
- Historical metrics
- Docker management
- PM2 monitoring
- Health checks
- Event logging
- Project discovery
- Deployment
- Backup and rollback
- Environment management
- Docker Compose / PM2 configuration generation
- Telegram notifications
- Webhook notifications
- Authentication
- Role-based access
- SQLite
- Optional PostgreSQL HA infrastructure
- Cloudflare Tunnel
- React/Vite dashboard
- Go backend

These capabilities must be preserved unless there is a clear technical reason to replace them.

---

## 2.2 Purple

Infrastructure-management reference:

```text
https://github.com/erickochen/purple
```

Purple should be treated as an architectural and UX reference.

Important concepts to adopt:

- SSH host management
- SSH configuration
- Multi-host operations
- Cloud provider discovery
- Docker/Podman fleet management
- SSH tunnel management
- SSH key management
- File management
- Command snippets
- Infrastructure search
- Vault SSH certificates
- MCP/AI integration
- Read-only AI infrastructure access
- Auditability
- Agentless infrastructure operations

Purple must **not** simply be copied into the project.

The goal is:

```text
Monitoring-Server
        +
Purple concepts
        ↓
Infrastructure Monitoring & Management Platform
```

---

# 3. Current Repository Architecture

The current repository is approximately:

```text
Monitoring-Server/
│
├── vps-dashboard/
│   │
│   ├── backend-go/
│   │   ├── cmd/api/
│   │   ├── internal/
│   │   │   ├── app/
│   │   │   ├── auth/
│   │   │   ├── config/
│   │   │   ├── db/
│   │   │   │   └── migrations/
│   │   │   ├── httpx/
│   │   │   │   ├── handlers/
│   │   │   │   └── middleware/
│   │   │   └── models/
│   │   ├── data/
│   │   ├── Dockerfile
│   │   ├── go.mod
│   │   └── go.sum
│   │
│   ├── frontend/
│   │   ├── public/
│   │   ├── src/
│   │   ├── Dockerfile
│   │   ├── nginx.conf
│   │   ├── package.json
│   │   └── vite.config.js
│   │
│   ├── cloudflared-config/
│   ├── scripts/
│   └── docker-compose.yml
│
├── database-cluster/
│   ├── config/
│   ├── init-scripts/
│   ├── pgadmin/
│   ├── scripts/
│   ├── docker-compose.yml
│   └── README.md
│
└── README.md
```

The existing repository already separates:

```text
Application
    ↓
vps-dashboard

Database Infrastructure
    ↓
database-cluster
```

This separation should be preserved.

---

# 4. Current Backend

The Go backend is currently the foundation of the system.

Current architecture:

```text
backend-go/
│
├── cmd/api/
│
└── internal/
    ├── app/
    ├── auth/
    ├── config/
    ├── db/
    │   └── migrations/
    ├── httpx/
    │   ├── handlers/
    │   └── middleware/
    └── models/
```

The backend currently uses:

- Go
- Gin
- SQLite
- `modernc.org/sqlite`
- Embedded migrations
- JWT
- bcrypt
- Zerolog
- Request IDs
- Configurable CORS

The current backend is therefore a good foundation for adding infrastructure services.

---

# 5. Target Backend Architecture

The backend should evolve toward:

```text
internal/
│
├── app/
│
├── config/
│
├── db/
│   ├── migrations/
│   └── repositories/
│
├── auth/
├── rbac/
│
├── infrastructure/
│   ├── hosts/
│   ├── inventory/
│   ├── discovery/
│   ├── tags/
│   └── search/
│
├── ssh/
│   ├── client/
│   ├── config/
│   ├── keys/
│   ├── sessions/
│   └── tunnels/
│
├── monitoring/
│   ├── metrics/
│   ├── history/
│   ├── health/
│   └── stream/
│
├── docker/
│
├── podman/
│
├── pm2/
│
├── commands/
│   ├── snippets/
│   ├── execution/
│   └── multi_host/
│
├── files/
│
├── cloud/
│   ├── providers/
│   ├── discovery/
│   └── synchronization/
│
├── deploy/
│
├── backup/
│
├── alerts/
│
├── notifier/
│
├── audit/
│
└── mcp/
```

This structure is a **target architecture**.

Do not create all directories immediately.

Create each module only when its feature is implemented.

---

# 6. Most Important New Concept: Server Registry

The most important architectural addition is:

> **Server Registry**

The Server Registry becomes the central identity of every managed server.

Conceptually:

```text
                    SERVER REGISTRY
                          │
         ┌────────────────┼────────────────┐
         │                │                │
         ▼                ▼                ▼
     Monitoring          SSH            Containers
         │                │                │
         ▼                ▼                ▼
      Metrics          Terminal         Docker
      Health           Files            Podman
      Alerts           Tunnel
         │                │
         └────────────────┼────────────────┐
                          │                │
                          ▼                ▼
                      Deployment         Backup
```

---

# 7. Server Entity

The server entity should eventually contain:

```text
Server
├── ID
├── Name
├── Hostname
├── IP Address
├── SSH Port
├── SSH Username
├── SSH Credential Reference
├── Operating System
├── Architecture
├── Provider
├── Provider Instance ID
├── Environment
├── Status
├── Last Seen
├── Created At
└── Updated At
```

Additional relationships:

```text
Server
├── Tags
├── Metrics
├── Health Checks
├── Containers
├── PM2 Processes
├── Deployments
├── Backups
├── Alerts
├── SSH Tunnels
├── SSH Keys
└── Audit Events
```

---

# 8. Server Status Model

Use explicit states:

```text
ONLINE
DEGRADED
OFFLINE
UNKNOWN
```

Example:

```text
ONLINE
    SSH works
    Monitoring works
    Health checks pass

DEGRADED
    SSH works
    But one or more important checks fail

OFFLINE
    SSH unavailable
    Or server unreachable

UNKNOWN
    Server has not been checked recently
```

Do not infer status from a single metric.

---

# 9. Infrastructure Communication Model

The platform should be primarily agentless.

Preferred:

```text
Go Backend
    │
    │ SSH
    ▼
Remote Server
```

Avoid requiring a custom monitoring agent on every server unless a capability genuinely requires one.

The backend should communicate with remote servers using:

- SSH
- Docker CLI/API over SSH where appropriate
- Podman over SSH
- SCP/SFTP
- Cloud APIs
- HTTP health checks

---

# 10. SSH Architecture

Introduce:

```text
internal/ssh/
```

with responsibilities separated into:

```text
ssh/
├── client/
├── config/
├── keys/
├── sessions/
└── tunnels/
```

## SSH Client

Responsible for:

- Connecting
- Authentication
- Command execution
- Timeouts
- Connection errors
- Context cancellation

## SSH Config

Responsible for:

- Host aliases
- Hostname
- Port
- User
- Identity file
- ProxyJump
- Other supported OpenSSH directives

## SSH Sessions

Responsible for:

- Interactive shell
- PTY
- Terminal streaming

## SSH Tunnels

Responsible for:

- Local forwarding
- Remote forwarding
- SOCKS/dynamic forwarding
- Tunnel lifecycle
- Tunnel status

---

# 11. SSH Credentials

Credentials must never be stored directly in ordinary server API responses.

Use:

```text
Server
   ↓
Credential Reference
   ↓
Secure Credential Store
```

The frontend should never receive raw:

- Private keys
- Passwords
- Cloud tokens
- Vault tokens

The API should return only safe metadata.

Example:

```json
{
  "credential": {
    "type": "ssh_key",
    "name": "production-key",
    "fingerprint": "SHA256:..."
  }
}
```

Not:

```json
{
  "private_key": "-----BEGIN..."
}
```

---

# 12. Monitoring Architecture

The existing monitoring system should remain.

Target architecture:

```text
                 MONITORING ENGINE
                        │
        ┌───────────────┼───────────────┐
        ▼               ▼               ▼
      Metrics         Health           Events
        │               │               │
        ▼               ▼               ▼
       CPU             HTTP            Alerts
       RAM             TCP             Audit
       Disk            SSH
       Network         Docker
```

Metrics should support:

- Current state
- Historical state
- Real-time streaming
- Configurable retention

---

# 13. Monitoring vs Management

Do not mix responsibilities.

Monitoring answers:

```text
What is happening?
```

Management answers:

```text
What can I do?
```

Example:

```text
Monitoring:
CPU = 95%

Management:
Open terminal
Inspect process
Inspect container
Restart service
```

---

# 14. Docker / Podman Architecture

Existing Docker functionality should be preserved.

However, the architecture should eventually support:

```text
Container Engine
├── Docker
└── Podman
```

Both should expose a common abstraction:

```text
ContainerService
├── List
├── Inspect
├── Logs
├── Start
├── Stop
├── Restart
├── Exec
└── Remove
```

The frontend should not care whether the remote host uses Docker or Podman.

---

# 15. Multi-Server Container View

The UI should eventually support:

```text
ALL CONTAINERS

server-01
├── nginx
├── api
├── postgres

server-02
├── nginx
├── backend

server-03
├── redis
└── worker
```

This is one of the major capabilities inspired by Purple.

---

# 16. PM2

The existing PM2 module should remain.

PM2 belongs under:

```text
Process Management
```

rather than being mixed with Docker.

Target:

```text
Process Management
├── PM2
├── Systemd
└── Other process managers
```

Future support can be added without redesigning the whole system.

---

# 17. Command Engine

Introduce:

```text
internal/commands/
```

Responsibilities:

```text
commands/
├── snippets/
├── execution/
└── multi_host/
```

A command execution should contain:

```text
Command Run
├── User
├── Server
├── Command
├── Started At
├── Finished At
├── Exit Code
├── stdout
├── stderr
└── Status
```

---

# 18. Multi-Host Command Execution

Multi-host execution must be independent.

Example:

```text
server-01    SUCCESS
server-02    SUCCESS
server-03    TIMEOUT
server-04    SUCCESS
```

A failure on one server must not automatically terminate execution on other servers.

Use bounded concurrency.

Do not spawn unlimited goroutines.

---

# 19. Blast Radius Protection

Before executing a multi-host operation:

```text
TARGETS

4 servers

COMMAND

docker restart api
```

The user must be shown:

```text
4 hosts will be affected.
```

Dangerous commands require explicit confirmation.

For example:

```text
rm
shutdown
reboot
systemctl stop
docker rm
docker system prune
database destructive commands
```

should be treated as high-risk.

---

# 20. Command Snippets

Create reusable commands:

```text
Snippet
├── ID
├── Name
├── Description
├── Command
├── Variables
├── Created By
├── Updated By
└── Created At
```

Example:

```text
Name:
Check Docker

Command:
docker ps
```

Variables:

```text
docker logs {{container}}
```

---

# 21. File Manager

Introduce:

```text
internal/files/
```

Capabilities:

```text
Browse
Upload
Download
Rename
Delete
Create Directory
Metadata
```

Preferred transport:

```text
SFTP / SCP over SSH
```

Never expose arbitrary filesystem paths without authorization.

---

# 22. SSH Tunnel Manager

Introduce:

```text
internal/ssh/tunnels/
```

Tunnel:

```text
Tunnel
├── ID
├── Name
├── Server
├── Type
├── Local Address
├── Remote Address
├── Status
├── Started At
└── User
```

Possible types:

```text
LOCAL
REMOTE
SOCKS
```

---

# 23. Cloud Discovery

Introduce:

```text
internal/cloud/
```

Architecture:

```text
Cloud Provider
      ↓
Provider Adapter
      ↓
Discovery
      ↓
Server Registry
```

Provider abstraction:

```go
type Provider interface {
    ListInstances(ctx context.Context) ([]Instance, error)
    GetInstance(ctx context.Context, id string) (*Instance, error)
}
```

Each provider should be isolated.

Example:

```text
cloud/
├── providers/
│   ├── aws/
│   ├── gcp/
│   ├── azure/
│   └── hetzner/
└── discovery/
```

Do not hard-code AWS logic into the Server Registry.

---

# 24. Cloud Discovery Safety

Discovery must not automatically mean management authorization.

Correct:

```text
AWS
 ↓
Instance discovered
 ↓
Server Registry
 ↓
User approves management
 ↓
SSH configured
```

Do not automatically grant administrative access simply because an instance was discovered.

---

# 25. Tags

Introduce:

```text
tags
server_tags
```

Example:

```text
production
database
web
critical
monitoring
development
aws
hetzner
```

Tags should support filtering:

```text
tag:production
tag:database
tag:critical
```

---

# 26. Global Search

Create a global infrastructure search.

UI:

```text
Ctrl + K

Search infrastructure...
```

Search across:

```text
Servers
Containers
PM2 processes
Tags
Commands
Tunnels
Cloud resources
Alerts
Deployments
Backups
```

This is inspired by Purple's infrastructure-oriented search model.

---

# 27. Deployment Architecture

Keep:

```text
internal/deploy/
```

Deployment flow:

```text
User
 ↓
Select Project
 ↓
Select Server
 ↓
Preflight Check
 ↓
Backup
 ↓
Deploy
 ↓
Health Check
 ↓
Success
```

Failure:

```text
Deploy failed
     ↓
Health check failed
     ↓
Rollback
     ↓
Audit event
     ↓
Alert
```

---

# 28. Backup Architecture

Keep:

```text
internal/backup/
```

Backup should be connected to:

```text
Server
Project
Deployment
Database
```

Track:

```text
Backup
├── Server
├── Type
├── Size
├── Location
├── Created At
├── Status
└── Retention
```

---

# 29. Alert Architecture

Existing alerting should remain.

Target:

```text
Metric
 ↓
Rule
 ↓
Evaluation
 ↓
Alert Event
 ↓
Notification
```

Examples:

```text
CPU > 80%
Memory > 85%
Disk > 90%
Server offline
Container stopped
Health check failed
Deployment failed
Backup failed
```

---

# 30. Notification System

Keep the existing notifier architecture.

Support:

```text
Telegram
Webhook
```

Future:

```text
Email
Discord
Slack
PagerDuty
```

should be added through a common notification interface.

---

# 31. Audit System

Create:

```text
internal/audit/
```

Every infrastructure-changing operation must create an audit event.

Example:

```text
User:
admin

Action:
restart_container

Server:
production-01

Target:
api

Result:
success

Timestamp:
2026-08-29T20:30:00Z
```

Audit logs must never contain secrets.

---

# 32. RBAC

Expand current roles.

Minimum:

```text
ADMIN
OPERATOR
VIEWER
```

## VIEWER

Can:

- View servers
- View metrics
- View containers
- View alerts
- View logs

Cannot:

- Execute commands
- Restart containers
- Deploy
- Delete
- Modify infrastructure

## OPERATOR

Can:

- Everything Viewer can
- Restart containers
- Run approved commands
- Deploy
- Backup

## ADMIN

Can:

- Everything
- Manage users
- Manage credentials
- Manage providers
- Configure infrastructure
- Configure permissions

---

# 33. Frontend Architecture

The existing frontend uses:

```text
React
Vite
JavaScript
CSS
Chart.js
```

Do not rewrite the frontend immediately.

First reorganize it logically.

Target:

```text
src/
├── api/
├── auth/
├── components/
├── features/
│   ├── dashboard/
│   ├── servers/
│   ├── monitoring/
│   ├── containers/
│   ├── terminal/
│   ├── commands/
│   ├── deployments/
│   ├── backups/
│   ├── files/
│   ├── tunnels/
│   ├── cloud/
│   ├── alerts/
│   └── audit/
├── pages/
├── layouts/
├── hooks/
└── utils/
```

Do not move everything at once.

---

# 34. Main Navigation

Recommended navigation:

```text
Dashboard

Infrastructure
├── Servers
├── Containers
├── Cloud
└── Search

Operations
├── Terminal
├── Commands
├── Files
├── Tunnels
├── Deployments
└── Backups

Monitoring
├── Overview
├── Metrics
├── Health
└── Alerts

Security
├── SSH Keys
├── Users
├── Roles
└── Audit Logs
```

---

# 35. Dashboard Design

The dashboard should prioritize infrastructure state.

Example:

```text
INFRASTRUCTURE OVERVIEW

Servers
12 Online
1 Offline
2 Degraded

Containers
37 Running
2 Stopped

Alerts
2 Critical
5 Warning
```

Then:

```text
Critical Infrastructure

database-01
Memory 94%

production-03
Disk 91%

api-01
Health check failed
```

Then:

```text
Recent Events
```

---

# 36. Server Detail Page

The server page is the most important page.

It should combine:

```text
Monitoring
+
Management
```

Example:

```text
production-01

ONLINE

CPU       42%
Memory    61%
Disk      72%
Network   13 MB/s

[Terminal]
[Containers]
[Files]
[Commands]
[Deploy]
[Backup]
[Tunnel]
```

Below:

```text
CPU History
Memory History
Disk History
Network History
```

Then:

```text
Containers
PM2
Health Checks
Recent Events
```

---

# 37. Terminal Architecture

Interactive terminal should use:

```text
Browser
   │
WebSocket
   │
Go Backend
   │
SSH PTY
   │
Remote Server
```

Do not implement an interactive terminal through ordinary HTTP polling.

Use proper session lifecycle management.

---

# 38. Real-Time Architecture

Existing SSE should remain useful for monitoring.

Use:

```text
SSE
```

for:

- Metrics
- Alerts
- Events
- Health changes

Use:

```text
WebSocket
```

for:

- Interactive terminal
- Potential future live command streams

Do not replace SSE simply because WebSockets exist.

---

# 39. Database Strategy

Current application database:

```text
SQLite
```

should remain the default for local/small deployments.

The PostgreSQL cluster remains a separate infrastructure option.

Do not force every installation to run PostgreSQL.

The target should support:

```text
Small installation
    ↓
SQLite

Larger installation
    ↓
PostgreSQL / TimescaleDB
```

The application layer should minimize database-specific assumptions.

---

# 40. PostgreSQL / TimescaleDB

The existing `database-cluster` already provides:

```text
PostgreSQL 16
TimescaleDB
Master
Replica 1
Replica 2
pgAdmin
Backups
```

This should be treated as an optional production infrastructure component.

Do not make the application dependent on the cluster during the initial infrastructure-management implementation.

---

# 41. Metrics Storage Strategy

Metrics are time-series data.

For SQLite:

```text
metrics
```

can remain optimized for the existing deployment.

For PostgreSQL:

```text
TimescaleDB
```

can later provide:

- Compression
- Retention
- Time-series partitioning
- Larger historical datasets

The application should expose a common metrics repository interface.

---

# 42. API Architecture

Existing API conventions should remain.

Current endpoints include:

```text
/auth/login
/auth/me

/api/system/metrics
/api/system/history
/api/system/stream

/api/docker/containers
/api/docker/containers/:id/start
/api/docker/containers/:id/stop
/api/docker/containers/:id/logs

/api/pm2/processes
/api/pm2/processes/:id/restart

/api/projects
/api/projects/:id/deploy

/api/health
/api/alerts
/api/events
/api/backups
```

New infrastructure APIs should follow the same `/api/...` convention.

---

# 43. Proposed Infrastructure APIs

## Servers

```text
GET    /api/servers
POST   /api/servers
GET    /api/servers/:id
PATCH  /api/servers/:id
DELETE /api/servers/:id
POST   /api/servers/:id/test
```

## SSH

```text
POST /api/servers/:id/ssh/test
POST /api/servers/:id/ssh/command
POST /api/servers/:id/ssh/session
```

## Containers

```text
GET  /api/servers/:id/containers
GET  /api/servers/:id/containers/:container
POST /api/servers/:id/containers/:container/start
POST /api/servers/:id/containers/:container/stop
POST /api/servers/:id/containers/:container/restart
GET  /api/servers/:id/containers/:container/logs
```

## Commands

```text
GET  /api/commands/snippets
POST /api/commands/snippets
POST /api/commands/run
POST /api/commands/run/multi
```

## Tunnels

```text
GET    /api/tunnels
POST   /api/tunnels
DELETE /api/tunnels/:id
```

## Files

```text
GET  /api/servers/:id/files
POST /api/servers/:id/files/upload
GET  /api/servers/:id/files/download
```

## Cloud

```text
GET  /api/cloud/providers
POST /api/cloud/providers
POST /api/cloud/:provider/discover
```

These are proposed APIs, not requirements to implement immediately.

---

# 44. Error Model

Infrastructure APIs should return meaningful errors.

Example:

```json
{
  "success": false,
  "error": {
    "code": "SSH_CONNECTION_TIMEOUT",
    "message": "Unable to connect to production-01 within 10 seconds"
  },
  "timestamp": "2026-08-29T20:30:00Z"
}
```

Avoid returning raw internal errors to users.

---

# 45. Timeouts

Every infrastructure operation must have a timeout.

Examples:

```text
SSH connection
10 seconds

Command
30 seconds default

Cloud discovery
30-60 seconds

Health check
5-10 seconds

File transfer
configurable
```

Long-running operations should become asynchronous jobs where appropriate.

---

# 46. Concurrency

Never create uncontrolled concurrency for:

- Server discovery
- Monitoring
- Multi-host commands
- Container operations
- Cloud API calls

Use:

```text
Worker Pool
+
Context Cancellation
+
Timeout
+
Rate Limiting
```

---

# 47. Security Requirements

Infrastructure management is high-risk.

Mandatory principles:

```text
Least Privilege
Explicit Authorization
Secure Credentials
Audit Everything
Never Log Secrets
Fail Safely
```

Never expose:

```text
SSH private keys
Passwords
Cloud API tokens
JWT secrets
Database passwords
Vault tokens
Environment secrets
```

to the frontend.

---

# 48. AI / MCP Future Architecture

AI should be added after the infrastructure foundation is stable.

Target:

```text
AI Agent
   │
   ▼
MCP
   │
   ▼
Infrastructure API
   │
   ├── Monitoring
   ├── Servers
   ├── Containers
   ├── Alerts
   └── Audit
```

Initial AI mode:

```text
READ ONLY
```

Examples:

```text
Which servers are offline?

Which server has the highest RAM usage?

What containers run on production-01?

What alerts occurred today?
```

---

# 49. AI Action Model

Later:

```text
AI
 ↓
Propose Action
 ↓
User Approval
 ↓
Infrastructure API
 ↓
Execution
 ↓
Audit
```

Never:

```text
AI
 ↓
Root shell
 ↓
Anything
```

Dangerous operations require explicit user confirmation.

---

# 50. Purple Feature Mapping

The following mapping should guide implementation.

| Purple Concept | New Platform |
|---|---|
| SSH host manager | Server Registry + SSH |
| SSH config editor | SSH Config |
| Cloud sync | Cloud Discovery |
| Docker fleet | Container Management |
| Podman fleet | Container abstraction |
| SSH tunnel | Tunnel Manager |
| SCP/file browser | File Manager |
| SSH key push | SSH Key Management |
| Command snippets | Command Engine |
| Multi-host commands | Multi-Server Operations |
| Fuzzy search | Global Infrastructure Search |
| Vault SSH certificates | Future Security Integration |
| MCP | Future AI Integration |
| Read-only AI | AI Safety Model |

---

# 51. What Must NOT Be Done

Do not:

```text
Copy the entire Purple repository
```

Do not:

```text
Embed the Purple TUI inside the web application
```

Do not:

```text
Replace the existing monitoring system unnecessarily
```

Do not:

```text
Force PostgreSQL on every installation
```

Do not:

```text
Create a custom agent for every server without necessity
```

Do not:

```text
Give AI unrestricted root access
```

Do not:

```text
Expose SSH private keys through REST APIs
```

Do not:

```text
Run arbitrary commands without RBAC and audit logging
```

---

# 52. Development Phases

## Phase 0 — Stabilization

Before adding major functionality:

- Confirm current backend builds
- Confirm frontend builds
- Confirm database migrations
- Confirm authentication
- Confirm monitoring
- Confirm Docker management
- Confirm existing API behavior

Do not add infrastructure features until the current baseline is understood.

---

## Phase 1 — Server Registry

Implement:

```text
servers
tags
server_tags
```

Create:

```text
GET /api/servers
POST /api/servers
GET /api/servers/:id
PATCH /api/servers/:id
DELETE /api/servers/:id
```

Create the Server Management UI.

---

## Phase 2 — SSH Engine

Implement:

```text
SSH connection
SSH test
Command execution
Credential references
```

Do not implement interactive terminal yet.

First establish a reliable SSH foundation.

---

## Phase 3 — Remote Monitoring

Connect monitoring to the Server Registry.

Current:

```text
Local Server
```

Target:

```text
Server Registry
       ↓
Remote Server
       ↓
Metrics
```

This is a major architectural transition.

---

## Phase 4 — Docker / Podman Fleet

Implement:

```text
Server
 ↓
Container Engine
 ↓
Containers
```

Add:

- List
- Logs
- Start
- Stop
- Restart
- Exec

---

## Phase 5 — Terminal

Implement:

```text
React
 ↓
WebSocket
 ↓
Go
 ↓
SSH PTY
```

---

## Phase 6 — Multi-Host Commands

Implement:

```text
Snippet
 ↓
Server Selection
 ↓
Preview
 ↓
Confirmation
 ↓
Parallel Execution
 ↓
Per-host Results
 ↓
Audit
```

---

## Phase 7 — File Manager

Implement:

```text
SFTP/SCP
```

with safe path handling.

---

## Phase 8 — SSH Tunnels

Implement:

```text
Local Forward
Remote Forward
SOCKS
```

---

## Phase 9 — Cloud Discovery

Start with one provider.

Recommended first provider:

```text
AWS
```

Then build the abstraction.

Do not implement 10+ providers at once.

---

## Phase 10 — Infrastructure Search

Implement:

```text
Ctrl + K
```

Search:

```text
Servers
Containers
Tags
Commands
Alerts
```

---

## Phase 11 — Security & Audit

Strengthen:

- RBAC
- Audit logs
- Credential handling
- Dangerous command confirmation
- Session management

---

## Phase 12 — MCP / AI

Only after the infrastructure APIs are stable.

Start:

```text
READ ONLY
```

Then:

```text
APPROVED ACTIONS
```

Finally:

```text
USER-CONFIRMED DANGEROUS ACTIONS
```

---

# 53. Migration Strategy

Do not make a destructive migration.

The project should evolve:

```text
CURRENT
Local Monitoring
     ↓
Server Registry
     ↓
Remote Monitoring
     ↓
Infrastructure Management
```

Existing functionality should continue working during development.

Every major phase should be independently testable.

---

# 54. Definition of Success

The project is successful when a user can:

### Discover

```text
See all infrastructure
```

### Monitor

```text
See CPU
RAM
Disk
Network
Health
Alerts
```

### Connect

```text
SSH into a server
```

### Operate

```text
View containers
Restart containers
View logs
Run commands
```

### Manage

```text
Files
Tunnels
Deployments
Backups
```

### Automate

```text
Multi-host commands
Cloud discovery
Deployment workflows
```

### Audit

```text
Know who did what and when
```

### Assist

```text
Eventually allow AI to inspect and safely operate infrastructure
```

---

# 55. Final Architecture

The final system should conceptually become:

```text
                           USER
                            │
                            ▼
                  ┌──────────────────┐
                  │   WEB DASHBOARD  │
                  │      React       │
                  └────────┬─────────┘
                           │
                           ▼
                  ┌──────────────────┐
                  │    GO BACKEND    │
                  │   API + RBAC     │
                  └────────┬─────────┘
                           │
        ┌──────────────────┼──────────────────┐
        │                  │                  │
        ▼                  ▼                  ▼
   MONITORING          MANAGEMENT         AUTOMATION
        │                  │                  │
        │                  │                  │
    Metrics               SSH              Deploy
    Health                Docker           Backup
    Alerts                Podman           Commands
    History               Files
    Events                Tunnel
        │                 Cloud
        │                 Keys
        │                  │
        └──────────────────┼──────────────────┘
                           │
                           ▼
                    SERVER REGISTRY
                           │
             ┌─────────────┼─────────────┐
             ▼             ▼             ▼
         SERVER 01      SERVER 02      SERVER 03
             │             │             │
          Docker         Docker        Podman
             │             │             │
             └─────────────┼─────────────┘
                           │
                           ▼
                    INFRASTRUCTURE
```

---

# 56. Final Product Definition

The project should no longer be thought of as:

> "A website that shows VPS CPU/RAM usage."

Instead:

> **Infrastructure Monitoring & Management Platform is a centralized infrastructure control plane that combines monitoring, SSH access, container management, server operations, deployment, backup, cloud discovery, automation, security, and eventually AI-assisted infrastructure management in one web interface.**

The design philosophy is:

```text
PURPLE
Infrastructure Management Concepts
              +
MONITORING-SERVER
Monitoring + Automation + Dashboard
              ↓
INFRASTRUCTURE MONITORING
        & MANAGEMENT
          PLATFORM
```

The project should remain:

- Modular
- Agentless where practical
- Secure
- Auditable
- Extensible
- Web-first
- Infrastructure-oriented
- Compatible with standard SSH
- Capable of managing multiple servers
- Capable of scaling from a single VPS to a small server fleet

---

# 57. Golden Rules for Future Development

Every future contributor or AI coding agent must follow these rules:

1. **Read this document before modifying architecture.**
2. **Inspect the existing code before creating new code.**
3. **Reuse existing functionality whenever possible.**
4. **Do not duplicate existing modules.**
5. **Do not blindly copy Purple source code.**
6. **Use Purple as an architectural reference.**
7. **Do not break existing monitoring features.**
8. **Do not expose credentials.**
9. **Every infrastructure-changing action must be authorized.**
10. **Every infrastructure-changing action must be auditable.**
11. **Dangerous multi-host operations require confirmation.**
12. **Prefer SSH-based agentless operations.**
13. **Use bounded concurrency.**
14. **Every remote operation must have a timeout.**
15. **Handle partial failures gracefully.**
16. **Keep SQLite viable for small deployments.**
17. **Keep PostgreSQL/TimescaleDB optional for larger deployments.**
18. **Do not add unnecessary infrastructure dependencies.**
19. **Do not rewrite working components without justification.**
20. **Implement the platform incrementally, phase by phase.**

---

# 58. Current Priority

At the current stage of the project, the immediate priority is **NOT**:

```text
AI
Cloud Provider x16
Vault
Advanced tunnels
```

The immediate priority is:

```text
1. Server Registry
2. SSH Engine
3. Remote Server Connection
4. Remote Monitoring
5. Docker Fleet
6. Terminal
7. Multi-host Commands
```

Once these foundations are stable, advanced capabilities can be layered on top.

The most important architectural transition is:

```text
CURRENT:

Dashboard
    ↓
Local Server


TARGET:

Dashboard
    ↓
Server Registry
    ↓
Multiple Remote Servers
```

This transition creates the foundation for the entire **Infrastructure Monitoring & Management Platform**.