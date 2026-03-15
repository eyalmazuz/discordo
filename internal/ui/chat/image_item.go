package chat

import (
	"fmt"
	"image"
	"io"
	"log/slog"
	"time"

	imgpkg "github.com/ayn2op/discordo/internal/image"
	"github.com/eyalmazuz/tview"
	"github.com/gdamore/tcell/v3"
)

// imageItem renders an inline image inside the messages list.
// It implements tview.ListItem.
type imageItem struct {
	*tview.Box
	cache    *imgpkg.Cache
	url      string
	maxW     int
	maxH     int
	useKitty bool

	// Lazy-init cell pixel dimensions (Kitty mode only).
	cellW, cellH int
	initted      bool

	// Pre-rendered half-block lines (cached per draw width).
	renderedLines []tview.Line
	renderedWidth int

	// Cached Kitty encoded payload.
	kittyPayload string
	kittyCols    int
	kittyRows    int

	// Kitty placement tracking.
	kittyID          uint32 // unique Kitty protocol image ID
	lastX, lastY     int    // last drawn screen position
	kittyVisibleRows int    // number of cells displayed vertically in the crop
	kittyCropY       int    // crop start Y (in pixels)
	kittyCropH       int    // crop height (in pixels)
	kittyPlaced      bool   // true if currently placed on screen
	kittyUploaded    bool   // true if the payload was transmitted to memory
	drawnThisFrame   bool   // reset each frame by messagesList

	// pendingPlace is set during Draw to defer the actual TTY write
	// to AfterDraw (after screen.Show completes).
	pendingPlace bool

	lastFrameIndex int
	requestRedraw  func(time.Duration)

	getViewport func() (int, int, int, int)
}

func newImageItem(cache *imgpkg.Cache, url string, maxW, maxH int, useKitty bool, kittyID uint32, getViewport func() (int, int, int, int), requestRedraw func(time.Duration)) *imageItem {
	return &imageItem{
		Box:            tview.NewBox(),
		cache:          cache,
		url:            url,
		maxW:           maxW,
		maxH:           maxH,
		useKitty:       useKitty,
		kittyID:        kittyID,
		lastFrameIndex: -1,
		requestRedraw:  requestRedraw,
		getViewport:    getViewport,
	}
}

func (it *imageItem) invalidateKittyPlacement() {
	it.kittyPlaced = false
	it.kittyUploaded = false
}

// setCellDimensions allows messagesList to push cell dimensions before Height()
// is called, so Height() can compute correct pixel-based sizing.
func (it *imageItem) setCellDimensions(cellW, cellH int) {
	if it.cellW != cellW || it.cellH != cellH {
		it.cellW = cellW
		it.cellH = cellH
		it.initted = cellW > 0 && cellH > 0
		it.kittyPayload = "" // invalidate cached payload
	}
}

// actualCellSize computes the true cols×rows after aspect-ratio-preserving
// resize, mirroring the resizeImage logic in the image package.
func (it *imageItem) actualCellSize(img image.Image, maxCols, maxRows int) (int, int) {
	srcW := img.Bounds().Dx()
	srcH := img.Bounds().Dy()
	if srcW == 0 || srcH == 0 || it.cellW == 0 || it.cellH == 0 {
		return maxCols, maxRows
	}
	targetW := maxCols * it.cellW
	targetH := maxRows * it.cellH
	scaleW := float64(targetW) / float64(srcW)
	scaleH := float64(targetH) / float64(srcH)
	scale := min(scaleW, scaleH)
	newW := max(int(float64(srcW)*scale), 1)
	newH := max(int(float64(srcH)*scale), 1)
	cols := (newW + it.cellW - 1) / it.cellW
	rows := (newH + it.cellH - 1) / it.cellH
	return max(cols, 1), max(rows, 1)
}

// Height reports the height in cells for a given available width.
func (it *imageItem) Height(width int) int {
	img, ok := it.cache.Get(it.url)
	if !ok {
		return 1 // placeholder
	}

	bounds := img.Bounds()
	srcW := bounds.Dx()
	srcH := bounds.Dy()
	if srcW == 0 || srcH == 0 {
		return 1
	}

	displayW := min(width, it.maxW)
	if displayW <= 0 {
		return 1
	}

	if it.useKitty {
		if it.cellW == 0 || it.cellH == 0 {
			return 1 // cell dims not yet known
		}
		pixelW := displayW * it.cellW
		pixelH := pixelW * srcH / srcW
		h := (pixelH + it.cellH - 1) / it.cellH
		return min(max(h, 1), it.maxH)
	}

	// Half-block: 2 pixels per cell vertically.
	pixelH := srcH * displayW / srcW
	cellH := (pixelH + 1) / 2
	return min(max(cellH, 1), it.maxH)
}

