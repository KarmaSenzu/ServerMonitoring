#!/bin/bash
# Build script for VPS Dashboard single binary
# Builds frontend and embeds it into Go binary

set -e  # Exit on error

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Build configuration
VERSION="${VERSION:-dev}"
COMMIT="${COMMIT:-$(git rev-parse --short HEAD 2>/dev/null || echo 'unknown')}"
BUILD_TIME="$(date -u '+%Y-%m-%d_%H:%M:%S')"
BINARY_NAME="${BINARY_NAME:-vpsdash}"

# Detect OS and architecture
GOOS="${GOOS:-$(go env GOOS)}"
GOARCH="${GOARCH:-$(go env GOARCH)}"

echo -e "${BLUE}=== VPS Dashboard Build Script ===${NC}"
echo -e "${BLUE}Version:${NC}     ${VERSION}"
echo -e "${BLUE}Commit:${NC}      ${COMMIT}"
echo -e "${BLUE}Build Time:${NC}  ${BUILD_TIME}"
echo -e "${BLUE}Target:${NC}      ${GOOS}/${GOARCH}"
echo ""

# Navigate to project root
cd "$(dirname "$0")/.."

# Step 1: Build Frontend
echo -e "${YELLOW}Step 1/3: Building frontend...${NC}"
cd frontend

# Check if node_modules exists
if [ ! -d "node_modules" ]; then
    echo -e "${YELLOW}Installing frontend dependencies...${NC}"
    npm ci
fi

# Build frontend
echo -e "${YELLOW}Building React app with Vite...${NC}"
npm run build

# Verify dist directory was created
if [ ! -d "dist" ] || [ ! -f "dist/index.html" ]; then
    echo -e "${RED}❌ Frontend build failed - dist/index.html not found${NC}"
    exit 1
fi

echo -e "${GREEN}✓ Frontend built successfully${NC}"
echo ""

# Step 1.5: Copy dist to backend embed tree
echo -e "${YELLOW}Copying dist to backend-go/internal/httpx/dist...${NC}"
cd ..
mkdir -p backend-go/internal/httpx/dist
cp -r frontend/dist/* backend-go/internal/httpx/dist/

if [ ! -f "backend-go/internal/httpx/dist/index.html" ]; then
    echo -e "${RED}❌ Failed to copy dist - backend-go/internal/httpx/dist/index.html not found${NC}"
    exit 1
fi

echo -e "${GREEN}✓ Frontend copied to embed tree${NC}"
echo ""

# Step 2: Build Go Binary
echo -e "${YELLOW}Step 2/3: Building Go binary...${NC}"
cd ../backend-go

# Build with embedded frontend
LDFLAGS="-s -w"
LDFLAGS="${LDFLAGS} -X 'vps-dashboard-api/internal/app.Version=${VERSION}'"
LDFLAGS="${LDFLAGS} -X 'vps-dashboard-api/internal/app.BuildCommit=${COMMIT}'"
LDFLAGS="${LDFLAGS} -X 'vps-dashboard-api/internal/app.BuildTime=${BUILD_TIME}'"

echo -e "${YELLOW}Building binary with ldflags: ${LDFLAGS}${NC}"

go build \
    -ldflags="${LDFLAGS}" \
    -o "../${BINARY_NAME}" \
    cmd/api/main.go

if [ ! -f "../${BINARY_NAME}" ]; then
    echo -e "${RED}❌ Go build failed${NC}"
    exit 1
fi

cd ..

echo -e "${GREEN}✓ Binary built successfully${NC}"
echo ""

# Step 3: Display build info
echo -e "${YELLOW}Step 3/3: Build summary${NC}"

BINARY_SIZE=$(du -h "${BINARY_NAME}" | cut -f1)
echo -e "${GREEN}✓ Build complete!${NC}"
echo ""
echo -e "${BLUE}Binary:${NC}        ${BINARY_NAME}"
echo -e "${BLUE}Size:${NC}          ${BINARY_SIZE}"
echo -e "${BLUE}Platform:${NC}     ${GOOS}/${GOARCH}"
echo ""
echo -e "${GREEN}To run:${NC}        ./${BINARY_NAME}"
echo -e "${GREEN}To install:${NC}    sudo mv ${BINARY_NAME} /usr/local/bin/"
echo ""

# Optional: Test that binary runs
if [ "${RUN_TEST:-}" = "true" ]; then
    echo -e "${YELLOW}Testing binary...${NC}"
    ./"${BINARY_NAME}" --version || true
    echo ""
fi
