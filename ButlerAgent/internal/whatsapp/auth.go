package whatsapp

import (
	"fmt"
	"os/exec"
	"runtime"

	"github.com/skip2/go-qrcode"
)

// displayQR generates a QR code PNG and opens it
func displayQR(code string) {
	// Generate QR code PNG
	qrPath := "/tmp/codebutler-qr.png"
	err := qrcode.WriteFile(code, qrcode.Medium, 512, qrPath)
	if err != nil {
		fmt.Println("Error generating QR code:", err)
		fmt.Println("\nQR Code string:", code)
		return
	}

	fmt.Println("\n📱 Opening QR code in image viewer...")
	fmt.Println("   (Scan with WhatsApp > Settings > Linked Devices > Link a Device)")

	// Open PNG with default image viewer
	var openCmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin": // macOS
		openCmd = exec.Command("open", qrPath)
	case "linux":
		openCmd = exec.Command("xdg-open", qrPath)
	case "windows":
		openCmd = exec.Command("cmd", "/c", "start", qrPath)
	}

	if openCmd != nil {
		if err := openCmd.Start(); err != nil {
			fmt.Printf("⚠️  Couldn't open automatically. View manually: %s\n", qrPath)
		} else {
			fmt.Printf("✅ QR code opened: %s\n", qrPath)
		}
	}

	// Show text fallback
	fmt.Println("\n💡 Can't scan? Paste this in WhatsApp > Link a Device:")
	fmt.Printf("   %s\n\n", code)
	fmt.Println("⏳ Waiting for scan...")
}
