#!/bin/bash

# Build script for quality gates validator
# Compiles the Go quality gates tool for use in CI/CD

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"

# Colors for output
GREEN='\033[0;32m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

echo -e "${BLUE}Building quality gates validator...${NC}"

cd "$SCRIPT_DIR"

# Build the quality gates tool
if go build -o quality-gates quality-gates.go; then
    echo -e "${GREEN}Quality gates validator built successfully: $SCRIPT_DIR/quality-gates${NC}"
    
    # Make it executable
    chmod +x quality-gates
    
    # Test the build
    if ./quality-gates 2>/dev/null || [ $? -eq 1 ]; then
        echo -e "${GREEN}Quality gates validator is working correctly${NC}"
    else
        echo "Warning: Quality gates validator may have issues"
    fi
else
    echo "Error: Failed to build quality gates validator"
    exit 1
fi