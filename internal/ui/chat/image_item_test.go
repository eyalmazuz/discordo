package chat

import (
	"bytes"
	"image"
	"net/http"
	"testing"

	imgpkg "github.com/ayn2op/discordo/internal/image"
	"github.com/gdamore/tcell/v3"
)

type mockScreen struct {
	tcell.Screen
	content map[string]rune
}

func (m *mockScreen) SetContent(x int, y int, primary rune, combining []rune, style tcell.Style) {
	if m.content == nil {
		m.content = make(map[string]rune)
	}
	m.content[string(rune(x))+","+string(rune(y))] = primary
}

func (m *mockScreen) LockRegion(x, y, width, height int, lock bool) {}

func (m *mockScreen) Tty() (tcell.Tty, bool) {
	return nil, false
}

func setupMockImageItem(useKitty bool, viewportY, viewportH int) (*mockScreen, *imageItem, image.Image) {
	screen := &mockScreen{}
	cache := imgpkg.NewCache(http.DefaultClient)

	getViewport := func() (int, int, int, int) {
		return 0, viewportY, 100, viewportH
	}

	item := newImageItem(cache, "http://example.com/image.png", 50, 50, useKitty, 1, getViewport)
	item.cellW = 10
	item.cellH = 20
	item.initted = true
	img := image.NewRGBA(image.Rect(0, 0, 100, 100))
	
	return screen, item, img
}

func TestImageItem_Kitty_FullVisibility(t *testing.T) {
	screen, item, img := setupMockImageItem(true, 10, 20)
	
	// y=15, rows=5. Ends at 20. Viewport is 10-30.
	item.drawnThisFrame = false
	item.drawKitty(screen, img, 0, 15, 10, 5) 
	
	if !item.drawnThisFrame {
		t.Errorf("Expected fully visible image to be drawn")
	}
	if item.kittyCropY != 0 {
		t.Errorf("Expected no top crop, got %d", item.kittyCropY)
	}
	if item.kittyVisibleRows != 5 {
		t.Errorf("Expected 5 visible rows, got %d", item.kittyVisibleRows)
	}
}

func TestImageItem_Kitty_ScrollUp_TopCrop(t *testing.T) {
	screen, item, img := setupMockImageItem(true, 10, 20)

	// y=8, rows=5. Ends at 13. Viewport is 10-30. Top 2 rows cut off.
	item.drawKitty(screen, img, 0, 8, 10, 5)
	
	if !item.drawnThisFrame {
		t.Errorf("Expected partially visible top-scrolled image to be drawn")
	}
	if expectedCropY := 2 * item.cellH; item.kittyCropY != expectedCropY {
		t.Errorf("Expected top crop %d, got %d", expectedCropY, item.kittyCropY)
	}
	if item.kittyVisibleRows != 3 {
		t.Errorf("Expected 3 visible rows, got %d", item.kittyVisibleRows)
	}
}

func TestImageItem_Kitty_ScrollDown_BottomCrop(t *testing.T) {
	screen, item, img := setupMockImageItem(true, 10, 20)

	// y=28, rows=5. Ends at 33. Viewport is 10-30. Bottom 3 rows cut off.
	item.drawKitty(screen, img, 0, 28, 10, 5)
	
	if !item.drawnThisFrame {
		t.Errorf("Expected partially visible bottom-scrolled image to be drawn")
	}
	if item.kittyCropY != 0 {
		t.Errorf("Expected no top crop, got %d", item.kittyCropY)
	}
	if item.kittyVisibleRows != 2 {
		t.Errorf("Expected 2 visible rows, got %d", item.kittyVisibleRows)
	}
}

func TestImageItem_Kitty_OutOfBounds(t *testing.T) {
	screen, item, img := setupMockImageItem(true, 10, 20)

	// y=0, rows=5. Ends at 5. Viewport is 10-30. Completely off top.
	item.drawKitty(screen, img, 0, 0, 10, 5)
	if item.drawnThisFrame {
		t.Errorf("Expected completely off-top image to be skipped")
	}

	// y=30, rows=5. Ends at 35. Viewport is 10-30. Completely off bottom.
	item.drawKitty(screen, img, 0, 30, 10, 5)
	if item.drawnThisFrame {
		t.Errorf("Expected completely off-bottom image to be skipped")
	}
}

func TestImageItem_Kitty_StateIntegrity_NoInfiniteUpload(t *testing.T) {
	screen, item, img := setupMockImageItem(true, 10, 20)

	// Frame 1: Full visible
	item.drawKitty(screen, img, 0, 15, 10, 5)
	item.kittyUploaded = true // simulate flush
	
	// Frame 2: Scroll up, 1 row cut off
	item.drawKitty(screen, img, 0, 9, 10, 5)
	
	// Verify it did NOT reset kittyUploaded
	if !item.kittyUploaded {
		t.Errorf("Scrolling should not trigger a re-upload of the payload")
	}
	
	// Verify it kept the original payload row dimensions intact
	if item.kittyRows != 5 {
		t.Errorf("Expected kittyRows to remain 5, got %d", item.kittyRows)
	}
	if item.kittyVisibleRows != 4 {
		t.Errorf("Expected visible rows to be 4, got %d", item.kittyVisibleRows)
	}
}

func TestImageItem_FlushKittyPlace_Output(t *testing.T) {
	_, item, img := setupMockImageItem(true, 10, 20)
	screen := &mockScreen{}
	
	item.drawKitty(screen, img, 0, 8, 10, 5) // y=8 -> visibleTop=10, 3 visible rows
	
	var buf bytes.Buffer
	item.flushKittyPlace(&buf)
	
	out := buf.String()
	
	// Must contain transmit chunk (since it wasn't uploaded yet)
	if !bytes.Contains([]byte(out), []byte("a=t")) {
		t.Errorf("Flush should upload the image using a=t. Got: %s", out)
	}
	
	// Must contain placement chunk for the crop
	// c=10, r=3 (visible rows), x=0, y=40 (2 rows cut off = 40px), w=100 (10*10), h=60 (3*20)
	expectedCrop := "a=p,i=1,c=10,r=3,x=0,y=40,w=100,h=60"
	if !bytes.Contains([]byte(out), []byte(expectedCrop)) {
		t.Errorf("Flush should place crop %s. Got: %s", expectedCrop, out)
	}
}

func TestImageItem_Halfblock_Boundary(t *testing.T) {
	screen, item, img := setupMockImageItem(false, 10, 20)

	item.drawHalfBlock(screen, img, 0, 8, 10, 10)

	if len(screen.content) == 0 {
		t.Errorf("Halfblock should have rendered visible rows")
	}

	for key := range screen.content {
		var y int
		for i, r := range key {
			if r == ',' {
				y = int(key[i+1:][0])
				break
			}
		}
		if y < 10 || y >= 30 {
			t.Errorf("Halfblock rendered outside viewport at y=%d", y)
		}
	}
}
