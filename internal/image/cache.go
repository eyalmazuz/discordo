package image

import (
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"log/slog"
	"net/http"
	"sync"

	_ "golang.org/x/image/webp"
)

type entry struct {
	img     image.Image
	loading bool
	failed  bool
}

// Cache stores decoded images keyed by URL. Images are downloaded and decoded
// asynchronously; callers are notified via a callback when an image is ready.
type Cache struct {
	mu      sync.RWMutex
	entries map[string]*entry
	client  *http.Client
}

// NewCache creates a new image cache. If client is nil, http.DefaultClient is used.
func NewCache(client *http.Client) *Cache {
	if client == nil {
		client = http.DefaultClient
	}
	return &Cache{
		entries: make(map[string]*entry),
		client:  client,
	}
}

// Get returns the cached image for url, if available.
// The second return value is true when the image is loaded and ready.
func (c *Cache) Get(url string) (image.Image, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	e, ok := c.entries[url]
	if !ok || e.loading || e.failed {
		return nil, false
	}
	return e.img, true
}

// Failed reports whether the image at url failed to download or decode.
func (c *Cache) Failed(url string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	e, ok := c.entries[url]
	return ok && e.failed
}

// Request starts an asynchronous download of the image at url if it is not
// already cached or in-flight. When the image is ready, onReady is called
// (from a goroutine) so the caller can trigger a redraw.
//
// If attachmentSize exceeds maxFileSize the request is skipped.
func (c *Cache) Request(url string, maxFileSize int64, attachmentSize uint64, onReady func()) {
	if maxFileSize > 0 && attachmentSize > uint64(maxFileSize) {
		return
	}

	c.mu.Lock()
	if _, ok := c.entries[url]; ok {
		c.mu.Unlock()
		return
	}

	e := &entry{loading: true}
	c.entries[url] = e
	c.mu.Unlock()

	go func() {
		img, err := c.downloadAndDecode(url, maxFileSize)

		c.mu.Lock()
		if err != nil {
			slog.Error("failed to download image", "url", url, "err", err)
			e.failed = true
			e.loading = false
			c.mu.Unlock()
		} else {
			e.img = img
			e.loading = false
			c.mu.Unlock()
		}

		if onReady != nil {
			onReady()
		}
	}()
}

func (c *Cache) downloadAndDecode(url string, maxFileSize int64) (image.Image, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("http new request: %w", err)
	}
	req.Header.Set("User-Agent", "discordo")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http get: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("http status %d", resp.StatusCode)
	}

	var reader io.Reader = resp.Body
	if maxFileSize > 0 {
		reader = io.LimitReader(resp.Body, maxFileSize+1)
	}

	img, _, err := image.Decode(reader)
	if err != nil {
		return nil, fmt.Errorf("image decode: %w", err)
	}
	return img, nil
}
