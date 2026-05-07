package chat

import (
	"sync"
	"time"

	"github.com/ayn2op/discordo/internal/image"
	"github.com/ayn2op/tview"
	"github.com/gdamore/tcell/v3"
)

// kittyImage implements tview.Model to render images via Kitty protocol.
type kittyImage struct {
	*tview.Box

	url          string
	cache        *image.Cache
	kittyID      uint32
	cellW, cellH int

	drawnThisFrame bool
	mu             sync.Mutex
}

func newKittyImage(url string, cache *image.Cache, kittyID uint32, cw, ch int) *kittyImage {
	return &kittyImage{
		Box:     tview.NewBox(),
		url:     url,
		cache:   cache,
		kittyID: kittyID,
		cellW:   cw,
		cellH:   ch,
	}
}

func (ki *kittyImage) View(screen tcell.Screen) {
	ki.Box.View(screen)
	_, _, w, h := ki.InnerRect()
	if w <= 0 || h <= 0 {
		return
	}

	ki.mu.Lock()
	ki.drawnThisFrame = true
	ki.mu.Unlock()

	tty, ok := screen.Tty()
	if !ok {
		return
	}

	now := time.Now()
	img, _, _, animated, ok := ki.cache.GetFrame(ki.url, now)
	if !ok {
		return
	}

	if animated {
	}

	err := image.EncodeKitty(tty, img, w, h, ki.cellW, ki.cellH)
	if err == nil {
		_ = image.PlaceKitty(tty, ki.kittyID, w, h)
	}
}