func (it *imageItem) Draw(screen tcell.Screen) {
	isEmote := it.maxW <= 2 && it.maxH == 1
	if !isEmote {
		it.Box.DrawForSubclass(screen, it)
	}

	x, y, w, h := it.GetInnerRect()
	if w <= 0 || h <= 0 {
		return
	}

	displayW := min(w, it.maxW)
	displayH := min(h, it.maxH)

	now := time.Now()
	img, frameIndex, nextDelay, animated, ok := it.cache.GetFrame(it.url, now)
	if !ok {
		if it.cache.Failed(it.url) {
			if !isEmote {
				tview.Print(screen, "[image failed]", x, y, w, tview.AlignmentLeft, tcell.ColorRed)
			}
		} else {
			if !isEmote {
				tview.Print(screen, "[loading image...]", x, y, w, tview.AlignmentLeft, tcell.ColorDimGray)
			} else {
				// Small placeholder for emote
				screen.SetContent(x, y, '…', nil, tcell.StyleDefault.Foreground(tcell.ColorDimGray))
			}
		}
		return
	}
	it.setFrame(screen, frameIndex)
	if animated && nextDelay > 0 && it.requestRedraw != nil {
		it.requestRedraw(nextDelay)
	}

	if it.useKitty {
		it.drawKitty(screen, img, x, y, displayW, displayH)
	} else {
		it.drawHalfBlock(screen, img, x, y, displayW, displayH)
	}
}

func (it *imageItem) setFrame(screen tcell.Screen, frameIndex int) {
	if frameIndex < 0 || frameIndex == it.lastFrameIndex {
		return
	}

	if it.kittyPlaced {
		it.unlockRegion(screen)
	}

	it.lastFrameIndex = frameIndex
	it.renderedLines = nil
	it.renderedWidth = 0
	it.kittyPayload = ""
	it.kittyUploaded = false
	it.kittyPlaced = false
	it.pendingPlace = false
}

func (it *imageItem) drawKitty(screen tcell.Screen, img image.Image, x, y, w, h int) {
	isEmote := it.maxW <= 2 && it.maxH == 1
	if !it.initted {
		it.initCellDimensions(screen)
	}
	if it.cellW == 0 || it.cellH == 0 {
		// Fall back to half-block if we can't get cell dimensions.
		it.drawnThisFrame = true
		it.drawHalfBlock(screen, img, x, y, w, h)
		return
	}

	// Determine visual rows based strictly on the caller's provided 'h'.
	// This ensures we don't draw outside the space the list has allocated.
	cols, rows := it.actualCellSize(img, w, h)

	var listY, listH int
	if it.getViewport != nil {
		_, listY, _, listH = it.getViewport()
	} else {
		// Fallback for tests if not provided
		listY = -9999
		listH = 99999
	}

	visibleTop := max(y, listY)
	visibleBottom := min(y+rows, listY+listH)
	visibleRows := visibleBottom - visibleTop

	// Skip placement if completely outside the available list region
	if visibleRows <= 0 {
		return
	}

	// For non-emote images, clear the background to remove ghost text (like "holy").
	if !isEmote {
		for i := 0; i < visibleRows; i++ {
			screen.SetContent(x, visibleTop+i, ' ', nil, tcell.StyleDefault)
		}
	}

	// Calculate how many rows are cut off from the top
	rowsCutTop := visibleTop - y
	cropYPixels := rowsCutTop * it.cellH
	cropHPixels := visibleRows * it.cellH

	it.drawnThisFrame = true

	// If the crop changed or size changed, we need to unlock old region
	if it.kittyPlaced && (it.lastX != x || it.lastY != visibleTop || it.kittyCols != cols || it.kittyVisibleRows != visibleRows || it.kittyCropY != cropYPixels || it.kittyCropH != cropHPixels) {
		screen.LockRegion(it.lastX, it.lastY, it.kittyCols, it.kittyVisibleRows, false)
	}

	// If already placed at the same position and exact crop size, skip re-send.
	if it.kittyPlaced && it.lastX == x && it.lastY == visibleTop && it.kittyCols == cols && it.kittyVisibleRows == visibleRows && it.kittyCropY == cropYPixels && it.kittyCropH == cropHPixels {
		return
	}

	// Cache the expensive resize+PNG+base64 step. This payload is the FULL image.
	// Note: We use it.maxH for the payload size so the full image is stored in terminal memory.
	fullCols, fullRows := it.actualCellSize(img, w, it.maxH)
	if it.kittyPayload == "" || it.kittyCols != fullCols || it.kittyRows != fullRows {
		payload, err := imgpkg.EncodeKittyPayload(img, fullCols, fullRows, it.cellW, it.cellH)
		if err != nil {
			slog.Error("failed to encode kitty payload", "url", it.url, "err", err)
			return
		}
		it.kittyPayload = payload
		it.kittyCols = fullCols
		it.kittyRows = fullRows
		it.kittyUploaded = false
	}

	it.kittyVisibleRows = visibleRows
	it.kittyCropY = cropYPixels
	it.kittyCropH = cropHPixels

	// Lock the visible region so tcell's Show() skips these cells.
	screen.LockRegion(x, visibleTop, fullCols, visibleRows, true)

	// Defer the actual TTY write to AfterDraw (after screen.Show).
	it.lastX = x
	it.lastY = visibleTop
	it.pendingPlace = true
	it.kittyPlaced = true
}

