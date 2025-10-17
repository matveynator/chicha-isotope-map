#!/bin/bash
#
# Build and run chicha-isotope-map with DuckDB and Safecast realtime sensors enabled
#
# This script:
# 1. Builds the application with DuckDB support (requires CGO)
# 2. Runs it with realtime sensors enabled
# 3. Uses DuckDB as the database backend
#

set -e

# Colors for output
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m' # No Color

echo -e "${GREEN}═══════════════════════════════════════════════════════${NC}"
echo -e "${GREEN}  Chicha Isotope Map - Realtime Build & Run Script${NC}"
echo -e "${GREEN}═══════════════════════════════════════════════════════${NC}"
echo ""

# Configuration
DB_PATH="${DB_PATH:-./data/chicha-realtime.duckdb}"
PORT="${PORT:-8080}"
BINARY_NAME="chicha-isotope-map"

# Create data directory if it doesn't exist
mkdir -p "$(dirname "$DB_PATH")"

echo -e "${YELLOW}[1/3] Building with DuckDB support...${NC}"
echo "      This requires CGO and may take a minute..."

# Enable CGO and build with duckdb tag
export CGO_ENABLED=1

if go build -tags duckdb -o "$BINARY_NAME"; then
    echo -e "${GREEN}✓ Build successful!${NC}"
    echo ""
else
    echo -e "${RED}✗ Build failed!${NC}"
    echo ""
    echo "DuckDB requires CGO. Please ensure you have:"
    echo "  - GCC or a C compiler installed"
    echo "  - CGO_ENABLED=1 (already set by this script)"
    echo ""
    exit 1
fi

echo -e "${YELLOW}[2/3] Configuration:${NC}"
echo "      Database: DuckDB"
echo "      Database path: $DB_PATH"
echo "      Port: $PORT"
echo "      Realtime sensors: ENABLED"
echo "      Polling: https://tt.safecast.org/devices every 5 minutes"
echo ""

echo -e "${YELLOW}[3/3] Starting server...${NC}"
echo ""
echo -e "${GREEN}Server will be available at:${NC}"
echo "      http://localhost:$PORT"
echo ""
echo -e "${GREEN}Debug endpoint:${NC}"
echo "      http://localhost:$PORT/realtime_debug"
echo ""
echo -e "${GREEN}API docs:${NC}"
echo "      http://localhost:$PORT/api/docs"
echo ""
echo -e "${YELLOW}Press Ctrl+C to stop${NC}"
echo "═══════════════════════════════════════════════════════"
echo ""

# Run with all the features enabled
./"$BINARY_NAME" \
    -db-type=duckdb \
    -db-path="$DB_PATH" \
    -port="$PORT" \
    -safecast-realtime=true

# Note: The binary will continue running until interrupted
