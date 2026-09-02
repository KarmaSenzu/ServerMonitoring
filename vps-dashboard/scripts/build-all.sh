#!/bin/bash
# Build VPS Dashboard for multiple platforms
# Creates release binaries for Linux, macOS, and Windows

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

VERSION="${VERSION:-$(git describe --tags --always 2>/dev/null || echo 'dev')}"
COMMIT="${COMMIT:-$(git rev-parse --short HEAD 2>/dev/null || echo 'unknown')}"
BUILD_TIME="$(date -u '+%Y-%m-%d_%H:%M:%S')"

echo -e "${BLUE}=== VPS Dashboard Multi-Platform Build ===${NC}"
echo -e "${BLUE}Version:${NC}     ${VERSION}"
echo -e "${BLUE}Commit:${NC}      ${COMMIT}"
echo ""

# Navigate to project root
cd "$(dirname "$0")/.."

# Create dist directory
DIST_DIR="dist"
rm -rf "${DIST_DIR}"
mkdir -p "${DIST_DIR}"

# Build frontend once (shared by all platforms)
echo -e "${YELLOW}Building frontend...${NC}"
cd frontend
if [ ! -d "node_modules" ]; then
    npm ci
fi
npm run build

if [ ! -f "dist/index.html" ]; then
    echo -e "${RED}❌ Frontend build failed${NC}"
    exit 1
fi
echo -e "${GREEN}✓ Frontend built${NC}"
echo ""

cd ..

# Copy frontend dist to backend embed tree (required for go:embed)
echo -e "${YELLOW}Copying dist to backend-go/internal/httpx/dist...${NC}"
mkdir -p backend-go/internal/httpx/dist
cp -r frontend/dist/* backend-go/internal/httpx/dist/

if [ ! -f "backend-go/internal/httpx/dist/index.html" ]; then
    echo -e "${RED}❌ Failed to copy dist${NC}"
    exit 1
fi
echo -e "${GREEN}✓ Frontend copied to embed tree${NC}"
echo ""

# Platform definitions: "GOOS/GOARCH/binary-suffix"
PLATFORMS=(
    "linux/amd64/"
    "linux/arm64/"
    "darwin/amd64/"
    "darwin/arm64/"
    "windows/amd64/.exe"
    "windows/arm64/.exe"
)

# Build for each platform
for platform in "${PLATFORMS[@]}"; do
    IFS='/' read -r GOOS GOARCH SUFFIX <<< "$platform"
    
    BINARY_NAME="vpsdash${SUFFIX}"
    OUTPUT_PATH="${DIST_DIR}/vpsdash-${GOOS}-${GOARCH}${SUFFIX}"
    
    echo -e "${YELLOW}Building for ${GOOS}/${GOARCH}...${NC}"
    
    cd backend-go
    
    LDFLAGS="-s -w"
    LDFLAGS="${LDFLAGS} -X 'vps-dashboard-api/internal/app.Version=${VERSION}'"
    LDFLAGS="${LDFLAGS} -X 'vps-dashboard-api/internal/app.BuildCommit=${COMMIT}'"
    LDFLAGS="${LDFLAGS} -X 'vps-dashboard-api/internal/app.BuildTime=${BUILD_TIME}'"
    
    GOOS="${GOOS}" GOARCH="${GOARCH}" go build \
        -ldflags="${LDFLAGS}" \
        -o "../${OUTPUT_PATH}" \
        cmd/api/main.go
    
    cd ..
    
    if [ ! -f "${OUTPUT_PATH}" ]; then
        echo -e "${RED}❌ Build failed for ${GOOS}/${GOARCH}${NC}"
        continue
    fi
    
    SIZE=$(du -h "${OUTPUT_PATH}" | cut -f1)
    echo -e "${GREEN}✓ Built ${GOOS}/${GOARCH} (${SIZE})${NC}"
    
    # Create tar.gz archive (skip for Windows)
    if [ "${GOOS}" != "windows" ]; then
        ARCHIVE="${DIST_DIR}/vpsdash-${VERSION}-${GOOS}-${GOARCH}.tar.gz"
        tar -czf "${ARCHIVE}" -C "${DIST_DIR}" "vpsdash-${GOOS}-${GOARCH}${SUFFIX}"
        echo -e "${BLUE}   Archive: $(basename ${ARCHIVE})${NC}"
    else
        # Create zip for Windows
        ARCHIVE="${DIST_DIR}/vpsdash-${VERSION}-${GOOS}-${GOARCH}.zip"
        (cd "${DIST_DIR}" && zip -q "${ARCHIVE##*/}" "vpsdash-${GOOS}-${GOARCH}${SUFFIX}")
        echo -e "${BLUE}   Archive: $(basename ${ARCHIVE})${NC}"
    fi
    
    echo ""
done

# Generate checksums
echo -e "${YELLOW}Generating checksums...${NC}"
cd "${DIST_DIR}"
shasum -a 256 vpsdash-${VERSION}-* > SHA256SUMS
cd ..
echo -e "${GREEN}✓ Checksums generated${NC}"
echo ""

# Summary
echo -e "${GREEN}=== Build Complete ===${NC}"
echo -e "${BLUE}Output directory:${NC} ${DIST_DIR}/"
echo ""
echo -e "${BLUE}Built binaries:${NC}"
ls -lh "${DIST_DIR}"/vpsdash-* | awk '{print "  " $9 " (" $5 ")"}'
echo ""
echo -e "${BLUE}Archives:${NC}"
ls -lh "${DIST_DIR}"/*.{tar.gz,zip} 2>/dev/null | awk '{print "  " $9 " (" $5 ")"}'
echo ""
echo -e "${GREEN}Ready for distribution!${NC}"
