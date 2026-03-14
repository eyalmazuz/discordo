package image

import (
	"image"
	"image/color"
	"image/draw"
	"image/gif"
	"testing"
	"time"
)

func TestNewAnimation_Disposal(t *testing.T) {
	rect := image.Rect(0, 0, 10, 10)
	img1 := image.NewPaletted(rect, color.Palette{color.Transparent, color.Black})
	img2 := image.NewPaletted(rect, color.Palette{color.Transparent, color.Black})

	g := &gif.GIF{
		Image:    []*image.Paletted{img1, img2},
		Delay:    []int{10, 10},
		Disposal: []byte{gif.DisposalBackground, gif.DisposalPrevious},
		Config:   image.Config{Width: 10, Height: 10},
	}

	anim := newAnimation(g, time.Now())
	if anim == nil {
		t.Fatal("expected animation to be created")
	}
	if len(anim.frames) != 2 {
		t.Errorf("expected 2 frames, got %d", len(anim.frames))
	}
}

func TestNewAnimation_DisposalPreviousRestoresCanvas(t *testing.T) {
	palette := color.Palette{
		color.Transparent,
		color.RGBA{R: 255, A: 255},
		color.RGBA{B: 255, A: 255},
	}
	rect := image.Rect(0, 0, 1, 1)
	img1 := image.NewPaletted(rect, palette)
	img1.SetColorIndex(0, 0, 1)
	img2 := image.NewPaletted(rect, palette)
	img2.SetColorIndex(0, 0, 2)
	img3 := image.NewPaletted(rect, palette)

	g := &gif.GIF{
		Image:    []*image.Paletted{img1, img2, img3},
		Delay:    []int{10, 10, 10},
		Disposal: []byte{gif.DisposalNone, gif.DisposalPrevious, gif.DisposalNone},
		Config:   image.Config{Width: 1, Height: 1},
	}

	anim := newAnimation(g, time.Now())
	if anim == nil || len(anim.frames) != 3 {
		t.Fatalf("expected three-frame animation, got %#v", anim)
	}

	last, ok := anim.frames[2].(*image.RGBA)
	if !ok {
		t.Fatalf("expected RGBA frame, got %T", anim.frames[2])
	}
	if got := last.RGBAAt(0, 0); got.R != 255 || got.B != 0 {
		t.Fatalf("expected disposal-previous restore to bring back red frame, got %#v", got)
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

func TestAnimationHelpersAndEdgeCases(t *testing.T) {
	t.Run("NewAnimationNilAndSingleFrame", func(t *testing.T) {
		if anim := newAnimation(nil, time.Now()); anim != nil {
			t.Fatal("expected nil animation for nil GIF")
		}

		palette := color.Palette{color.Transparent, color.Black}
		single := &gif.GIF{
			Image:  []*image.Paletted{image.NewPaletted(image.Rect(0, 0, 1, 1), palette)},
			Delay:  []int{0},
			Config: image.Config{Width: 1, Height: 1},
		}
		if anim := newAnimation(single, time.Now()); anim != nil {
			t.Fatal("expected single-frame GIF not to create animation")
		}

	})

	t.Run("FrameAtSingleFrameAndNegativeElapsed", func(t *testing.T) {
		img := image.NewRGBA(image.Rect(0, 0, 1, 1))
		anim := &animation{
			frames:        []image.Image{img},
			delays:        []time.Duration{100 * time.Millisecond},
			totalDuration: 100 * time.Millisecond,
			startedAt:     time.Unix(10, 0),
		}

		got, idx, next := anim.FrameAt(time.Unix(0, 0))
		if got != img || idx != 0 || next != 0 {
			t.Fatalf("unexpected single-frame result: got=%v idx=%d next=%v", got, idx, next)
		}
	})

	t.Run("FrameAtNegativeElapsedMultiFrame", func(t *testing.T) {
		imgA := image.NewRGBA(image.Rect(0, 0, 1, 1))
		imgB := image.NewRGBA(image.Rect(0, 0, 1, 1))
		anim := &animation{
			frames:        []image.Image{imgA, imgB},
			delays:        []time.Duration{100 * time.Millisecond, 50 * time.Millisecond},
			totalDuration: 150 * time.Millisecond,
			startedAt:     time.Unix(10, 0),
		}

		got, idx, next := anim.FrameAt(time.Unix(0, 0))
		if got != imgA || idx != 0 || next != 100*time.Millisecond {
			t.Fatalf("unexpected multi-frame negative elapsed result: got=%v idx=%d next=%v", got, idx, next)
		}
	})

	t.Run("FrameAtZeroTotalDurationMultiFrame", func(t *testing.T) {
		imgA := image.NewRGBA(image.Rect(0, 0, 1, 1))
		imgB := image.NewRGBA(image.Rect(0, 0, 1, 1))
		anim := &animation{
			frames:        []image.Image{imgA, imgB},
			delays:        []time.Duration{100 * time.Millisecond, 50 * time.Millisecond},
			totalDuration: 0,
			startedAt:     time.Unix(10, 0),
		}

		got, idx, next := anim.FrameAt(time.Unix(20, 0))
		if got != imgA || idx != 0 || next != 0 {
			t.Fatalf("unexpected zero-total result: got=%v idx=%d next=%v", got, idx, next)
		}
	})

	t.Run("FrameAtFallsBackToLastFrameForInconsistentTotals", func(t *testing.T) {
		imgA := image.NewRGBA(image.Rect(0, 0, 1, 1))
		imgB := image.NewRGBA(image.Rect(0, 0, 1, 1))
		anim := &animation{
			frames:        []image.Image{imgA, imgB},
			delays:        []time.Duration{10 * time.Millisecond, 20 * time.Millisecond},
			totalDuration: 100 * time.Millisecond,
			startedAt:     time.Unix(0, 0),
		}

		got, idx, next := anim.FrameAt(time.Unix(0, int64(50*time.Millisecond)))
		if got != imgB || idx != 1 || next != 20*time.Millisecond {
			t.Fatalf("unexpected fallback result: got=%v idx=%d next=%v", got, idx, next)
		}
	})

	t.Run("NormalizeDelay", func(t *testing.T) {
		if got := normalizeGIFDelay(0); got != 100*time.Millisecond {
			t.Fatalf("expected zero delay fallback, got %v", got)
		}
		if got := normalizeGIFDelay(1); got != 20*time.Millisecond {
			t.Fatalf("expected clamped short delay, got %v", got)
		}
		if got := normalizeGIFDelay(5); got != 50*time.Millisecond {
			t.Fatalf("expected normal delay, got %v", got)
		}
	})

	t.Run("ClearRectAndClone", func(t *testing.T) {
		rgba := image.NewRGBA(image.Rect(0, 0, 2, 2))
		draw.Draw(rgba, rgba.Bounds(), &image.Uniform{C: color.RGBA{R: 200, A: 255}}, image.Point{}, draw.Src)
		clearRect(nil, rgba.Bounds())
		clearRect(rgba, image.Rect(0, 0, 1, 1))
		if got := rgba.RGBAAt(0, 0); got.A != 0 {
			t.Fatalf("expected cleared pixel to be transparent, got %#v", got)
		}

		if cloneRGBA(nil) != nil {
			t.Fatal("expected nil clone for nil source")
		}
		clone := cloneRGBA(rgba)
		if clone == nil || clone == rgba {
			t.Fatal("expected deep RGBA clone")
		}
	})

	t.Run("DelayAndDisposalBounds", func(t *testing.T) {
		g := &gif.GIF{
			Delay:    []int{7},
			Disposal: []byte{gif.DisposalPrevious},
		}
		if got := delayAt(g, -1); got != 0 {
			t.Fatalf("expected out-of-range delay to be 0, got %d", got)
		}
		if got := delayAt(g, 0); got != 7 {
			t.Fatalf("expected in-range delay, got %d", got)
		}
		if got := disposalAt(g, 1); got != gif.DisposalNone {
			t.Fatalf("expected out-of-range disposal to be none, got %d", got)
		}
		if got := disposalAt(g, 0); got != gif.DisposalPrevious {
			t.Fatalf("expected in-range disposal, got %d", got)
		}
	})
}
