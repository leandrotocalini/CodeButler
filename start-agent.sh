#!/bin/bash

# CodeButler Agent Starter
# This script checks setup and starts the WhatsApp agent

set -e

echo "🤖 CodeButler Agent Starter"
echo ""

# Check if config exists
if [ ! -f "config.json" ]; then
    echo "❌ No config.json found"
    echo ""
    echo "Please run setup first:"
    echo "  cd ButlerAgent"
    echo "  go build -o ../codebutler cmd/codebutler/main.go"
    echo "  cd .."
    echo "  ./codebutler"
    echo ""
    exit 1
fi

echo "✅ Configuration found"
echo ""

# Check if agent binary exists
if [ ! -f "codebutler-agent" ]; then
    echo "📦 Building agent binary..."
    cd ButlerAgent
    go build -o ../codebutler-agent cmd/agent/main.go
    cd ..
    echo "✅ Agent built"
    echo ""
fi

# Check if agent is already running
if pgrep -f codebutler-agent > /dev/null; then
    echo "⚠️  Agent is already running"
    echo ""
    echo "To stop it:"
    echo "  pkill codebutler-agent"
    echo ""
    exit 0
fi

# Clean up old temp files
rm -f .codebutler-response .codebutler-question .codebutler-answer

# Start agent
echo "🚀 Starting CodeButler agent..."
echo ""

./codebutler-agent
