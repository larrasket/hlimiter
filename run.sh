#!/bin/bash

set -e

echo "Building services..."
go build -o bin/hlimiter-server ./cmd/server
go build -o bin/payment-service ./cmd/examples/payment
go build -o bin/client ./cmd/client

echo ""
echo "Starting rate limiter service on port 8080..."
CONFIG_PATH=config.yaml PORT=8080 ./bin/hlimiter-server &
LIMITER_PID=$!

sleep 1

echo "Starting payment service on port 9000..."
LIMITER_URL=http://localhost:8080 PORT=9000 ./bin/payment-service &
PAYMENT_PID=$!

sleep 2

echo "Running integration tests..."
PAYMENT_URL=http://localhost:9000 ./bin/client

echo ""
echo "Stopping services..."
kill $PAYMENT_PID 2>/dev/null || true
kill $LIMITER_PID 2>/dev/null || true

echo ""
echo "Done!"
