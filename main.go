package main

import (
	"fmt"
	"os"
)

const version = "0.1.0"

func main() {
	fmt.Println("╔════════════════════════════════════════╗")
	fmt.Println("║         CodeButler v" + version + "            ║")
	fmt.Println("║  Your code's personal butler           ║")
	fmt.Println("╚════════════════════════════════════════╝")
	fmt.Println()

	if fileExists("config.json") {
		fmt.Println("✅ Configuration found")
		fmt.Println()
		fmt.Println("Starting CodeButler...")
		fmt.Println("(Not implemented yet - see IMPLEMENTATION_ROADMAP.md)")
	} else {
		fmt.Println("👋 Welcome! Let's set up CodeButler for the first time.")
		fmt.Println()
		fmt.Println("This wizard will guide you through:")
		fmt.Println("  1. Connecting to WhatsApp")
		fmt.Println("  2. Setting up API keys")
		fmt.Println("  3. Creating your CodeButler Developer group")
		fmt.Println()
		fmt.Println("(Setup not implemented yet - see IMPLEMENTATION_ROADMAP.md)")
		fmt.Println()
		fmt.Println("To continue development, check the roadmap:")
		fmt.Println("  👉 IMPLEMENTATION_ROADMAP.md")
	}
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
