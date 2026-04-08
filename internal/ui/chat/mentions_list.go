package chat

import (
	"fmt"
	"net/http"

	"github.com/ayn2op/discordo/internal/config"
	httpkg "github.com/ayn2op/discordo/internal/http"
	imgpkg "github.com/ayn2op/discordo/internal/image"
	"github.com/ayn2op/discordo/internal/ui"
	"github.com/eyalmazuz/tview"
	"github.com/eyalmazuz/tview/list"
	"github.com/gdamore/tcell/v3"
)

type mentionsListItem struct {
	insertText  string
	displayText string
	style       tcell.Style
	previewURL  string
}

type mentionsList struct {
	*list.Model
	cfg            *config.Config
	chatView       *Model
	items          []mentionsListItem
	emoteItemByKey map[string]*imageItem
	imageCache     *imgpkg.Cache
	nextKittyID    uint32
	useKitty       bool
	cellW          int
	cellH          int
	pendingDeletes []uint32
}

func newMentionsList(cfg *config.Config, chatView *Model) *mentionsList {
	m := &mentionsList{
		Model:          list.NewModel(),
		cfg:            cfg,
		chatView:       chatView,
		emoteItemByKey: make(map[string]*imageItem),
		imageCache:     imgpkg.NewCache(&http.Client{Transport: httpkg.NewTransport()}),
		nextKittyID:    100000,
		useKitty:       resolveKittyMode(cfg.InlineImages.Renderer),
	}
	m.SetKeybinds(list.Keybinds{
		SelectUp:     cfg.Keybinds.MentionsList.Up.Keybind,
		SelectDown:   cfg.Keybinds.MentionsList.Down.Keybind,
		SelectTop:    cfg.Keybinds.MentionsList.Top.Keybind,
		SelectBottom: cfg.Keybinds.MentionsList.Bottom.Keybind,
	})

	m.Box = ui.ConfigureBox(m.Box, &cfg.Theme)
	m.SetSnapToItems(true).SetTitle("Mentions")

	b := m.GetBorderSet()
	b.BottomLeft, b.BottomRight = b.BottomT, b.BottomT
	m.SetBorderSet(b)

	return m
}

func (m *mentionsList) append(item mentionsListItem) {
	m.items = append(m.items, item)
}

func (m *mentionsList) clear() {
	for _, item := range m.emoteItemByKey {
		if item.kittyPlaced {
			m.pendingDeletes = append(m.pendingDeletes, item.kittyID)
		}
	}
	m.items = nil
	clear(m.emoteItemByKey)
	m.Clear()
}

func (m *mentionsList) rebuild() {
	m.SetBuilder(func(index int, cursor int) list.Item {
		if index < 0 || index >= len(m.items) {
			return nil
		}

		item := m.items[index]
		style := item.style
		if index == cursor {
			style = style.Reverse(true)
		}

		return &mentionsListRowItem{
			Box:      tview.NewBox(),
			item:     item,
			style:    style,
			preview:  m.previewItemFor(index, item),
			useKitty: m.useKitty,
		}
	})

	if len(m.items) == 0 {
		m.SetCursor(-1)
		return
	}
	m.SetCursor(0)
}

func (m *mentionsList) itemCount() int {
	return len(m.items)
}

func (m *mentionsList) selectedInsertText() (string, bool) {
	index := m.Cursor()
	if index < 0 || index >= len(m.items) {
		return "", false
	}
	return m.items[index].insertText, true
}

func (m *mentionsList) maxDisplayWidth() int {
	width := 0
	for _, item := range m.items {
		width = max(width, tview.TaggedStringWidth(item.displayText))
	}
	return width
}

func (m *mentionsList) Draw(screen tcell.Screen) {
	if m.cfg.InlineImages.Enabled && m.useKitty {
		m.updateCellDimensions(screen)
	}
	for _, item := range m.emoteItemByKey {
		item.drawnThisFrame = false
	}
	m.Model.Draw(screen)
}

