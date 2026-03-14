package image

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

type errReadCloser struct{}

func (errReadCloser) Read([]byte) (int, error) { return 0, errors.New("read") }
func (errReadCloser) Close() error             { return nil }

func pngBytes(t *testing.T) []byte {
	t.Helper()

	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("png.Encode: %v", err)
	}
	return buf.Bytes()
}

func TestCacheEntryHelpers(t *testing.T) {
	c := NewCache(nil)
	if c.client != http.DefaultClient {
		t.Fatal("expected nil client to use http.DefaultClient")
	}

	c.entries["loading"] = &entry{loading: true}
	c.entries["failed"] = &entry{failed: true}

	staticImg := image.NewRGBA(image.Rect(0, 0, 1, 1))
	c.entries["static"] = &entry{img: staticImg}

	gifData := buildAnimatedGIF(t)
	firstFrame, anim, err := decodeImageData(gifData, "https://cdn.discordapp.com/emojis/1.gif")
	if err != nil {
		t.Fatalf("decodeImageData returned error: %v", err)
	}
	c.entries["animated"] = &entry{img: firstFrame, anim: anim}

	if img, ok := c.Get("missing"); ok || img != nil {
		t.Fatal("expected missing entry to be unavailable")
	}
	if img, ok := c.Get("loading"); ok || img != nil {
		t.Fatal("expected loading entry to be unavailable")
	}
	if img, ok := c.Get("failed"); ok || img != nil {
		t.Fatal("expected failed entry to be unavailable")
	}
	if img, ok := c.Get("static"); !ok || img == nil {
		t.Fatal("expected static entry to be available")
	}

	if _, frameIndex, _, animated, ok := c.GetFrame("missing", time.Now()); ok || animated || frameIndex != -1 {
		t.Fatal("expected missing frame lookup to be unavailable")
	}
	if img, frameIndex, delay, animated, ok := c.GetFrame("static", time.Now()); !ok || animated || frameIndex != 0 || delay != 0 || img == nil {
		t.Fatalf("unexpected static frame result: ok=%v animated=%v frame=%d delay=%v img=%v", ok, animated, frameIndex, delay, img != nil)
	}
	if img, frameIndex, delay, animated, ok := c.GetFrame("animated", time.Unix(0, 0)); !ok || !animated || frameIndex < 0 || delay <= 0 || img == nil {
		t.Fatalf("unexpected animated frame result: ok=%v animated=%v frame=%d delay=%v img=%v", ok, animated, frameIndex, delay, img != nil)
	}

	if c.Failed("missing") {
		t.Fatal("expected missing entry not to be marked failed")
	}
	if !c.Failed("failed") {
		t.Fatal("expected failed entry to be marked failed")
	}
	if !c.Requested("static") {
		t.Fatal("expected existing entry to count as requested")
	}
	if c.Requested("absent") {
		t.Fatal("expected absent entry not to count as requested")
	}
}

func TestCacheRequest(t *testing.T) {
	pngData := pngBytes(t)

	var requests int
	client := &http.Client{
		Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			requests++
			if req.Method != http.MethodGet {
				t.Fatalf("unexpected method %q", req.Method)
			}
			if ua := req.Header.Get("User-Agent"); ua != "discordo" {
				t.Fatalf("unexpected user agent %q", ua)
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewReader(pngData)),
			}, nil
		}),
	}
	c := NewCache(client)

	skipped := NewCache(client)
	skipped.Request("https://example.com/skip.png", 2, 3, nil)
	if skipped.Requested("https://example.com/skip.png") {
		t.Fatal("expected oversized attachment to skip request setup")
	}

	ready := make(chan struct{}, 1)
	url := "https://example.com/image.png"
	c.Request(url, 0, 0, func() { ready <- struct{}{} })
	select {
	case <-ready:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for async image request")
	}

	if requests != 1 {
		t.Fatalf("expected exactly one request, got %d", requests)
	}
	if !c.Requested(url) {
		t.Fatal("expected successful request to remain tracked")
	}
	if img, ok := c.Get(url); !ok || img == nil {
		t.Fatal("expected requested image to be cached")
	}
	if c.Failed(url) {
		t.Fatal("expected successful request not to be marked failed")
	}

	c.Request(url, 0, 0, func() { t.Fatal("duplicate request should not invoke callback") })
	time.Sleep(20 * time.Millisecond)
	if requests != 1 {
		t.Fatalf("expected duplicate request to be ignored, got %d fetches", requests)
	}
}

func TestCacheRequestFailure(t *testing.T) {
	client := &http.Client{
		Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
			return nil, errors.New("boom")
		}),
	}
	c := NewCache(client)

	ready := make(chan struct{}, 1)
	url := "https://example.com/fail.png"
	c.Request(url, 0, 0, func() { ready <- struct{}{} })
	select {
	case <-ready:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for failed async image request")
	}

	if !c.Requested(url) {
		t.Fatal("expected failed request to remain tracked")
	}
	if !c.Failed(url) {
		t.Fatal("expected failed request to be marked failed")
	}
	if img, ok := c.Get(url); ok || img != nil {
		t.Fatal("expected failed request not to produce a cached image")
	}
}

func TestCacheDownloadAndDecodeErrors(t *testing.T) {
	t.Run("bad url", func(t *testing.T) {
		c := NewCache(http.DefaultClient)
		if _, err := c.downloadAndDecode("://bad-url", 0); err == nil {
			t.Fatal("expected request construction error")
		}
	})

	t.Run("transport error", func(t *testing.T) {
		c := NewCache(&http.Client{
			Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
				return nil, errors.New("transport")
			}),
		})
		if _, err := c.downloadAndDecode("https://example.com/image.png", 0); err == nil || !strings.Contains(err.Error(), "http get") {
			t.Fatalf("expected transport error, got %v", err)
		}
	})

	t.Run("non-200", func(t *testing.T) {
		c := NewCache(&http.Client{
			Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: http.StatusTeapot, Body: io.NopCloser(strings.NewReader(""))}, nil
			}),
		})
		if _, err := c.downloadAndDecode("https://example.com/image.png", 0); err == nil || !strings.Contains(err.Error(), "http status") {
			t.Fatalf("expected status error, got %v", err)
		}
	})

	t.Run("read error", func(t *testing.T) {
		c := NewCache(&http.Client{
			Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: http.StatusOK, Body: errReadCloser{}}, nil
			}),
		})
		if _, err := c.downloadAndDecode("https://example.com/image.png", 0); err == nil || !strings.Contains(err.Error(), "read image") {
			t.Fatalf("expected read error, got %v", err)
		}
	})

	t.Run("too large", func(t *testing.T) {
		c := NewCache(&http.Client{
			Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("abcdef"))}, nil
			}),
		})
		if _, err := c.downloadAndDecode("https://example.com/image.png", 3); err == nil || !strings.Contains(err.Error(), "image too large") {
			t.Fatalf("expected max-file-size error, got %v", err)
		}
	})

	t.Run("decode error", func(t *testing.T) {
		c := NewCache(&http.Client{
			Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("not-an-image"))}, nil
			}),
		})
		if _, err := c.downloadAndDecode("https://example.com/image.png", 0); err == nil || !strings.Contains(err.Error(), "image decode") {
			t.Fatalf("expected decode error, got %v", err)
		}
	})
}

