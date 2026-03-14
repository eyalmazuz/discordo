package image

import (
	"image"
	"image/color"
	"testing"
)

func TestRenderHalfBlock(t *testing.T) {
	t.Run("EmptyImage", func(t *testing.T) {
		if lines := RenderHalfBlock(image.NewRGBA(image.Rect(0, 0, 0, 0)), 10, 10); lines != nil {
			t.Error("expected nil for empty image")
		}
	})

	t.Run("InvalidDimensions", func(t *testing.T) {
		img := image.NewRGBA(image.Rect(0, 0, 1, 1))
		if lines := RenderHalfBlock(img, 0, 10); lines != nil {
			t.Error("expected nil for zero maxWidth")
		}
	})

	t.Run("MergingSegments", func(t *testing.T) {
		// Create a 2x2 image with same colors to trigger merging
		img := image.NewRGBA(image.Rect(0, 0, 2, 2))
		for y := 0; y < 2; y++ {
			for x := 0; x < 2; x++ {
				img.Set(x, y, color.Black)
			}
		}

		lines := RenderHalfBlock(img, 10, 10)
		if len(lines) != 5 {
			t.Fatalf("expected 5 lines, got %d", len(lines))
		}
		if len(lines[0]) != 1 {
			t.Errorf("expected 1 merged segment, got %d", len(lines[0]))
		}
		if len(lines[0][0].Text) != 30 {
			t.Errorf("expected merged text length 30 (10 cells * 3 bytes), got %d", len(lines[0][0].Text))
		}
	})

	t.Run("OddHeight", func(t *testing.T) {
		// 1x1 image -> 1 cell row, py1 >= newH
		img := image.NewRGBA(image.Rect(0, 0, 1, 1))
		img.Set(0, 0, color.White)
		lines := RenderHalfBlock(img, 1, 1)
		if len(lines) != 1 {
			t.Fatalf("expected 1 line, got %d", len(lines))
		}
	})
}
