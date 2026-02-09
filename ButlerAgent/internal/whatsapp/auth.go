package whatsapp

import (
	"fmt"

	"github.com/skip2/go-qrcode"
)

// displayQR prints a QR code to the terminal
func displayQR(code string) {
	qr, err := qrcode.New(code, qrcode.Medium)
	if err != nil {
		fmt.Println("Error generating QR code:", err)
		fmt.Println("\nQR Code string:", code)
		return
	}

	// Print QR as ASCII art to terminal
	fmt.Println(qr.ToSmallString(false))
}
