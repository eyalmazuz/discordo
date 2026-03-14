package image

import (
	"image"
	"image/color"
	"image/gif"
	"testing"
	"time"
)

func TestNewAnimation_Disposal(t *testing.T) {
	rect := image.Rect(0, 0, 10, 10)
	img1 := image.NewPaletted(rect, color.Palette{color.Transparent, color.Black})
	img2 := image.NewPaletted(rect, color.Palette{color.Transparent, color.Black})
	
	g := &gif.GIF{
		Image: []*image.Paletted{img1, img2},
		Delay: []int{10, 10},
		Disposal: []byte{gif.DisposalBackground, gif.DisposalPrevious},
		Config: image.Config{Width: 10, Height: 10},
	}

	anim := newAnimation(g, time.Now())
	if anim == nil {
		t.Fatal("expected animation to be created")
	}
	if len(anim.frames) != 2 {
		t.Errorf("expected 2 frames, got %d", len(anim.frames))
	}
}

func TestAnimation_FrameAt(t *testing.T) {
	frames := []image.Image{image.NewRGBA(image.Rect(0, 0, 1, 1)), image.NewRGBA(image.Rect(0, 0, 1, 1))}
	delays := []time.Duration{100 * time.Millisecond, 100 * time.Millisecond}
	start := time.Now()
	anim := &animation{
		frames:        frames,
		delays:        delays,
		totalDuration: 200 * time.Millisecond,
		startedAt:     start,
	}

	t.Run("FirstFrame", func(t *testing.T) {
		_, idx, _ := anim.FrameAt(start.Add(50 * time.Millisecond))
		if idx != 0 {
			t.Errorf("expected frame 0, got %d", idx)
		}
	})

	t.Run("SecondFrame", func(t *testing.T) {
		_, idx, _ := anim.FrameAt(start.Add(150 * time.Millisecond))
		if idx != 1 {
			t.Errorf("expected frame 1, got %d", idx)
		}
	})

	t.Run("Loop", func(t *testing.T) {
		_, idx, _ := anim.FrameAt(start.Add(250 * time.Millisecond))
		if idx != 0 {
			t.Errorf("expected frame 0 after loop, got %d", idx)
		}
	})

	t.Run("NilSafety", func(t *testing.T) {
		var a *animation
		if f := a.FirstFrame(); f != nil {
			t.Error("expected nil")
		}
		if _, idx, _ := a.FrameAt(time.Now()); idx != -1 {
			t.Errorf("expected -1, got %d", idx)
		}
	})
}

func TestIsGIFData(t *testing.T) {
	if !isGIFData([]byte("GIF89a..."), "") {
		t.Error("expected true for GIF89a header")
	}
	if !isGIFData(nil, "test.gif") {
		t.Error("expected true for .gif extension")
	}
}
