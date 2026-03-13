package image

import (
	"bytes"
	"image"
	"image/color"
	"image/gif"
	"testing"
	"time"
)

func TestDecodeImageData_AnimatedGIF(t *testing.T) {
	data := buildAnimatedGIF(t)

	img, anim, err := decodeImageData(data, "https://cdn.discordapp.com/emojis/123.gif")
	if err != nil {
		t.Fatalf("decodeImageData returned error: %v", err)
	}
	if anim == nil {
		t.Fatal("expected animated GIF to produce animation data")
	}
	if img == nil {
		t.Fatal("expected first frame image")
	}
	if len(anim.frames) != 2 {
		t.Fatalf("expected 2 frames, got %d", len(anim.frames))
	}
	if anim.totalDuration <= 0 {
		t.Fatalf("expected positive total duration, got %v", anim.totalDuration)
	}
}

func TestAnimation_FrameAt(t *testing.T) {
	data := buildAnimatedGIF(t)
	_, anim, err := decodeImageData(data, "https://cdn.discordapp.com/emojis/123.gif")
	if err != nil {
		t.Fatalf("decodeImageData returned error: %v", err)
	}
	if anim == nil {
		t.Fatal("expected animation")
	}

	anim.startedAt = time.Unix(0, 0)
	frame0, index0, next0 := anim.FrameAt(time.Unix(0, 0))
	frame1, index1, next1 := anim.FrameAt(time.Unix(0, int64(150*time.Millisecond)))

	if frame0 == nil || frame1 == nil {
		t.Fatal("expected non-nil frames")
	}
	if index0 != 0 {
		t.Fatalf("expected first frame index 0, got %d", index0)
	}
	if index1 != 1 {
		t.Fatalf("expected second frame index 1, got %d", index1)
	}
	if next0 <= 0 || next1 <= 0 {
		t.Fatalf("expected positive next-frame delays, got %v and %v", next0, next1)
	}
}

func buildAnimatedGIF(t *testing.T) []byte {
	t.Helper()

	palette := color.Palette{
		color.Transparent,
		color.RGBA{R: 255, A: 255},
		color.RGBA{G: 255, A: 255},
	}

	frame0 := image.NewPaletted(image.Rect(0, 0, 2, 2), palette)
	frame0.SetColorIndex(0, 0, 1)
	frame0.SetColorIndex(1, 0, 1)
	frame0.SetColorIndex(0, 1, 1)
	frame0.SetColorIndex(1, 1, 1)

	frame1 := image.NewPaletted(image.Rect(0, 0, 2, 2), palette)
	frame1.SetColorIndex(0, 0, 2)
	frame1.SetColorIndex(1, 0, 2)
	frame1.SetColorIndex(0, 1, 2)
	frame1.SetColorIndex(1, 1, 2)

	g := &gif.GIF{
		Image: []*image.Paletted{frame0, frame1},
		Delay: []int{10, 10},
		Config: image.Config{
			Width:      2,
			Height:     2,
			ColorModel: palette,
		},
	}

	var buf bytes.Buffer
	if err := gif.EncodeAll(&buf, g); err != nil {
		t.Fatalf("gif.EncodeAll: %v", err)
	}
	return buf.Bytes()
}
