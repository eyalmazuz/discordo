package image

import (
	"image"

	"github.com/eyalmazuz/tview"
	"github.com/gdamore/tcell/v3"
	"golang.org/x/image/draw"
)

// RenderHalfBlock renders img as half-block characters (▄) fitting within
// maxWidth×maxHeight cells. Each cell encodes 2 vertical pixels: top pixel
// as background color, bottom pixel as foreground color of '▄'.
func RenderHalfBlock(img image.Image, maxWidth, maxHeight int) []tview.Line {
	if maxWidth <= 0 || maxHeight <= 0 {
		return nil
	}

	srcBounds := img.Bounds()
	srcW := srcBounds.Dx()
	srcH := srcBounds.Dy()
	if srcW == 0 || srcH == 0 {
		return nil
	}

	// Target pixel dimensions: maxWidth wide, maxHeight*2 tall (2 pixels per cell row).
	pixelW := maxWidth
	pixelH := maxHeight * 2

	// Preserve aspect ratio.
	scale := min(float64(pixelW)/float64(srcW), float64(pixelH)/float64(srcH))
	newW := max(int(float64(srcW)*scale), 1)
	newH := max(int(float64(srcH)*scale), 1)

	dst := image.NewRGBA(image.Rect(0, 0, newW, newH))
	draw.ApproxBiLinear.Scale(dst, dst.Bounds(), img, srcBounds, draw.Over, nil)

	// Number of cell rows: ceiling of newH/2
	cellRows := (newH + 1) / 2
	lines := make([]tview.Line, 0, cellRows)

	for cy := 0; cy < cellRows; cy++ {
		line := make(tview.Line, 0, newW)
		py0 := cy * 2 // top pixel row
		py1 := py0 + 1 // bottom pixel row

		for cx := 0; cx < newW; cx++ {
			topR, topG, topB, _ := dst.At(cx, py0).RGBA()
			topColor := tcell.NewRGBColor(int32(topR>>8), int32(topG>>8), int32(topB>>8))

			var botColor tcell.Color
			if py1 < newH {
				botR, botG, botB, _ := dst.At(cx, py1).RGBA()
				botColor = tcell.NewRGBColor(int32(botR>>8), int32(botG>>8), int32(botB>>8))
			} else {
				botColor = tcell.ColorDefault
			}

			style := tcell.StyleDefault.Background(topColor).Foreground(botColor)

			// Try to merge with previous segment if same style.
			if n := len(line); n > 0 && line[n-1].Style == style {
				line[n-1].Text += "▄"
			} else {
				line = append(line, tview.Segment{Text: "▄", Style: style})
			}
		}

		lines = append(lines, line)
	}

	return lines
}
