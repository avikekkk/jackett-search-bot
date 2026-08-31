#!/usr/bin/env bash
# Run the Go bot in the foreground. Press Ctrl+C to stop.
set -e

cd "$(dirname "$0")"

go build -o bin/jackettbot ./cmd/jackettbot
exec ./bin/jackettbot
