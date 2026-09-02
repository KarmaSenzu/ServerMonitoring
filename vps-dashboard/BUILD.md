# Build Instructions

VPS Dashboard can be built as a **single binary** with embedded frontend for easy distribution across platforms.

## Quick Start

```bash
# Build single binary for current platform
./scripts/build.sh

# Build for all platforms (Linux, macOS, Windows)
./scripts/build-all.sh

# Binary output
./vpsdash
```

## Prerequisites

### Required
- **Go 1.23+** - Backend compilation
- **Node.js 20+** & npm - Frontend build
- **Git** - Version control (optional)

### Platform-Specific
- **Linux/macOS**: bash, tar, chmod
- **Windows**: PowerShell or WSL

## Build Process

### 1. Single Binary Build

The build process:
1. Builds React frontend → `frontend/dist/`
2. Embeds dist/ into Go binary via `//go:embed`
3. Compiles Go binary with embedded assets
4. Results in single executable (~15-20MB)

**Build for current platform:**

```bash
cd vps-dashboard
./scripts/build.sh
```

**Output:** `./vpsdash` (or `vpsdash.exe` on Windows)

### 2. Cross-Platform Build

Build for multiple platforms simultaneously:

```bash
./scripts/build-all.sh
```

**Output:** `dist/` directory containing:
```
dist/
├── vpsdash-linux-amd64
├── vpsdash-linux-arm64
├── vpsdash-darwin-amd64
├── vpsdash-darwin-arm64
├── vpsdash-windows-amd64.exe
├── vpsdash-windows-arm64.exe
├── vpsdash-v1.0.0-linux-amd64.tar.gz
├── vpsdash-v1.0.0-darwin-arm64.tar.gz
└── SHA256SUMS
```

### 3. Manual Build Steps

If you prefer to build manually:

```bash
# 1. Build frontend
cd frontend
npm ci
npm run build
# Output: frontend/dist/

# 2. Build Go binary (from project root)
cd ../backend-go
go build -ldflags="-s -w \
  -X 'vps-dashboard-api/internal/app.Version=v1.0.0' \
  -X 'vps-dashboard-api/internal/app.BuildCommit=$(git rev-parse --short HEAD)' \
  -X 'vps-dashboard-api/internal/app.BuildTime=$(date -u +%Y-%m-%d_%H:%M:%S)'" \
  -o ../vpsdash \
  cmd/api/main.go
```

### 4. Build Without Frontend

To build backend-only (for development):

```bash
# Skip frontend build, Go will embed empty filesystem
cd backend-go
go build -o vpsdash cmd/api/main.go
```

⚠️ **Note:** Binary will run but won't serve frontend UI.

## Build Configuration

### Version Information

Set via ldflags during build:

```bash
VERSION="v1.2.3"
COMMIT="abc123"
BUILD_TIME="2026-09-02_10:00:00"

go build -ldflags="-X 'vps-dashboard-api/internal/app.Version=${VERSION}' \
  -X 'vps-dashboard-api/internal/app.BuildCommit=${COMMIT}' \
  -X 'vps-dashboard-api/internal/app.BuildTime=${BUILD_TIME}'" \
  -o vpsdash cmd/api/main.go
```

### Build Flags

Recommended ldflags:
- `-s` - Strip debug symbols (reduces size)
- `-w` - Strip DWARF debug info (reduces size)
- `-X` - Set package variables at build time

### Binary Size Optimization

Standard build: ~20MB
Compressed (UPX): ~7-8MB

```bash
# Optional: Compress with UPX
upx --best --lzma vpsdash
```

## Platform-Specific Notes

### Linux
- Requires glibc (or use CGO_ENABLED=0 for static binary)
- ARM64 support for Raspberry Pi / ARM servers

**Static binary (no libc dependency):**
```bash
CGO_ENABLED=0 go build ...
```

### macOS
- Universal binary requires separate amd64/arm64 builds + lipo
- Code signing required for distribution (optional for personal use)

**Ad-hoc sign (bypass Gatekeeper warning):**
```bash
codesign -s - vpsdash
```

### Windows
- No special requirements
- Creates `.exe` executable
- PowerShell or WSL recommended for build scripts

## Verification

### Check Build Info

```bash
./vpsdash --version
# Output:
# VPS Dashboard v1.0.0
# Build Commit: abc123
# Build Time:   2026-09-02_10:00:00
# Frontend:     embedded
```

### Test Binary

```bash
# Run with test config
export JWT_SECRET="test-secret-key-32-chars-long"
./vpsdash

# Access dashboard
open http://localhost:3001
```

## Distribution

### GitHub Releases

Automated via GitHub Actions:

```yaml
# .github/workflows/release.yml
on:
  push:
    tags: ['v*']
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - run: ./scripts/build-all.sh
      - uses: softprops/action-gh-release@v1
        with:
          files: dist/*
```

### Installation Methods

**1. Direct Download**
```bash
# Download latest release
curl -LO https://github.com/user/repo/releases/latest/download/vpsdash-linux-amd64.tar.gz
tar xzf vpsdash-linux-amd64.tar.gz
sudo mv vpsdash /usr/local/bin/
```

**2. Installation Script**
```bash
curl -sSL https://raw.githubusercontent.com/user/repo/main/install.sh | bash
```

**3. Package Managers**
```bash
# Homebrew (after tap creation)
brew install user/tap/vpsdash

# Go install
go install github.com/user/repo/cmd/vpsdash@latest
```

## Troubleshooting

### Frontend Not Embedded

**Symptom:** `frontend_embedded: false` in startup logs

**Solution:** Build frontend first
```bash
cd frontend && npm run build
```

### Binary Too Large

**Symptom:** Binary >30MB

**Causes:**
- Debug symbols included (use `-s -w`)
- Not using production React build
- CGO enabled (use `CGO_ENABLED=0`)

### Module Issues

**Symptom:** `package not found` errors

**Solution:** Update dependencies
```bash
cd backend-go
go mod tidy
go mod download
```

## Development Builds

For rapid iteration during development:

```bash
# Backend only (no frontend embed)
cd backend-go
go run cmd/api/main.go

# Frontend dev server (separate terminal)
cd frontend
npm run dev  # http://localhost:5173
```

Frontend dev server proxies API requests to `:3001`.

## CI/CD Integration

### Docker Build

```dockerfile
# Multi-stage build
FROM node:20 AS frontend
WORKDIR /app/frontend
COPY frontend/package*.json ./
RUN npm ci
COPY frontend/ ./
RUN npm run build

FROM golang:1.23 AS backend
WORKDIR /app
COPY --from=frontend /app/frontend/dist/ /app/frontend/dist/
COPY backend-go/ ./backend-go/
WORKDIR /app/backend-go
RUN go build -o /vpsdash cmd/api/main.go

FROM debian:bookworm-slim
COPY --from=backend /vpsdash /usr/local/bin/
CMD ["vpsdash"]
```

## Support

- **Documentation:** [README.md](README.md)
- **Issues:** https://github.com/user/repo/issues
- **Discussions:** https://github.com/user/repo/discussions
