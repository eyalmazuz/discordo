// Command kittytest is a standalone diagnostic tool for the Kitty graphics protocol.
// It queries terminal cell dimensions via ioctl, generates a gradient test image,
// sends it using the Kitty protocol, and waits for Enter before cleaning up.
package main

import (
	"bufio"
	"fmt"
	"image"
	"image/color"
	"io"
	"os"

	imgpkg "github.com/ayn2op/discordo/internal/image"
	"golang.org/x/sys/unix"
)

type deps struct {
	isKittySupported func() bool
	ioctlGetWinsize  func(int, uint) (*unix.Winsize, error)
	encodeKitty      func(io.Writer, image.Image, int, int, int, int) error
	deleteAllKitty   func(io.Writer) error
}

func run(stdout, stderr io.Writer, stdin io.Reader, d deps) int {
	if !d.isKittySupported() {
		fmt.Fprintln(stderr, "Terminal does not appear to support Kitty graphics protocol.")
		fmt.Fprintln(stderr, "Set TERM_PROGRAM=kitty (or wezterm/ghostty) to override.")
		return 1
	}

	ws, err := d.ioctlGetWinsize(int(os.Stdout.Fd()), unix.TIOCGWINSZ)
	if err != nil {
		fmt.Fprintf(stderr, "ioctl TIOCGWINSZ: %v\n", err)
		return 1
	}

	cols := int(ws.Col)
	rows := int(ws.Row)
	xpixel := int(ws.Xpixel)
	ypixel := int(ws.Ypixel)

	if cols == 0 || rows == 0 {
		fmt.Fprintln(stderr, "Could not determine terminal size.")
		return 1
	}

	cellW := 0
	cellH := 0
	if xpixel > 0 && ypixel > 0 {
		cellW = xpixel / cols
		cellH = ypixel / rows
	}

	fmt.Fprintf(stdout, "Terminal: %dx%d cells, %dx%d pixels\n", cols, rows, xpixel, ypixel)
	fmt.Fprintf(stdout, "Cell size: %dx%d pixels\n", cellW, cellH)

	if cellW == 0 || cellH == 0 {
		fmt.Fprintln(stderr, "Cell pixel dimensions are zero — terminal may not report pixel size.")
		return 1
	}

	imgCols := min(16, cols-2)
	imgRows := min(8, rows-4)
	imgW := imgCols * cellW
	imgH := imgRows * cellH

	fmt.Fprintf(stdout, "Image: %dx%d cells (%dx%d pixels)\n", imgCols, imgRows, imgW, imgH)

	img := image.NewRGBA(image.Rect(0, 0, imgW, imgH))
	for y := range imgH {
		for x := range imgW {
			r := uint8(255 * x / imgW)
			g := uint8(255 * y / imgH)
			b := uint8(128)
			img.SetRGBA(x, y, color.RGBA{R: r, G: g, B: b, A: 255})
		}
	}

	fmt.Fprintln(stdout, "Sending image via Kitty a=T ...")
	if err := d.encodeKitty(stdout, img, imgCols, imgRows, cellW, cellH); err != nil {
		fmt.Fprintf(stderr, "\nEncodeKitty error: %v\n", err)
		return 1
	}
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "If you see a gradient above, Kitty graphics protocol is working.")
	fmt.Fprint(stdout, "Press Enter to clean up and exit...")

	bufio.NewReader(stdin).ReadLine()

	_ = d.deleteAllKitty(stdout)
	fmt.Fprintln(stdout, "\nDone.")
	return 0
}

func main() {
	os.Exit(run(os.Stdout, os.Stderr, os.Stdin, deps{
		isKittySupported: imgpkg.IsKittySupported,
		ioctlGetWinsize:  unix.IoctlGetWinsize,
		encodeKitty:      imgpkg.EncodeKitty,
		deleteAllKitty:   imgpkg.DeleteAllKitty,
	}))
}
