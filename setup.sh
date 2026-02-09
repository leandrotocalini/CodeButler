#!/bin/bash
# CodeButler Automatic Setup
# Runs the complete setup process

set -e

echo "🤖 CodeButler Automatic Setup"
echo ""

# Check if already configured
if [ -f "config.json" ]; then
    echo "⚠️  config.json already exists"
    echo ""
    echo "Do you want to:"
    echo "1. Keep existing config and start agent"
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
go build -o ../codebutler cmd/codebutler/main.go
echo "   ✅ Wizard built"
echo ""

echo "🔨 Building WhatsApp agent..."
go build -o ../codebutler-agent cmd/agent/main.go
cd ..
echo "   ✅ Agent built"
echo ""

echo "🎉 Binaries ready!"
echo ""
echo "📋 Now running interactive setup wizard..."
echo "   You'll be asked about:"
echo "   - Voice transcription (OpenAI API key)"
echo "   - WhatsApp group name"
echo "   - Bot prefix"
echo "   - Sources directory"
echo ""
echo "   Then you'll scan a QR code with WhatsApp"
echo ""

# Run the wizard
./codebutler

echo ""
echo "✅ Setup complete!"
echo ""
echo "To start the agent, run:"
echo "  ./start-agent.sh"
echo ""
