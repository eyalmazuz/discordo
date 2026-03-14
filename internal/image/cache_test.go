package image

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/gif"
	"image/png"
	"io"
	"net/http"
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

func TestAnimation_FrameAt_Cache(t *testing.T) {
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

func TestCacheStateAccessors(t *testing.T) {
	static := image.NewRGBA(image.Rect(0, 0, 1, 1))
	anim := &animation{
		frames:        []image.Image{static, image.NewRGBA(image.Rect(0, 0, 1, 1))},
		delays:        []time.Duration{100 * time.Millisecond, 100 * time.Millisecond},
		totalDuration: 200 * time.Millisecond,
		startedAt:     time.Unix(0, 0),
	}

	c := &Cache{
		client:  http.DefaultClient,
		entries: map[string]*entry{},
	}

	if img, ok := c.Get("missing"); img != nil || ok {
		t.Fatalf("expected missing Get to be empty, got %v %v", img, ok)
	}
	if _, idx, _, animated, ok := c.GetFrame("missing", time.Now()); idx != -1 || animated || ok {
		t.Fatalf("expected missing GetFrame to be empty, got idx=%d animated=%v ok=%v", idx, animated, ok)
	}
	if c.Failed("missing") {
		t.Fatal("expected missing entry not to be failed")
	}
	if c.Requested("missing") {
		t.Fatal("expected missing entry not to be requested")
	}

	c.entries["loading"] = &entry{loading: true}
	if img, ok := c.Get("loading"); img != nil || ok {
		t.Fatalf("expected loading Get to be empty, got %v %v", img, ok)
	}
	if _, idx, _, animated, ok := c.GetFrame("loading", time.Now()); idx != -1 || animated || ok {
		t.Fatalf("expected loading GetFrame to be empty, got idx=%d animated=%v ok=%v", idx, animated, ok)
	}

	c.entries["failed"] = &entry{failed: true}
	if !c.Failed("failed") {
		t.Fatal("expected failed entry to report failed")
	}

	c.entries["static"] = &entry{img: static}
	if img, ok := c.Get("static"); img == nil || !ok {
		t.Fatal("expected static image to be available")
	}
	if img, idx, next, animated, ok := c.GetFrame("static", time.Now()); img == nil || idx != 0 || next != 0 || animated || !ok {
		t.Fatalf("unexpected static frame result: img=%v idx=%d next=%v animated=%v ok=%v", img, idx, next, animated, ok)
	}

	c.entries["anim"] = &entry{img: static, anim: anim}
	if img, idx, next, animated, ok := c.GetFrame("anim", time.Unix(0, 0)); img == nil || idx != 0 || next <= 0 || !animated || !ok {
		t.Fatalf("unexpected animated frame result: img=%v idx=%d next=%v animated=%v ok=%v", img, idx, next, animated, ok)
	}
	if !c.Requested("anim") {
		t.Fatal("expected anim entry to report requested")
	}
}

func TestNewCache_DefaultClient(t *testing.T) {
	c := NewCache(nil)
	if c.client == nil {
		t.Fatal("expected default client to be installed")
	}
	if c.entries == nil {
		t.Fatal("expected entries map to be initialized")
	}
}

func TestCacheRequest_SuccessFailureAndSkips(t *testing.T) {
	pngData := buildPNG(t)

	t.Run("SkipsTooLargeAttachment", func(t *testing.T) {
		c := NewCache(http.DefaultClient)
		c.Request("https://example.invalid/skip.png", 4, 5, func() {
			t.Fatal("onReady should not be called when attachment is skipped")
		})
		if c.Requested("https://example.invalid/skip.png") {
			t.Fatal("expected oversized attachment not to be requested")
		}
	})

	t.Run("LoadsOnceAndCaches", func(t *testing.T) {
		requests := 0
		url := "https://example.com/static.png"
		c := NewCache(&http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			requests++
			if req.URL.String() != url {
				t.Fatalf("unexpected request URL %q", req.URL.String())
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewReader(pngData)),
				Header:     make(http.Header),
			}, nil
		})})
		ready := make(chan struct{}, 2)
		c.Request(url, 0, 0, func() { ready <- struct{}{} })
		c.Request(url, 0, 0, func() { ready <- struct{}{} })

		select {
		case <-ready:
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for cache request")
		}

		if requests != 1 {
			t.Fatalf("expected single request for duplicate cache loads, got %d", requests)
		}
		if !c.Requested(url) {
			t.Fatal("expected URL to be marked requested")
		}
		if c.Failed(url) {
			t.Fatal("expected successful request not to be marked failed")
		}
		if img, ok := c.Get(url); img == nil || !ok {
			t.Fatal("expected loaded image to be retrievable")
		}
	})

	t.Run("MarksFailuresAndCallsReady", func(t *testing.T) {
		url := "https://example.com/fail.png"
		c := NewCache(&http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusBadGateway,
				Body:       io.NopCloser(bytes.NewReader(nil)),
				Header:     make(http.Header),
			}, nil
		})})
		ready := make(chan struct{}, 1)
		c.Request(url, 0, 0, func() { ready <- struct{}{} })

		select {
		case <-ready:
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for failed cache request")
		}

		if !c.Failed(url) {
			t.Fatal("expected failed request to be marked failed")
		}
		if img, ok := c.Get(url); img != nil || ok {
			t.Fatalf("expected failed request to be unavailable, got %v %v", img, ok)
		}
	})
}