func (m *mentionsList) AfterDraw(screen tcell.Screen) {
	if !m.cfg.InlineImages.Enabled || !m.useKitty {
		return
	}
	tty, ok := screen.Tty()
	if !ok {
		return
	}

	fmt.Fprint(tty, "\x1b7")
	for _, id := range m.pendingDeletes {
		_ = imgpkg.DeleteKittyByID(tty, id)
	}
	m.pendingDeletes = m.pendingDeletes[:0]

	for key, item := range m.emoteItemByKey {
		if !item.drawnThisFrame {
			if item.kittyPlaced {
				m.pendingDeletes = append(m.pendingDeletes, item.kittyID)
			}
			delete(m.emoteItemByKey, key)
			continue
		}
	}
	for _, id := range m.pendingDeletes {
		_ = imgpkg.DeleteKittyByID(tty, id)
	}
	m.pendingDeletes = m.pendingDeletes[:0]

	for _, item := range m.emoteItemByKey {
		item.flushKittyPlace(tty)
	}
	fmt.Fprint(tty, "\x1b8")
}

func (m *mentionsList) updateCellDimensions(screen tcell.Screen) {
	tty, ok := screen.Tty()
	if !ok || tty == nil {
		return
	}
	ws, err := tty.WindowSize()
	if err != nil {
		return
	}
	cw, ch := ws.CellDimensions()
	if cw <= 0 || ch <= 0 {
		return
	}
	if cw != m.cellW || ch != m.cellH {
		m.cellW = cw
		m.cellH = ch
		for _, item := range m.emoteItemByKey {
			item.setCellDimensions(cw, ch)
		}
	}
}

func (m *mentionsList) previewItemFor(index int, item mentionsListItem) *imageItem {
	if !m.cfg.InlineImages.Enabled {
		return nil
	}
	if item.previewURL == "" {
		return nil
	}
	key := fmt.Sprintf("mention:%s", item.previewURL)
	if existing, ok := m.emoteItemByKey[key]; ok {
		return existing
	}
	kittyID := m.nextKittyID
	m.nextKittyID++
	imgItem := newImageItem(m.imageCache, item.previewURL, inlineEmoteWidth, 1, m.useKitty, kittyID, nil, nil)
	imgItem.lockKittyRegion = false
	if m.cellW > 0 {
		imgItem.setCellDimensions(m.cellW, m.cellH)
	}
	m.emoteItemByKey[key] = imgItem
	m.imageCache.Request(item.previewURL, 0, 0, func() {
		if m.chatView != nil && m.chatView.app != nil {
			triggerRedraw(m.chatView.app)
		}
	})
	return imgItem
}

type mentionsListRowItem struct {
	*tview.Box
	item     mentionsListItem
	style    tcell.Style
	preview  *imageItem
	useKitty bool
}

func (i *mentionsListRowItem) Height(width int) int {
	return 1
}

func (i *mentionsListRowItem) Draw(screen tcell.Screen) {
	x, y, w, h := i.InnerRect()
	if w <= 0 || h <= 0 {
		return
	}

	for col := 0; col < w; col++ {
		screen.SetContent(x+col, y, ' ', nil, i.style)
	}

	textX := x
	if i.preview != nil {
		i.preview.drawnThisFrame = true
		i.preview.SetRect(x, y, inlineEmoteWidth, 1)
		i.preview.Draw(screen)
		if i.useKitty {
			for offset := 1; offset < inlineEmoteWidth && x+offset < x+w; offset++ {
				screen.SetContent(x+offset, y, ' ', nil, i.style)
			}
		}
		textX += inlineEmoteWidth + 1
	}

	col := textX
	for _, r := range i.item.displayText {
		if col >= x+w {
			break
		}
		screen.SetContent(col, y, r, nil, i.style)
		col++
	}
}
