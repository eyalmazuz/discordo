// Command kittytest is a standalone diagnostic tool for the Kitty graphics protocol.
// It queries terminal cell dimensions via ioctl, generates a gradient test image,
// sends it using the Kitty protocol, and waits for Enter before cleaning up.
package main

import (
	"bufio"
	"fmt"
	"image"
	"image/color"
	"os"

	imgpkg "github.com/ayn2op/discordo/internal/image"
	"golang.org/x/sys/unix"
)

func main() {
	if !imgpkg.IsKittySupported() {
		fmt.Fprintln(os.Stderr, "Terminal does not appear to support Kitty graphics protocol.")
		fmt.Fprintln(os.Stderr, "Set TERM_PROGRAM=kitty (or wezterm/ghostty) to override.")
		os.Exit(1)
	}

	// Query cell dimensions via ioctl.
	ws, err := unix.IoctlGetWinsize(int(os.Stdout.Fd()), unix.TIOCGWINSZ)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ioctl TIOCGWINSZ: %v\n", err)
		os.Exit(1)
	}

	cols := int(ws.Col)
	rows := int(ws.Row)
	xpixel := int(ws.Xpixel)
	ypixel := int(ws.Ypixel)

	if cols == 0 || rows == 0 {
		fmt.Fprintln(os.Stderr, "Could not determine terminal size.")
		os.Exit(1)
	}

	cellW := 0
	cellH := 0
	if xpixel > 0 && ypixel > 0 {
		cellW = xpixel / cols
		cellH = ypixel / rows
	}

	fmt.Printf("Terminal: %dx%d cells, %dx%d pixels\n", cols, rows, xpixel, ypixel)
	fmt.Printf("Cell size: %dx%d pixels\n", cellW, cellH)

	if cellW == 0 || cellH == 0 {
		fmt.Fprintln(os.Stderr, "Cell pixel dimensions are zero — terminal may not report pixel size.")
		os.Exit(1)
	}

	// Generate a gradient test image (16x8 cells).
	imgCols := min(16, cols-2)
	imgRows := min(8, rows-4)
	imgW := imgCols * cellW
	imgH := imgRows * cellH

	fmt.Printf("Image: %dx%d cells (%dx%d pixels)\n", imgCols, imgRows, imgW, imgH)

	img := image.NewRGBA(image.Rect(0, 0, imgW, imgH))
	for y := range imgH {
		for x := range imgW {
			r := uint8(255 * x / imgW)
			g := uint8(255 * y / imgH)
			b := uint8(128)
			img.SetRGBA(x, y, color.RGBA{R: r, G: g, B: b, A: 255})
		}
	}

	// Encode and send via Kitty protocol.
	fmt.Println("Sending image via Kitty a=T ...")
	if err := imgpkg.EncodeKitty(os.Stdout, img, imgCols, imgRows, cellW, cellH); err != nil {
		fmt.Fprintf(os.Stderr, "\nEncodeKitty error: %v\n", err)
		os.Exit(1)
	}
	fmt.Println()
	fmt.Println("If you see a gradient above, Kitty graphics protocol is working.")
	fmt.Print("Press Enter to clean up and exit...")

	bufio.NewReader(os.Stdin).ReadLine()

	// Clean up all Kitty images.
	_ = imgpkg.DeleteAllKitty(os.Stdout)
	fmt.Println("\nDone.")
}
