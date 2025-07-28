#!/bin/bash

# Script to generate and serve Go documentation locally
# This is useful for reviewing documentation during development

set -e

echo "Generating Go documentation..."

# Check if godoc is installed
if ! command -v godoc &> /dev/null; then
    echo "Installing godoc..."
    go install golang.org/x/tools/cmd/godoc@latest
fi

# Get the port from command line or use default
PORT=${1:-6060}

echo "Starting documentation server on http://localhost:$PORT"
echo "Press Ctrl+C to stop"
echo ""

# Start the documentation server
godoc -http=:$PORT 