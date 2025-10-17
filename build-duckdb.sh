#!/bin/bash
#
# Build chicha-isotope-map with DuckDB support
#
# DuckDB provides excellent performance and requires CGO.
# This script handles the build with proper flags.
#

set -e

GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

echo -e "${GREEN}Building chicha-isotope-map with DuckDB support...${NC}"
echo ""

# Check for required tools
if ! command -v go &> /dev/null; then
    echo -e "${RED}Error: Go is not installed${NC}"
    exit 1
fi

if ! command -v gcc &> /dev/null && ! command -v clang &> /dev/null; then
    echo -e "${RED}Warning: No C compiler found (gcc/clang)${NC}"
    echo "DuckDB requires CGO and a C compiler"
    echo ""
fi

# Enable CGO
export CGO_ENABLED=1

echo "Building with CGO_ENABLED=1 and -tags duckdb..."
echo ""

# Build
if go build -tags duckdb -o chicha-isotope-map; then
    echo ""
    echo -e "${GREEN}✓ Build successful!${NC}"
    echo ""
    echo "Binary: ./chicha-isotope-map"
    echo ""
    echo "To run with realtime sensors:"
    echo "  ./chicha-isotope-map -db-type=duckdb -db-path=./data/db.duckdb -safecast-realtime=true"
    echo ""
    echo "Or use the convenience script:"
    echo "  ./run-with-realtime.sh"
    echo ""
else
    echo ""
    echo -e "${RED}✗ Build failed${NC}"
    echo ""
    echo "Common issues:"
    echo "  - Missing C compiler (install gcc or clang)"
    echo "  - Missing build dependencies"
    echo ""
    echo "On Ubuntu/Debian: sudo apt-get install build-essential"
    echo "On macOS: xcode-select --install"
    echo ""
    exit 1
fi
