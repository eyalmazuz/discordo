package chat

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/ayn2op/discordo/internal/config"
	imgpkg "github.com/ayn2op/discordo/internal/image"
	"github.com/ayn2op/discordo/internal/markdown"
	"github.com/ayn2op/discordo/internal/ui"
	"github.com/ayn2op/tview"
	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/gdamore/tcell/v3"
)

type mentionsListItem struct {
	insertText  string
	displayText string
	style       tcell.Style
	line        tview.Line
}

type mentionsList struct {
	*tview.List
	cfg          *config.Config
	chatView     *Model
	messagesList *messagesList
	imageCache   *imgpkg.Cache
	items        []mentionsListItem
	lastScreen    tcell.Screen

	emoteItemByKey map[string]*imageItem
	nextKittyID    uint32
	pendingDeletes []uint32
}

func newMentionsList(cfg *config.Config, chatView *Model) *mentionsList {
	m := &mentionsList{
		List:           tview.NewList(),
		cfg:            cfg,
		chatView:       chatView,
		emoteItemByKey: make(map[string]*imageItem),
		nextKittyID:    1,
	}
	if chatView != nil {
		m.messagesList = chatView.messagesList
		if chatView.messagesList != nil {
			m.imageCache = chatView.messagesList.imageCache
		}
	}

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
	m.queueKittyDeletes()
	m.items = nil
	m.Clear()
}

func (m *mentionsList) rebuild() {
	m.SetBuilder(func(index int, cursor int) tview.ListItem {
		if index < 0 || index >= len(m.items) {
			return nil
		}

		line := m.lineForItem(index == cursor, m.items[index])

		return tview.NewTextView().
			SetScrollable(false).
			SetWrap(false).
			SetWordWrap(false).
			SetLines([]tview.Line{line})
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
		line := m.lineForItem(false, item)
		lineWidth := 0
		for _, segment := range line {
			lineWidth += tview.TaggedStringWidth(segment.Text)
		}
		width = max(width, lineWidth)
	}
	return width
}

func (m *mentionsList) appendEmoji(emoji discord.Emoji) {
	m.items = append(m.items, mentionsListItem{
		insertText:  emoji.Name,
		displayText: emoji.Name,
		line:        m.lineForEmoji(emoji),
	})
}

func (m *mentionsList) lineForItem(selected bool, item mentionsListItem) tview.Line {
	line := item.line
	if len(line) == 0 {
		line = tview.NewLine(tview.NewSegment(item.displayText, item.style))
	}

	cloned := make(tview.Line, len(line))
	for i, segment := range line {
		cloned[i] = segment
		if selected {
			cloned[i].Style = cloned[i].Style.Reverse(true)
		}
	}
	return cloned
}

func (m *mentionsList) lineForEmoji(emoji discord.Emoji) tview.Line {
	labelStyle := m.cfg.Theme.MessagesList.MessageStyle.Style
	if !emoji.ID.IsValid() {
		return tview.Line{{Text: emoji.Name, Style: labelStyle}}
	}

	return tview.Line{
		{
			Text:  markdown.CustomEmojiText(emoji.Name, m.cfg.InlineImages.Enabled),
			Style: m.cfg.Theme.MessagesList.EmojiStyle.Style.Url(emoji.EmojiURL()),
		},
		{
			Text:  " " + emoji.Name,
			Style: labelStyle,
		},
	}
}

func (m *mentionsList) useKitty() bool {
	return m.cfg.InlineImages.Enabled && m.messagesList != nil && m.messagesList.useKitty
}

func (m *mentionsList) previewItemByURL(key string, url string) *imageItem {
	if item, ok := m.emoteItemByKey[key]; ok {
		item.useKitty = m.useKitty()
		return item
	}
	if m.imageCache == nil {
		return nil
	}

	queueRedraw := func(_ time.Duration) {}
	if m.messagesList != nil {
		queueRedraw = m.messagesList.scheduleAnimatedRedraw
	}
	item := newImageItem(m.imageCache, url, inlineEmoteWidth, 1, m.useKitty(), m.nextKittyID, m.GetInnerRect, queueRedraw)
	m.nextKittyID++
	m.emoteItemByKey[key] = item
	m.imageCache.Request(url, 0, 0, func() {
		if m.chatView != nil && m.chatView.app != nil {
			m.chatView.app.QueueUpdateDraw(func() {})
		}
	})
	return item
}

func (m *mentionsList) Draw(screen tcell.Screen) {
	m.lastScreen = screen
	if m.useKitty() && m.messagesList != nil {
		m.messagesList.updateCellDimensions(screen)
		m.prepareKittyItemsForFrame(screen)
	}

	m.List.Draw(screen)
	m.scanAndDrawEmotes(screen)
}

func (m *mentionsList) prepareKittyItemsForFrame(screen tcell.Screen) {
	for _, item := range m.emoteItemByKey {
		item.useKitty = true
		item.drawnThisFrame = false
		item.pendingPlace = false
		item.unlockRegion(screen)
		item.kittyPlaced = false
		item.kittyUploaded = false
		if m.messagesList != nil && m.messagesList.cellW > 0 {
			item.setCellDimensions(m.messagesList.cellW, m.messagesList.cellH)
		}
	}
}

func (m *mentionsList) AfterDraw(screen tcell.Screen) {
	if !m.useKitty() && len(m.pendingDeletes) == 0 {
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
	if !m.useKitty() {
		fmt.Fprint(tty, "\x1b8")
		return
	}
	for _, item := range m.emoteItemByKey {
		item.flushKittyPlace(tty)
	}
	fmt.Fprint(tty, "\x1b8")
}

func (m *mentionsList) hasPendingAfterDraw() bool {
	return len(m.pendingDeletes) > 0
}

func (m *mentionsList) queueKittyDeletes() {
	for _, item := range m.emoteItemByKey {
		if item == nil || (!item.kittyPlaced && !item.pendingPlace) {
			continue
		}
		if m.lastScreen != nil {
			item.unlockRegion(m.lastScreen)
		}
		if !slices.Contains(m.pendingDeletes, item.kittyID) {
			m.pendingDeletes = append(m.pendingDeletes, item.kittyID)
		}
		item.pendingPlace = false
		item.invalidateKittyPlacement()
	}
}

func (m *mentionsList) scanAndDrawEmotes(screen tcell.Screen) {
	if !m.cfg.InlineImages.Enabled {
		return
	}

	x, y, w, h := m.GetInnerRect()
	for row := y; row < y+h; row++ {
		for col := x; col < x+w; col++ {
			_, style, _ := screen.Get(col, row)
			_, url := style.GetUrl()
			if !strings.HasPrefix(url, "https://cdn.discordapp.com/emojis/") {
				continue
			}

			item := m.previewItemByURL(url, url)
			if item == nil {
				continue
			}
			item.SetRect(col, row, inlineEmoteWidth, 1)
			item.Draw(screen)

			for offset := 1; offset < inlineEmoteWidth && col+offset < x+w; offset++ {
				screen.SetContent(col+offset, row, ' ', nil, tcell.StyleDefault)
			}
			col += inlineEmoteWidth - 1
		}
	}
}
