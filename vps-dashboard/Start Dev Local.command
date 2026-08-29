#!/bin/bash
# ============================================
# Double-click untuk start development server
# Backend (port 3001) + Frontend (port 5173)
# ============================================

clear

PROJECT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$PROJECT_DIR"

# Colors
GREEN='\033[0;32m'
CYAN='\033[0;36m'
BOLD='\033[1m'
NC='\033[0m'

echo -e "${BOLD}"
echo "  ╔══════════════════════════════════════╗"
echo "  ║    VPS DASHBOARD - DEV SERVER        ║"
echo "  ╚══════════════════════════════════════╝"
echo -e "${NC}"

# Start backend in background
echo -e "${CYAN}Starting backend on port 3001...${NC}"
cd "$PROJECT_DIR/backend"
node server.js &
BACKEND_PID=$!
echo -e "${GREEN}✓ Backend started (PID: $BACKEND_PID)${NC}"

# Wait a moment
sleep 1

# Start frontend
echo -e "${CYAN}Starting frontend on port 5173...${NC}"
cd "$PROJECT_DIR/frontend"
echo ""
echo -e "${GREEN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo -e "${GREEN}  Backend:  http://localhost:3001${NC}"
echo -e "${GREEN}  Frontend: http://localhost:5173${NC}"
echo -e "${GREEN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo ""

# Open browser
open "http://localhost:5173" 2>/dev/null || true

# Run frontend (this blocks)
npm run dev

# When frontend stops, also stop backend
kill $BACKEND_PID 2>/dev/null
echo ""
echo "Dev servers stopped."
echo "Press any key to close..."
read -n 1
