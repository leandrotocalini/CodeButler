#!/bin/bash
# CodeButler Setup Script
# Builds wizard + agent, then runs web-based setup

set -e

echo "🤖 CodeButler Setup"
echo ""

# Check if already configured
if [ -f "config.json" ]; then
    echo "⚠️  config.json already exists"
    echo ""
    echo "Do you want to:"
    echo "1. Keep existing config"
    echo "2. Delete and reconfigure"
    echo ""
    read -p "Choice (1/2): " choice

    if [ "$choice" = "2" ]; then
        echo "🗑️  Removing old config..."
        rm -f config.json
        rm -rf whatsapp-session/
    else
        echo "✅ Keeping existing config"
        exit 0
    fi
fi

echo "📦 Installing Go dependencies..."
cd ButlerAgent
go mod download
echo "   ✅ Dependencies installed"
echo ""

echo "🔨 Building setup wizard..."
go build -o ../codebutler-wizard ./cmd/setup-wizard/
echo "   ✅ Wizard built"
echo ""

echo "🔨 Building agent..."
go build -o ../codebutler-agent ./cmd/agent/
cd ..
echo "   ✅ Agent built"
echo ""

echo "🌐 Starting setup wizard..."
echo "   Opening browser at http://localhost:3000"
echo ""

# Run wizard (blocks until setup complete)
./codebutler-wizard

echo ""
echo "✅ Setup complete!"
echo ""

# Check if setup was successful
if [ ! -f "/tmp/codebutler/setup-status.json" ]; then
    echo "⚠️  Setup status not found"
    exit 1
fi

echo "📋 Configuration saved to config.json"
echo ""
