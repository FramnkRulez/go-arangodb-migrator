#!/bin/bash

# Build script for ArangoDB Migrator CLI tool
# This script builds binaries for multiple platforms

set -e

VERSION=${1:-"dev"}
BUILD_DIR="dist"

echo "Building ArangoDB Migrator CLI v$VERSION"

# Create build directory
mkdir -p $BUILD_DIR

# Build for multiple platforms
echo "Building for Linux AMD64..."
GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o $BUILD_DIR/arangodb-migrator-linux-amd64 ./cmd/migrator

echo "Building for Linux ARM64..."
GOOS=linux GOARCH=arm64 go build -ldflags="-s -w" -o $BUILD_DIR/arangodb-migrator-linux-arm64 ./cmd/migrator

echo "Building for macOS AMD64..."
GOOS=darwin GOARCH=amd64 go build -ldflags="-s -w" -o $BUILD_DIR/arangodb-migrator-darwin-amd64 ./cmd/migrator

echo "Building for macOS ARM64..."
GOOS=darwin GOARCH=arm64 go build -ldflags="-s -w" -o $BUILD_DIR/arangodb-migrator-darwin-arm64 ./cmd/migrator

echo "Building for Windows AMD64..."
GOOS=windows GOARCH=amd64 go build -ldflags="-s -w" -o $BUILD_DIR/arangodb-migrator-windows-amd64.exe ./cmd/migrator

# Create checksums
echo "Creating checksums..."
cd $BUILD_DIR
sha256sum arangodb-migrator-* > checksums.txt

echo "Build complete! Binaries created in $BUILD_DIR/:"
ls -la

echo ""
echo "Checksums:"
cat checksums.txt 