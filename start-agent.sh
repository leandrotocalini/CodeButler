#!/bin/bash
# CodeButler Agent Starter
# Starts the WhatsApp agent in foreground

set -e

echo "🤖 Starting CodeButler Agent"
echo ""

# Check if config exists
if [ ! -f "config.json" ]; then
    echo "❌ No config.json found"
    echo ""
    echo "Please run setup first:"
    echo "  ./setup.sh"
    echo ""
    exit 1
fi

# Check if agent binary exists, rebuild if needed
if [ ! -f "codebutler-agent" ]; then
    echo "📦 Building agent..."
    cd ButlerAgent
    go build -o ../codebutler-agent ./cmd/agent/
    cd ..
    echo "✅ Agent built"
    echo ""
fi

# Check if already running
if pgrep -f codebutler-agent > /dev/null; then
    echo "⚠️  Agent already running"
    echo "   To stop: pkill codebutler-agent"
    echo ""
    exit 0
fi

# Initialize protocol directory
mkdir -p /tmp/codebutler

# Start agent
./codebutler-agent
