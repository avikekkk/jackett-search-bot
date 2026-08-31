#!/usr/bin/env bash
# Run the Go bot in the foreground. Press Ctrl+C to stop.
set -e

cd "$(dirname "$0")"

# Binaries go under bin/ so the build cannot collide with the Python package of
# the same name living beside it.
go build -o bin/jackettbot ./cmd/jackettbot
exec ./bin/jackettbot
