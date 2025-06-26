#!/usr/bin/env bash

# Install dependencies
npm ci

# Run VitePress dev server
npx vitepress dev docs --host 0.0.0.0 &

# Store the PID of the VitePress process
VITEPRESS_PID=$!

# Function to handle SIGINT (Ctrl+C)
handle_sigint() {
    echo "Stopping VitePress dev server..."
    kill -TERM "$VITEPRESS_PID"
    wait "$VITEPRESS_PID"
    exit 0
}

# Trap SIGINT and call the handle_sigint function
trap handle_sigint SIGINT

# Wait for the VitePress process to finish
wait "$VITEPRESS_PID"
