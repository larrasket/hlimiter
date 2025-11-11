#!/bin/bash

set -e

lsof -ti:50051,9000 2>/dev/null | xargs kill -9 2>/dev/null || true
sleep 1

if ! redis-cli ping > /dev/null 2>&1; then
    redis-server --daemonize yes --port 6379
    sleep 1
fi

redis-cli FLUSHALL > /dev/null

go build -o bin/hlimiter-server ./cmd/server
go build -o bin/payment-service ./cmd/examples/payment
go build -o bin/client ./cmd/client

echo ""
CONFIG_PATH=config.yaml ./bin/hlimiter-server &
LIMITER_PID=$!

sleep 2

echo "payment service on port 9000..."
LIMITER_GRPC_ADDR=localhost:50051 PORT=9000 ./bin/payment-service &
PAYMENT_PID=$!

sleep 2

echo "integration test"
PAYMENT_URL=http://localhost:9000 ./bin/client

echo ""
kill $PAYMENT_PID 2>/dev/null || true
kill $LIMITER_PID 2>/dev/null || true

echo ""
echo "Done!"
