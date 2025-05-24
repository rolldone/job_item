#!/bin/bash

# Detect OS and ARCH
OS=$(uname -s)
ARCH=$(uname -m)

# Set default values
GOOS=""
GOARCH=""

case "$OS" in
    Linux)
        GOOS="linux"
        ;;
    Darwin)
        GOOS="darwin"
        ;;
    MINGW*|MSYS*|CYGWIN*|Windows_NT)
        GOOS="windows"
        ;;
    *)
        echo "Unsupported OS: $OS"
        exit 1
        ;;
esac

case "$ARCH" in
    x86_64|amd64)
        GOARCH="amd64"
        ;;
    aarch64|arm64)
        GOARCH="arm64"
        ;;
    *)
        echo "Unsupported architecture: $ARCH"
        exit 1
        ;;
esac

echo "Building for $GOOS/$GOARCH..."

CGO_ENABLED=0 GOOS=$GOOS GOARCH=$GOARCH GO111MODULE=on go build -o main_$GOOS_$GOARCH main.go