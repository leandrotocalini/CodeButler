#!/bin/bash
# CodeButler Initial Installation
# Runs first-time setup

set -e

echo "🤖 CodeButler Installation"
echo ""

# Check if already installed
if [ -f "config.json" ]; then
    echo "⚠️  CodeButler is already installed (config.json exists)"
    echo ""
    read -p "Do you want to reconfigure? (yes/no): " answer

    if [ "$answer" != "yes" ] && [ "$answer" != "y" ]; then
        echo "Installation cancelled."
        exit 0
    fi

    echo ""
    echo "🗑️  Removing old configuration..."
    rm -f config.json
    rm -rf whatsapp-session/
    echo "   ✅ Old config removed"
    echo ""
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
echo "📋 Starting interactive setup wizard..."
echo ""
echo "   You'll be asked about:"
echo "   - Voice transcription (OpenAI API key)"
echo "   - WhatsApp group name"
echo "   - Bot prefix"
echo "   - Sources directory"
echo ""
echo "   Then you'll scan a QR code with WhatsApp"
echo ""
echo "Press Enter to continue..."
read

# Run the wizard
./codebutler

echo ""
echo "✅ Installation complete!"
echo ""
echo "📱 To start the agent, run:"
echo "   ./start-agent.sh"
echo ""
echo "📝 Or if using Claude Code, ask me to start it for you"
echo ""
