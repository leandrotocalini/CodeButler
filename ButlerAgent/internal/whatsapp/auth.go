package whatsapp

import (
	"fmt"
	"strings"

	"github.com/skip2/go-qrcode"
)

// displayQR prints a compact QR code to the terminal using half-block characters
func displayQR(code string) {
	qr, err := qrcode.New(code, qrcode.Low)
	if err != nil {
		fmt.Println("Error generating QR code:", err)
		fmt.Println("\nQR Code string:", code)
		return
	}

	// Get bitmap (2D array of booleans)
	bitmap := qr.Bitmap()

	// Print using half-block characters (2 rows per line)
	for i := 0; i < len(bitmap); i += 2 {
		line := ""
		for j := 0; j < len(bitmap[i]); j++ {
			top := bitmap[i][j]
			bottom := false
			if i+1 < len(bitmap) {
				bottom = bitmap[i+1][j]
			}

			// Use half-block characters
			if top && bottom {
				line += "█" // Both black
			} else if top && !bottom {
				line += "▀" // Top black
			} else if !top && bottom {
				line += "▄" // Bottom black
			} else {
				line += " " // Both white
			}
		}
		fmt.Println(strings.ReplaceAll(line, " ", "  ")) // Double spaces for square pixels
	}

	// Show code as backup
	fmt.Println("\n💡 Can't scan? Paste this in WhatsApp > Link a Device:")
	fmt.Printf("   %s\n", code)
}
