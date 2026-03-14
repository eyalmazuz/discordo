package chat

import (
	"bytes"
	"image"
	"image/png"
	"io"
	"net/http"
	"testing"

	imgpkg "github.com/ayn2op/discordo/internal/image"
	"github.com/ayn2op/tview"
	"github.com/gdamore/tcell/v3"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

type lockingScreen struct {
	MockScreen
	lockCalls int
}

func (s *lockingScreen) LockRegion(x, y, width, height int, lock bool) {
	s.lockCalls++
}

type cellSizeTty struct{}

func (cellSizeTty) Close() error                { return nil }
func (cellSizeTty) Read(p []byte) (int, error)  { return 0, nil }
func (cellSizeTty) Write(p []byte) (int, error) { return len(p), nil }
func (cellSizeTty) Size() (int, int, error)     { return 80, 24, nil }
func (cellSizeTty) Drain() error                { return nil }
func (cellSizeTty) NotifyResize(chan<- bool)    {}
func (cellSizeTty) Stop() error                 { return nil }
func (cellSizeTty) Start() error                { return nil }
func (cellSizeTty) WindowSize() (tcell.WindowSize, error) {
	return tcell.WindowSize{Width: 80, Height: 24, PixelWidth: 800, PixelHeight: 480}, nil
}

type ttyScreen struct {
	completeMockScreen
	tty tcell.Tty
}

func (s *ttyScreen) Tty() (tcell.Tty, bool) { return s.tty, true }

type putTrackingScreen struct {
	completeMockScreen
}

func (s *putTrackingScreen) Put(x, y int, text string, style tcell.Style) (string, int) {
	for offset, r := range text {
		s.SetContent(x+offset, y, r, nil, style)
	}
	return text, len([]rune(text))
}

func loadTestImageCache(t *testing.T, url string, img image.Image) *imgpkg.Cache {
	t.Helper()

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}

	client := &http.Client{
		Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(bytes.NewReader(buf.Bytes())),
			}, nil
		}),
	}

	cache := imgpkg.NewCache(client)
	done := make(chan struct{}, 1)
	cache.Request(url, 0, 0, func() { done <- struct{}{} })
	<-done

	if _, ok := cache.Get(url); !ok {
		t.Fatalf("expected cached image for %s", url)
	}
	return cache
}

func TestImageItemHeightBranches(t *testing.T) {
	url := "https://example.com/image.png"
	cache := loadTestImageCache(t, url, image.NewRGBA(image.Rect(0, 0, 100, 50)))

	missing := newImageItem(imgpkg.NewCache(nil), url, 10, 4, false, 1, nil, nil)
	if got := missing.Height(10); got != 1 {
		t.Fatalf("expected loading placeholder height 1, got %d", got)
	}

	kittyNoCells := newImageItem(cache, url, 10, 4, true, 2, nil, nil)
	if got := kittyNoCells.Height(10); got != 1 {
		t.Fatalf("expected kitty height 1 without cell dimensions, got %d", got)
	}

	kitty := newImageItem(cache, url, 10, 2, true, 3, nil, nil)
	kitty.setCellDimensions(10, 20)
	if got := kitty.Height(10); got != 2 {
		t.Fatalf("expected kitty height clamped to 2, got %d", got)
	}

	halfBlock := newImageItem(cache, url, 10, 10, false, 4, nil, nil)
	if got := halfBlock.Height(5); got != 1 {
		t.Fatalf("expected half-block height 1, got %d", got)
	}
	if got := halfBlock.Height(0); got != 1 {
		t.Fatalf("expected non-positive width to fall back to 1, got %d", got)
	}
}

func TestImageItemDrawBranches(t *testing.T) {
	screen := &MockScreen{}

	emote := newImageItem(imgpkg.NewCache(nil), "https://example.com/emote.png", 2, 1, false, 1, nil, nil)
	emote.SetRect(0, 0, 2, 1)
	emote.Draw(screen)
	if got := screen.Content["0,0"]; got != '…' {
		t.Fatalf("expected emote loading placeholder, got %q", got)
	}

	failedCache := imgpkg.NewCache(&http.Client{
		Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return nil, io.EOF
		}),
	})
	done := make(chan struct{}, 1)
	failedCache.Request("https://example.com/failed.png", 0, 0, func() { done <- struct{}{} })
	<-done

	failedScreen := &putTrackingScreen{}
	failed := newImageItem(failedCache, "https://example.com/failed.png", 10, 4, false, 2, nil, nil)
	failed.SetRect(0, 0, 10, 4)
	failed.Draw(failedScreen)
	if len(failedScreen.Content) == 0 {
		t.Fatal("expected failed-image placeholder to write to the screen")
	}

	cache := loadTestImageCache(t, "https://example.com/fallback.png", image.NewRGBA(image.Rect(0, 0, 40, 40)))
	fallbackScreen := &completeMockScreen{}
	fallback := newImageItem(cache, "https://example.com/fallback.png", 10, 4, true, 3, func() (int, int, int, int) {
		return 0, 0, 80, 24
	}, nil)
	fallback.SetRect(0, 0, 10, 4)
	fallback.Draw(fallbackScreen)
	if len(fallbackScreen.Content) == 0 {
		t.Fatal("expected kitty fallback draw to render half-block content")
	}
}

func TestImageItemSetFrameAndInitCellDimensions(t *testing.T) {
	item := newImageItem(imgpkg.NewCache(nil), "https://example.com/image.png", 10, 4, true, 1, nil, nil)
	item.lastFrameIndex = 1
	item.kittyPlaced = true
	item.kittyUploaded = true
	item.kittyCols = 4
	item.kittyVisibleRows = 2
	item.renderedLines = []tview.Line{{}}
	item.renderedWidth = 8
	item.kittyPayload = "payload"

	screen := &lockingScreen{}

	item.setFrame(screen, -1)
	if item.lastFrameIndex != 1 {
		t.Fatal("negative frame index should be ignored")
	}

	item.setFrame(screen, 1)
	if item.kittyPayload != "payload" {
		t.Fatal("same frame index should leave cached payload untouched")
	}

	item.setFrame(screen, 2)
	if item.lastFrameIndex != 2 {
		t.Fatalf("expected last frame index to update, got %d", item.lastFrameIndex)
	}
	if item.kittyPayload != "" || item.kittyUploaded || item.kittyPlaced || item.pendingPlace {
		t.Fatal("expected frame change to clear kitty cached state")
	}
	if item.renderedLines != nil || item.renderedWidth != 0 {
		t.Fatal("expected frame change to clear half-block cache")
	}
	if screen.lockCalls == 0 {
		t.Fatal("expected frame change to unlock the prior kitty region")
	}

	screenWithTTY := &ttyScreen{tty: cellSizeTty{}}
	item.initCellDimensions(screenWithTTY)
	if item.cellW != 10 || item.cellH != 20 || !item.initted {
		t.Fatalf("expected cell dimensions 10x20 and initialized state, got %dx%d initted=%v", item.cellW, item.cellH, item.initted)
	}
}
