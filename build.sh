#!/bin/bash
set -e

cd "$(dirname "$0")"

echo "📦 Running go mod tidy..."
go mod tidy

echo "🔨 Building..."
go build -ldflags="-s -w" -o goqrly .

echo "🧪 Running tests..."
go test ./...

echo "✅ Build and tests passed!"
