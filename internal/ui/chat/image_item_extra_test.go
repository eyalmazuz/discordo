package chat

import (
	"bytes"
	"image"
	"image/color"
	"image/gif"
	"image/png"
	"io"
	"net/http"
	"reflect"
	"testing"
	"time"
	"unsafe"

	imgpkg "github.com/ayn2op/discordo/internal/image"
	"github.com/ayn2op/tview"
	"github.com/gdamore/tcell/v3"
)

type zeroBoundsImage struct{}

func (zeroBoundsImage) ColorModel() color.Model { return color.RGBAModel }
func (zeroBoundsImage) Bounds() image.Rectangle { return image.Rect(0, 0, 0, 8) }
func (zeroBoundsImage) At(int, int) color.Color { return color.RGBA{} }

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

type lockingScreen struct {
	completeMockScreen
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

type failingWindowSizeTty struct{}

func (failingWindowSizeTty) Close() error                { return nil }
func (failingWindowSizeTty) Read(p []byte) (int, error)  { return 0, nil }
func (failingWindowSizeTty) Write(p []byte) (int, error) { return len(p), nil }
func (failingWindowSizeTty) Size() (int, int, error)     { return 80, 24, nil }
func (failingWindowSizeTty) Drain() error                { return nil }
func (failingWindowSizeTty) NotifyResize(chan<- bool)    {}
func (failingWindowSizeTty) Stop() error                 { return nil }
func (failingWindowSizeTty) Start() error                { return nil }
func (failingWindowSizeTty) WindowSize() (tcell.WindowSize, error) {
	return tcell.WindowSize{}, io.EOF
}

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

func injectCachedImage(t *testing.T, cache *imgpkg.Cache, url string, img image.Image) {
	t.Helper()

	cacheValue := reflect.ValueOf(cache).Elem()
	entriesField := cacheValue.FieldByName("entries")
	entries := reflect.NewAt(entriesField.Type(), unsafe.Pointer(entriesField.UnsafeAddr())).Elem()

	entryPtr := reflect.New(entries.Type().Elem().Elem())
	imgField := entryPtr.Elem().FieldByName("img")
	reflect.NewAt(imgField.Type(), unsafe.Pointer(imgField.UnsafeAddr())).Elem().Set(reflect.ValueOf(img))

	entries.SetMapIndex(reflect.ValueOf(url), entryPtr)
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

	zeroCache := imgpkg.NewCache(nil)
	injectCachedImage(t, zeroCache, "https://example.com/zero-bounds.png", zeroBoundsImage{})
	zeroBounds := newImageItem(zeroCache, "https://example.com/zero-bounds.png", 10, 4, false, 5, nil, nil)
	if got := zeroBounds.Height(10); got != 1 {
		t.Fatalf("expected zero-bounds image height 1, got %d", got)
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

	unchanged := newImageItem(imgpkg.NewCache(nil), "https://example.com/image.png", 10, 4, true, 2, nil, nil)
	unchanged.initCellDimensions(&ttyScreen{tty: failingWindowSizeTty{}})
	if unchanged.cellW != 0 || unchanged.cellH != 0 || unchanged.initted {
		t.Fatal("expected failed window-size lookup to leave cell dimensions unset")
	}
}

func loadAnimatedImageCache(t *testing.T, url string) *imgpkg.Cache {
	t.Helper()

	palette := color.Palette{color.Transparent, color.Black, color.White}
	frame1 := image.NewPaletted(image.Rect(0, 0, 2, 2), palette)
	frame1.SetColorIndex(0, 0, 1)
	frame2 := image.NewPaletted(image.Rect(0, 0, 2, 2), palette)
	frame2.SetColorIndex(1, 1, 2)

	var buf bytes.Buffer
	if err := gif.EncodeAll(&buf, &gif.GIF{
		Image:     []*image.Paletted{frame1, frame2},
		Delay:     []int{5, 5},
		Disposal:  []byte{gif.DisposalNone, gif.DisposalNone},
		LoopCount: 0,
		Config:    image.Config{Width: 2, Height: 2},
	}); err != nil {
		t.Fatalf("encode gif: %v", err)
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
	return cache
}

func TestImageItemDrawAdditionalBranches(t *testing.T) {
	t.Run("zero-size rect returns without drawing", func(t *testing.T) {
		cache := loadTestImageCache(t, "https://example.com/zero.png", image.NewRGBA(image.Rect(0, 0, 10, 10)))
		screen := &completeMockScreen{}
		item := newImageItem(cache, "https://example.com/zero.png", 10, 4, false, 1, nil, nil)
		item.SetRect(0, 0, 0, 0)
		item.Draw(screen)
		if len(screen.Content) != 0 {
			t.Fatal("expected zero-size draw to leave screen untouched")
		}
	})

	t.Run("loading non-emote renders placeholder text", func(t *testing.T) {
		screen := &putTrackingScreen{}
		item := newImageItem(imgpkg.NewCache(nil), "https://example.com/loading.png", 10, 4, false, 2, nil, nil)
		item.SetRect(0, 0, 10, 4)
		item.Draw(screen)
		if len(screen.Content) == 0 {
			t.Fatal("expected loading placeholder to write visible content")
		}
	})

	t.Run("animated draw requests redraw", func(t *testing.T) {
		url := "https://example.com/animated.gif"
		cache := loadAnimatedImageCache(t, url)
		requested := make(chan time.Duration, 1)
		item := newImageItem(cache, url, 10, 4, false, 3, func() (int, int, int, int) {
			return 0, 0, 80, 24
		}, func(after time.Duration) {
			requested <- after
		})
		item.SetRect(0, 0, 10, 4)
		screen := &completeMockScreen{}
		item.Draw(screen)

		select {
		case after := <-requested:
			if after <= 0 {
				t.Fatalf("expected animated draw to request a positive redraw delay, got %v", after)
			}
		case <-time.After(300 * time.Millisecond):
			t.Fatal("expected animated image draw to schedule a redraw")
		}
	})

	t.Run("actual cell size falls back when source or cell dimensions are zero", func(t *testing.T) {
		item := newImageItem(imgpkg.NewCache(nil), "https://example.com/fallback-size.png", 10, 4, true, 4, nil, nil)
		if cols, rows := item.actualCellSize(image.NewRGBA(image.Rect(0, 0, 10, 10)), 3, 2); cols != 3 || rows != 2 {
			t.Fatalf("expected unset cell dimensions to fall back to caller size, got %dx%d", cols, rows)
		}

		item.setCellDimensions(10, 20)
		if cols, rows := item.actualCellSize(image.NewRGBA(image.Rect(0, 0, 0, 0)), 3, 2); cols != 3 || rows != 2 {
			t.Fatalf("expected empty source image to fall back to caller size, got %dx%d", cols, rows)
		}
	})

	t.Run("identical kitty placement skips requeue", func(t *testing.T) {
		_, item, img := setupMockImageItem(true, 0, 20)
		item.drawKitty(&completeMockScreen{}, img, 0, 0, 4, 2)
		item.pendingPlace = false
		item.drawKitty(&completeMockScreen{}, img, 0, 0, 4, 2)
		if item.pendingPlace {
			t.Fatal("expected identical kitty placement to skip requeueing placement")
		}
	})

	t.Run("kitty draw without viewport uses fallback bounds", func(t *testing.T) {
		cache := loadTestImageCache(t, "https://example.com/fallback-viewport.png", image.NewRGBA(image.Rect(0, 0, 20, 20)))
		item := newImageItem(cache, "https://example.com/fallback-viewport.png", 4, 2, true, 5, nil, nil)
		item.setCellDimensions(10, 20)
		item.initted = true
		item.drawKitty(&completeMockScreen{}, image.NewRGBA(image.Rect(0, 0, 20, 20)), 0, 0, 4, 2)
		if !item.drawnThisFrame {
			t.Fatal("expected kitty draw without viewport callback to use fallback bounds")
		}
	})

	t.Run("kitty payload encode error leaves payload empty", func(t *testing.T) {
		item := newImageItem(imgpkg.NewCache(nil), "https://example.com/bad-payload.png", 4, 2, true, 6, nil, nil)
		item.setCellDimensions(10, 20)
		item.initted = true
		item.drawKitty(&completeMockScreen{}, image.NewRGBA(image.Rect(0, 0, 0, 0)), 0, 0, 4, 2)
		if item.kittyPayload != "" {
			t.Fatal("expected kitty encode failure to leave payload empty")
		}
	})

		t.Run("half-block fallback viewport and height cap", func(t *testing.T) {
			screen := &completeMockScreen{}
			item := newImageItem(imgpkg.NewCache(nil), "https://example.com/half.png", 10, 4, false, 7, nil, nil)
			item.drawHalfBlock(screen, image.NewRGBA(image.Rect(0, 0, 20, 20)), 0, 0, 4, 1)
		for key := range screen.Content {
			switch key {
			case "0,0", "1,0", "2,0", "3,0":
			default:
				t.Fatalf("expected half-block draw to stop at the requested height, saw cell %s", key)
				}
			}
		})

	t.Run("half-block stops iterating beyond requested height", func(t *testing.T) {
		screen := &completeMockScreen{}
		item := newImageItem(imgpkg.NewCache(nil), "https://example.com/half-prefilled.png", 10, 4, false, 8, nil, nil)
		item.renderedWidth = 2
		item.renderedLines = []tview.Line{
			{{Text: "A", Style: tcell.StyleDefault}},
			{{Text: "B", Style: tcell.StyleDefault}},
		}

		item.drawHalfBlock(screen, image.NewRGBA(image.Rect(0, 0, 2, 2)), 0, 0, 2, 1)

		if got := screen.Content["0,0"]; got != 'A' {
			t.Fatalf("expected first row to render, got %q", got)
		}
		if _, ok := screen.Content["0,1"]; ok {
			t.Fatal("expected drawHalfBlock to stop before the second rendered row")
		}
	})
}
