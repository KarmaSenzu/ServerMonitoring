# vps-dashboard-api (Go)

A pure-Go rewrite of the VPS dashboard backend. This is the Wave 1.1 foundation:
configuration, SQLite + migrations, auth primitives (bcrypt + HS256 JWT),
admin bootstrap, and a `/health` endpoint. Auth and resource routes land in
later waves.

The existing Node `backend/` is left untouched.

## Prerequisites

- Go 1.22 or newer (`go version`)
- macOS or Linux for development

The SQLite driver (`modernc.org/sqlite`) is pure Go, so no CGO toolchain is
required.

## Configure

```sh
cp .env.example .env
```

Then edit `.env` and set at minimum:

- `JWT_SECRET` — long random string. Generate one with `openssl rand -hex 64`.
- `BOOTSTRAP_ADMIN_PASSWORD` — required only on first run when the `users`
  table is empty.

All variables are documented in `.env.example`.

## Run locally

```sh
go run ./cmd/api
```

Defaults:

- HTTP listen: `:3001` (matches the existing `nginx-dashboard.conf` upstream).
- Database file: `./data/vps-dashboard.db` (parent directory is auto-created).
- Migrations under `internal/db/migrations/` are auto-applied on startup.
- On first run, an `admin` user is created from `BOOTSTRAP_ADMIN_USERNAME`
  and `BOOTSTRAP_ADMIN_PASSWORD`. On subsequent runs this is a no-op.

Smoke test:

```sh
curl -s http://localhost:3001/health
# {"status":"ok","timestamp":"...","version":"go-1.0.0"}
```

## Build

Local development binary:

```sh
go build -o bin/vps-dashboard-api ./cmd/api
```

Cross-compile from macOS to the Ubuntu 24.04 VPS (linux/amd64, fully static):

```sh
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o vps-dashboard-api ./cmd/api
```

The resulting `vps-dashboard-api` binary is statically linked and can be
copied to the VPS as-is.

## Project layout

```
backend-go/
├── cmd/api/                # entry point
└── internal/
    ├── app/                # shared dependency container
    ├── auth/               # bcrypt, JWT, admin bootstrap
    ├── config/             # env-driven Config
    ├── db/                 # sqlite open + embedded migrations
    │   └── migrations/
    ├── httpx/              # gin router, middleware, handlers
    │   ├── handlers/
    │   └── middleware/
    └── models/             # data access (User repo)
```

## Notes

- Logging is zerolog. Pretty console output in `development`, JSON in
  `production`.
- Every request gets an `X-Request-ID` (generated if not provided) which is
  echoed back in responses and included in log lines.
- CORS allowed origins are configured via `CORS_ORIGINS` (comma-separated).
