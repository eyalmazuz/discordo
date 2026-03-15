package image

import (
	"bytes"
	"image"
	"image/color"
	"image/draw"
	"image/gif"
	"strings"
	"time"
)

type animation struct {
	frames        []image.Image
	delays        []time.Duration
	totalDuration time.Duration
	startedAt     time.Time
}

func newAnimation(g *gif.GIF, startedAt time.Time) *animation {
	if g == nil || len(g.Image) == 0 {
		return nil
	}

	canvas := image.NewRGBA(image.Rect(0, 0, g.Config.Width, g.Config.Height))
	frames := make([]image.Image, 0, len(g.Image))
	delays := make([]time.Duration, 0, len(g.Image))
	var previous *image.RGBA

	for i, src := range g.Image {
		if i > 0 {
			switch disposalAt(g, i-1) {
			case gif.DisposalBackground:
				clearRect(canvas, g.Image[i-1].Bounds())
			case gif.DisposalPrevious:
				if previous != nil {
					draw.Draw(canvas, canvas.Bounds(), previous, image.Point{}, draw.Src)
				}
			}
		}

		if disposalAt(g, i) == gif.DisposalPrevious {
			previous = cloneRGBA(canvas)
		} else {
			previous = nil
		}

		draw.Draw(canvas, src.Bounds(), src, image.Point{}, draw.Over)
		frames = append(frames, cloneRGBA(canvas))
		delays = append(delays, normalizeGIFDelay(delayAt(g, i)))
	}

	if len(frames) <= 1 {
		return nil
	}

	var total time.Duration
	for _, delay := range delays {
		total += delay
	}

	return &animation{
		frames:        frames,
		delays:        delays,
		totalDuration: total,
		startedAt:     startedAt,
	}
}

func (a *animation) FirstFrame() image.Image {
	if a == nil || len(a.frames) == 0 {
		return nil
	}
	return a.frames[0]
}

func (a *animation) FrameAt(now time.Time) (image.Image, int, time.Duration) {
	if a == nil || len(a.frames) == 0 {
		return nil, -1, 0
	}
	if len(a.frames) == 1 || a.totalDuration <= 0 {
		return a.frames[0], 0, 0
	}

	elapsed := now.Sub(a.startedAt)
	if elapsed < 0 {
		elapsed = 0
	}
	elapsed %= a.totalDuration

	var cumulative time.Duration
	for i, delay := range a.delays {
		cumulative += delay
		if elapsed < cumulative {
			return a.frames[i], i, cumulative - elapsed
		}
	}

	last := len(a.frames) - 1
	return a.frames[last], last, a.delays[last]
}

func isGIFData(data []byte, url string) bool {
	if len(data) >= 6 {
		header := data[:6]
		if bytes.Equal(header, []byte("GIF87a")) || bytes.Equal(header, []byte("GIF89a")) {
			return true
		}
	}
	return strings.HasSuffix(strings.ToLower(url), ".gif")
}

func normalizeGIFDelay(delay int) time.Duration {
	if delay <= 0 {
		return 100 * time.Millisecond
	}
	d := time.Duration(delay) * 10 * time.Millisecond
	if d < 20*time.Millisecond {
		return 20 * time.Millisecond
	}
	return d
}

func clearRect(dst *image.RGBA, rect image.Rectangle) {
	if dst == nil {
		return
	}
	draw.Draw(dst, rect.Intersect(dst.Bounds()), &image.Uniform{C: color.Transparent}, image.Point{}, draw.Src)
}

func cloneRGBA(src *image.RGBA) *image.RGBA {
	if src == nil {
		return nil
	}
	clone := image.NewRGBA(src.Bounds())
	draw.Draw(clone, clone.Bounds(), src, src.Bounds().Min, draw.Src)
	return clone
}

func delayAt(g *gif.GIF, i int) int {
	if i < 0 || i >= len(g.Delay) {
		return 0
	}
	return g.Delay[i]
}

func disposalAt(g *gif.GIF, i int) byte {
	if i < 0 || i >= len(g.Disposal) {
		return gif.DisposalNone
	}
	return g.Disposal[i]
}
