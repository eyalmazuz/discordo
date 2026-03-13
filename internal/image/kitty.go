package image

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/png"
	"io"
	"os"
	"strings"

	"golang.org/x/image/draw"
)

const kittyChunkSize = 4096

// IsKittySupported checks environment variables to determine if the terminal
// supports the Kitty graphics protocol.
func IsKittySupported() bool {
	term := os.Getenv("TERM")
	termProgram := os.Getenv("TERM_PROGRAM")

	if strings.Contains(term, "kitty") {
		return true
	}

	switch strings.ToLower(termProgram) {
	case "kitty", "wezterm", "ghostty":
		return true
	}

	return false
}

// EncodeKittyPayload does the expensive work: resize + PNG encode + base64.
func EncodeKittyPayload(img image.Image, cols, rows, cellW, cellH int) (string, error) {
	targetW := cols * cellW
	targetH := rows * cellH
	if targetW <= 0 || targetH <= 0 {
		return "", fmt.Errorf("kitty: invalid target size %dx%d", targetW, targetH)
	}

	resized := resizeImage(img, targetW, targetH)

	var buf bytes.Buffer
	if err := png.Encode(&buf, resized); err != nil {
		return "", fmt.Errorf("kitty: png encode: %w", err)
	}

	return base64.StdEncoding.EncodeToString(buf.Bytes()), nil
}

// EncodeKitty resizes img to fit cols×rows cells (each cellW×cellH pixels)
// and writes the Kitty graphics protocol escape sequences to w.
func EncodeKitty(w io.Writer, img image.Image, cols, rows, cellW, cellH int) error {
	encoded, err := EncodeKittyPayload(img, cols, rows, cellW, cellH)
	if err != nil {
		return err
	}
	return WriteKittyChunks(w, encoded, cols, rows, 0)
}

// WriteKittyChunks writes pre-encoded data as Kitty protocol escape sequences.
// When id > 0, the image is assigned a Kitty protocol image ID so it can be
// individually managed (replaced in-place or deleted by ID).
func WriteKittyChunks(w io.Writer, data string, cols, rows int, id uint32) error {
	for i := 0; i < len(data); i += kittyChunkSize {
		end := i + kittyChunkSize
		if end > len(data) {
			end = len(data)
		}
		chunk := data[i:end]
		more := 1
		if end >= len(data) {
			more = 0
		}

		if i == 0 {
			// First chunk: include format, action, columns, rows
			if id > 0 {
				// Use a=t (transmit only, do not display immediately) when an ID is provided.
				// This prevents the full uncropped image from flashing and overflowing the terminal.
				_, err := fmt.Fprintf(w, "\x1b_Gq=2,f=100,a=t,i=%d,c=%d,r=%d,m=%d;%s\x1b\\", id, cols, rows, more, chunk)
				if err != nil {
					return err
				}
			} else {
				// Use a=T (transmit and display) for immediate anonymous placement
				_, err := fmt.Fprintf(w, "\x1b_Gq=2,f=100,a=T,c=%d,r=%d,m=%d;%s\x1b\\", cols, rows, more, chunk)
				if err != nil {
					return err
				}
			}
		} else {
			_, err := fmt.Fprintf(w, "\x1b_Gm=%d;%s\x1b\\", more, chunk)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// DeleteKittyByID deletes a specific Kitty image and its placements from memory.
func DeleteKittyByID(w io.Writer, id uint32) error {
	_, err := fmt.Fprintf(w, "\x1b_Gq=2,a=d,d=I,i=%d\x1b\\", id)
	return err
}

// DeleteKittyPlacement deletes all placements of a Kitty image but keeps it in memory.
func DeleteKittyPlacement(w io.Writer, id uint32) error {
	_, err := fmt.Fprintf(w, "\x1b_Gq=2,a=d,d=i,i=%d\x1b\\", id)
	return err
}

// DeleteAllKitty writes the Kitty graphics protocol escape to clear all
// images and placements from memory.
func DeleteAllKitty(w io.Writer) error {
	_, err := fmt.Fprint(w, "\x1b_Gq=2,a=d,d=A\x1b\\")
	return err
}

// PlaceKitty places a previously uploaded Kitty image.
func PlaceKitty(w io.Writer, id uint32, cols, rows int) error {
	_, err := fmt.Fprintf(w, "\x1b_Gq=2,a=p,i=%d,c=%d,r=%d\x1b\\", id, cols, rows)
	return err
}

// PlaceKittyCropped places a previously uploaded Kitty image, cropping it to a specific source rectangle.
// srcX, srcY are source pixel offsets. cols, rows define the visual dimension to display in cells.
func PlaceKittyCropped(w io.Writer, id uint32, cols, rows, srcX, srcY, srcW, srcH int) error {
	_, err := fmt.Fprintf(w, "\x1b_Gq=2,a=p,i=%d,c=%d,r=%d,x=%d,y=%d,w=%d,h=%d\x1b\\", id, cols, rows, srcX, srcY, srcW, srcH)
	return err
}

// resizeImage scales img to fit within targetW×targetH, preserving aspect ratio.
func resizeImage(img image.Image, targetW, targetH int) image.Image {
	srcBounds := img.Bounds()
	srcW := srcBounds.Dx()
	srcH := srcBounds.Dy()
	if srcW == 0 || srcH == 0 {
		return img
	}

	scale := min(float64(targetW)/float64(srcW), float64(targetH)/float64(srcH))
	newW := max(int(float64(srcW)*scale), 1)
	newH := max(int(float64(srcH)*scale), 1)

	dst := image.NewRGBA(image.Rect(0, 0, newW, newH))
	draw.ApproxBiLinear.Scale(dst, dst.Bounds(), img, srcBounds, draw.Over, nil)
	return dst
}
