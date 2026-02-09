#!/bin/bash
# Script de diagnóstico para WhatsApp connection

echo "=== WhatsApp Connection Diagnostics ==="
echo

echo "1. Checking whatsmeow version..."
grep whatsmeow go.mod
echo

echo "2. Checking if there's a newer stable version..."
echo "Run: go get -u go.mau.fi/whatsmeow@latest"
echo "Then: go mod tidy"
echo

echo "3. Alternatives to try:"
echo "   a) Update to latest whatsmeow:"
echo "      go get -u go.mau.fi/whatsmeow@latest"
echo "      go mod tidy"
echo "      go build -o test-integration ./cmd/test-integration/"
echo
echo "   b) Try a specific stable version (if latest fails):"
echo "      go get go.mau.fi/whatsmeow@v0.0.0-20240226145141-0b735c107394"
echo "      go mod tidy"
echo "      go build -o test-integration ./cmd/test-integration/"
echo

echo "4. Check your network:"
echo "   - Are you behind a corporate proxy?"
echo "   - Are you using a VPN?"
echo "   - Try: curl -I https://web.whatsapp.com/"
echo

echo "5. If still failing, we can:"
echo "   - Add proxy configuration to the client"
echo "   - Add custom user agent"
echo "   - Try different connection parameters"
echo

echo "=== What to run now ==="
echo "Try updating whatsmeow first:"
echo
echo "  go get -u go.mau.fi/whatsmeow@latest"
echo "  go mod tidy"
echo "  go build -o test-integration ./cmd/test-integration/"
echo "  ./test-integration"
echo
