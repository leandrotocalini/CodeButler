#!/bin/bash
# CodeButler Reinstall Skill
# Pulls latest changes, rebuilds agent, and restarts it

set -e

echo "🔄 CodeButler Reinstall"
echo ""

# Stop running agent
echo "🛑 Stopping current agent..."
if pgrep -f codebutler-agent > /dev/null; then
    pkill codebutler-agent
    sleep 2
    echo "   ✅ Agent stopped"
else
    echo "   ℹ️  No agent running"
fi

# Git pull
echo ""
echo "📥 Pulling latest changes from GitHub..."
git fetch origin
git pull origin main
echo "   ✅ Code updated"

# Rebuild setup binary
echo ""
echo "🔨 Rebuilding setup wizard..."
(cd ButlerAgent && go build -o ../codebutler ./cmd/codebutler/)
echo "   ✅ Setup wizard built"

# Rebuild agent
echo ""
echo "🔨 Rebuilding WhatsApp agent..."
(cd ButlerAgent && go build -o ../codebutler-agent ./cmd/agent/)
echo "   ✅ Agent built"

# Clean up temp files
echo ""
echo "🧹 Cleaning up..."
rm -f .codebutler-response .codebutler-question .codebutler-answer
echo "   ✅ Temp files removed"

# Start agent in background
echo ""
echo "🚀 Starting agent in background..."
nohup ./codebutler-agent > codebutler-agent.log 2>&1 &
sleep 2

if pgrep -f codebutler-agent > /dev/null; then
    echo "   ✅ Agent started successfully"
    echo ""
    echo "📋 Agent is running (PID: $(pgrep -f codebutler-agent))"
    echo "📄 Logs: tail -f codebutler-agent.log"
else
    echo "   ❌ Failed to start agent"
    echo "   Check logs: cat codebutler-agent.log"
    exit 1
fi

echo ""
echo "🎉 CodeButler reinstalled and restarted!"
echo ""