// unlockRegion unlocks the screen region occupied by this image's last placement.
// Must be called before removing/deleting the image so tcell resumes painting those cells.
func (it *imageItem) unlockRegion(screen tcell.Screen) {
	if it.kittyPlaced && it.kittyCols > 0 && it.kittyVisibleRows > 0 {
		screen.LockRegion(it.lastX, it.lastY, it.kittyCols, it.kittyVisibleRows, false)
	}
}

// flushKittyPlace writes the pending Kitty image placement to the TTY.
// Must be called AFTER screen.Show() to avoid corrupting tcell's output.
func (it *imageItem) flushKittyPlace(tty io.Writer) {
	if !it.pendingPlace {
		return
	}
	it.pendingPlace = false
	// Remove stale placement before re-placing (idempotent if ID doesn't exist yet).
	_ = imgpkg.DeleteKittyPlacement(tty, it.kittyID)
	fmt.Fprintf(tty, "\x1b[%d;%dH", it.lastY+1, it.lastX+1)

	if !it.kittyUploaded {
		_ = imgpkg.WriteKittyChunks(tty, it.kittyPayload, it.kittyCols, it.kittyRows, it.kittyID)
		_ = imgpkg.DeleteKittyPlacement(tty, it.kittyID)
		it.kittyUploaded = true
	}

	_ = imgpkg.PlaceKittyCropped(tty, it.kittyID, it.kittyCols, it.kittyVisibleRows, 0, it.kittyCropY, it.kittyCols*it.cellW, it.kittyCropH)
}

func (it *imageItem) drawHalfBlock(screen tcell.Screen, img image.Image, x, y, w, h int) {
	if it.renderedWidth != w || it.renderedLines == nil {
		it.renderedLines = imgpkg.RenderHalfBlock(img, w, h)
		it.renderedWidth = w
	}

	var listY, listH int
	if it.getViewport != nil {
		_, listY, _, listH = it.getViewport()
	} else {
		// Fallback for tests if not provided
		listY = -9999
		listH = 99999
	}

	for row, line := range it.renderedLines {
		if row >= h {
			break
		}
		screenY := y + row
		if screenY < listY || screenY >= listY+listH {
			continue // Skip row as it's outside the viewport bounds
		}

		col := x
		for _, seg := range line {
			for _, r := range seg.Text {
				screen.SetContent(col, screenY, r, nil, seg.Style)
				col++
			}
		}
	}
}

func (it *imageItem) initCellDimensions(screen tcell.Screen) {
	tty, ok := screen.Tty()
	if !ok {
		return
	}
	ws, err := tty.WindowSize()
	if err != nil {
		return
	}
	cw, ch := ws.CellDimensions()
	if cw > 0 && ch > 0 {
		it.cellW = cw
		it.cellH = ch
		it.initted = true
	}
}
