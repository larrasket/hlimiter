#!/bin/bash

set -e

echo "Building rate limiter service..."
go build -o bin/hlimiter-server ./cmd/server

echo "Building validation test service..."
go build -o bin/validator ./cmd/validator

echo ""
echo "Starting rate limiter service on port 8080..."
CONFIG_PATH=config.yaml PORT=8080 ./bin/hlimiter-server &
SERVER_PID=$!

sleep 2

echo "Starting validation tests..."
LIMITER_URL=http://localhost:8080 ./bin/validator

echo ""
echo "Stopping rate limiter service..."
kill $SERVER_PID 2>/dev/null || true

echo ""
echo "Done!"