func TestCacheDownloadAndDecode_Errors(t *testing.T) {
	t.Run("InvalidRequestURL", func(t *testing.T) {
		c := NewCache(http.DefaultClient)
		if _, err := c.downloadAndDecode("http://bad url", 0); err == nil {
			t.Fatal("expected invalid URL to fail")
		}
	})

	t.Run("TransportError", func(t *testing.T) {
		c := NewCache(&http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return nil, errors.New("dial failed")
		})})
		if _, err := c.downloadAndDecode("https://example.com/image.png", 0); err == nil {
			t.Fatal("expected transport error")
		}
	})

	t.Run("HTTPStatusError", func(t *testing.T) {
		c := NewCache(&http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusTeapot,
				Body:       io.NopCloser(bytes.NewReader(nil)),
				Header:     make(http.Header),
			}, nil
		})})
		if _, err := c.downloadAndDecode("https://example.com/image.png", 0); err == nil {
			t.Fatal("expected http status error")
		}
	})

	t.Run("ReadError", func(t *testing.T) {
		c := NewCache(&http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(errReader{}),
				Header:     make(http.Header),
			}, nil
		})})
		if _, err := c.downloadAndDecode("https://example.com/image.png", 0); err == nil {
			t.Fatal("expected read error")
		}
	})

	t.Run("TooLarge", func(t *testing.T) {
		payload := bytes.Repeat([]byte("a"), 8)
		c := NewCache(&http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewReader(payload)),
				Header:     make(http.Header),
			}, nil
		})})
		if _, err := c.downloadAndDecode("https://example.com/image.png", 4); err == nil {
			t.Fatal("expected size limit error")
		}
	})
}

func TestCacheDownloadAndDecode_SuccessAndAnimation(t *testing.T) {
	pngData := buildPNG(t)
	gifData := buildAnimatedGIF(t)

	t.Run("StaticImage", func(t *testing.T) {
		c := NewCache(&http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewReader(pngData)),
				Header:     make(http.Header),
			}, nil
		})})
		c.entries["https://example.com/static.png"] = &entry{}
		img, err := c.downloadAndDecode("https://example.com/static.png", 0)
		if err != nil || img == nil {
			t.Fatalf("expected static image success, got img=%v err=%v", img, err)
		}
		if c.entries["https://example.com/static.png"].anim != nil {
			t.Fatal("expected static image not to install animation")
		}
	})

	t.Run("AnimatedImage", func(t *testing.T) {
		c := NewCache(&http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewReader(gifData)),
				Header:     make(http.Header),
			}, nil
		})})
		url := "https://example.com/emoji.gif"
		c.entries[url] = &entry{}
		img, err := c.downloadAndDecode(url, 0)
		if err != nil || img == nil {
			t.Fatalf("expected animated image success, got img=%v err=%v", img, err)
		}
		if c.entries[url].anim == nil {
			t.Fatal("expected animated image to install animation")
		}
	})
}

func TestDecodeImageData_FallbacksAndErrors(t *testing.T) {
	pngData := buildPNG(t)

	t.Run("PNG", func(t *testing.T) {
		img, anim, err := decodeImageData(pngData, "https://example.com/static.png")
		if err != nil || img == nil || anim != nil {
			t.Fatalf("expected static decode success, got img=%v anim=%v err=%v", img, anim, err)
		}
	})

	t.Run("GIFSuffixFallsBackToStaticDecode", func(t *testing.T) {
		img, anim, err := decodeImageData(pngData, "https://example.com/not-really.gif")
		if err != nil || img == nil || anim != nil {
			t.Fatalf("expected gif suffix fallback to static decode, got img=%v anim=%v err=%v", img, anim, err)
		}
	})

	t.Run("DecodeError", func(t *testing.T) {
		if _, _, err := decodeImageData([]byte("not-an-image"), "https://example.com/bad.bin"); err == nil {
			t.Fatal("expected decode failure")
		}
	})
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

func buildPNG(t *testing.T) []byte {
	t.Helper()

	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	img.Set(0, 0, color.RGBA{R: 1, G: 2, B: 3, A: 255})

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("png.Encode: %v", err)
	}
	return buf.Bytes()
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

type errReader struct{}

func (errReader) Read([]byte) (int, error) {
	return 0, errors.New("read failed")
}
