#!/bin/bash
# CodeButler - Build and run

set -e

echo "🤖 CodeButler"
echo ""

echo "📦 Building..."
cd ButlerAgent
go build -o ../codebutler ./cmd/codebutler/
cd ..

echo "✅ Built: ./codebutler"
echo ""
echo "🚀 Starting..."
echo ""

./codebutler
