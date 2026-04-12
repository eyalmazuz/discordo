package chat

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/eyalmazuz/tview/layers"

	"github.com/ayn2op/discordo/internal/clipboard"
	"github.com/ayn2op/discordo/internal/config"
	"github.com/ayn2op/discordo/internal/consts"
	httpkg "github.com/ayn2op/discordo/internal/http"
	imgpkg "github.com/ayn2op/discordo/internal/image"
	"github.com/ayn2op/discordo/internal/markdown"
	"github.com/ayn2op/discordo/internal/ui"
	"github.com/diamondburned/arikawa/v3/api"
	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/diamondburned/arikawa/v3/gateway"
	"github.com/diamondburned/arikawa/v3/state"
	"github.com/diamondburned/arikawa/v3/utils/json/option"
	"github.com/diamondburned/arikawa/v3/utils/ws"
	"github.com/diamondburned/ningen/v3/discordmd"
	"github.com/eyalmazuz/tview"
	"github.com/eyalmazuz/tview/help"
	"github.com/eyalmazuz/tview/keybind"
	"github.com/eyalmazuz/tview/list"
	"github.com/gdamore/tcell/v3"
	"github.com/gdamore/tcell/v3/color"
	"github.com/rivo/uniseg"
	"github.com/skratchdot/open-golang/open"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
)

var openStart = open.Start

var (
	httpGetAttachment    = http.Get
	mkdirAllAttachment   = os.MkdirAll
	createAttachmentFile = os.Create
	copyAttachmentData   = io.Copy
	deleteMessageFunc    = func(s *state.State, channelID discord.ChannelID, messageID discord.MessageID, reason api.AuditLogReason) error {
		return s.DeleteMessage(channelID, messageID, reason)
	}
	pinMessageFunc = func(s *state.State, channelID discord.ChannelID, messageID discord.MessageID, reason api.AuditLogReason) error {
		return s.PinMessage(channelID, messageID, reason)
	}
	unpinMessageFunc = func(s *state.State, channelID discord.ChannelID, messageID discord.MessageID, reason api.AuditLogReason) error {
		return s.UnpinMessage(channelID, messageID, reason)
	}
	messageRemoveFunc = func(s *state.State, channelID discord.ChannelID, messageID discord.MessageID) error {
		return s.MessageRemove(channelID, messageID)
	}
	sendGatewayFunc = func(s *state.State, ctx context.Context, cmd ws.Event) error {
		return s.SendGateway(ctx, cmd)
	}
)

type messagesList struct {
	*list.Model
	cfg      *config.Config
	chat     *Model
	messages []discord.Message
	// rows is the virtual list model rendered by tview (message rows +
	// date-separator rows + image rows). It is rebuilt lazily when rowsDirty is true.
	rows      []messagesListRow
	rowsDirty bool

	renderer *markdown.Renderer
	// itemByID caches unselected message TextViews.
	itemByID map[discord.MessageID]*tview.TextView
	// imageItemByKey caches image items to avoid expensive recomputation on every draw. Key is messageID-attachmentIndex.
	imageItemByKey map[string]*imageItem
	// emoteItemByKey caches emoji items.
	emoteItemByKey map[string]*imageItem
	// stickerItemByKey caches sticker items.
	stickerItemByKey map[string]*imageItem

	attachmentsPicker *attachmentsPicker
	reactionPicker    *reactionPicker

	imageCache *imgpkg.Cache
	useKitty   bool

	nextKittyID         uint32
	kittyNeedsFullClear bool
	kittySuspended      bool
	cellW, cellH        int      // cached cell pixel dimensions for Kitty mode
	pendingDeletes      []uint32 // kitty IDs to delete in AfterDraw

	fetchingOlder bool

	fetchingMembers struct {
		mu    sync.Mutex
		value bool
		count uint
		done  chan struct{}
	}

	lastScreen tcell.Screen

	animationMu    sync.Mutex
	animationTimer *time.Timer
	animationDue   time.Time
	queueDraw      func()
}

var _ help.KeyMap = (*messagesList)(nil)

type messagesListRowKind uint8

const (
	messagesListRowMessage messagesListRowKind = iota
	messagesListRowSeparator
	messagesListRowImage
	messagesListRowSticker
	messagesListRowEmbedImage
)

type messagesListRow struct {
	kind            messagesListRowKind
	messageIndex    int
	snapshotIndex   int
	attachmentIndex int
	stickerIndex    int
	embedIndex      int
	isThumbnail     bool
	timestamp       discord.Timestamp
}

const inlineEmoteWidth = 2

func newMessagesList(cfg *config.Config, chat *Model) *messagesList {
	useKitty := resolveKittyMode(cfg.InlineImages.Renderer)
	ml := &messagesList{
		Model:            list.NewModel(),
		cfg:              cfg,
		chat:             chat,
		renderer:         markdown.NewRenderer(cfg),
		itemByID:         make(map[discord.MessageID]*tview.TextView),
		imageItemByKey:   make(map[string]*imageItem),
		emoteItemByKey:   make(map[string]*imageItem),
		stickerItemByKey: make(map[string]*imageItem),
		imageCache:       imgpkg.NewCache(&http.Client{Transport: httpkg.NewTransport()}),
		useKitty:         useKitty,
		nextKittyID:      1,
	}
	ml.attachmentsPicker = newAttachmentsPicker(cfg, chat)
	ml.reactionPicker = newReactionPicker(cfg, chat, ml)

	ml.Box = ui.ConfigureBox(ml.Box, &cfg.Theme)
	ml.SetTitle("Messages")
	ml.SetBuilder(ml.buildItem)
	ml.SetChangedFunc(ml.onRowCursorChanged)
	ml.SetTrackEnd(true)
	ml.SetAlignBottom(true)
	ml.SetKeybinds(list.Keybinds{
		ScrollUp:     cfg.Keybinds.MessagesList.ScrollUp.Keybind,
		ScrollDown:   cfg.Keybinds.MessagesList.ScrollDown.Keybind,
		ScrollTop:    cfg.Keybinds.MessagesList.ScrollTop.Keybind,
		ScrollBottom: cfg.Keybinds.MessagesList.ScrollBottom.Keybind,
	})
	ml.SetScrollBarVisibility(cfg.Theme.ScrollBar.Visibility.ScrollBarVisibility)
	ml.SetScrollBar(tview.NewScrollBar().
		SetTrackStyle(cfg.Theme.ScrollBar.TrackStyle.Style).
		SetThumbStyle(cfg.Theme.ScrollBar.ThumbStyle.Style).
		SetGlyphSet(cfg.Theme.ScrollBar.GlyphSet.GlyphSet))
	return ml
}

func (ml *messagesList) reset() {
	ml.stopAnimatedRedraw()
	ml.fetchingOlder = false
	ml.messages = nil
	ml.rows = nil
	ml.rowsDirty = false
	clear(ml.itemByID)
	ml.kittyNeedsFullClear = true
	if ml.chat != nil && ml.chat.HasLayer(reactionPickerLayerName) {
		ml.chat.RemoveLayer(reactionPickerLayerName)
	}
	ml.
		Clear().
		SetBuilder(ml.buildItem).
		SetTitle("")
}

func (ml *messagesList) Draw(screen tcell.Screen) {
	ml.lastScreen = screen
	overlayVisible := ml.chat != nil && ml.chat.hasPopupOverlay()
	if ml.cfg.InlineImages.Enabled && ml.useKitty {
		ml.setKittySuspended(screen, overlayVisible)
		if !ml.kittySuspended {
			ml.updateCellDimensions(screen)
			// Full clear only on channel switch / reset.
			if ml.kittyNeedsFullClear {
				ml.kittyNeedsFullClear = false
				for _, item := range ml.imageItemByKey {
					item.unlockRegion(screen)
					item.invalidateKittyPlacement()
					if item.kittyID > 0 {
						ml.pendingDeletes = append(ml.pendingDeletes, item.kittyID)
					}
				}
				for _, item := range ml.emoteItemByKey {
					item.unlockRegion(screen)
					item.invalidateKittyPlacement()
					if item.kittyID > 0 {
						ml.pendingDeletes = append(ml.pendingDeletes, item.kittyID)
					}
				}
				for _, item := range ml.stickerItemByKey {
					item.unlockRegion(screen)
					item.invalidateKittyPlacement()
					if item.kittyID > 0 {
						ml.pendingDeletes = append(ml.pendingDeletes, item.kittyID)
					}
				}
				clear(ml.imageItemByKey)
				clear(ml.emoteItemByKey)
				clear(ml.stickerItemByKey)
				ml.nextKittyID = 1
			}
			// Reset per-frame tracking and propagate cell dimensions.
			for _, item := range ml.imageItemByKey {
				item.drawnThisFrame = false
				item.setCellDimensions(ml.cellW, ml.cellH)
			}
			for _, item := range ml.emoteItemByKey {
				item.drawnThisFrame = false
				item.setCellDimensions(ml.cellW, ml.cellH)
			}
			for _, item := range ml.stickerItemByKey {
				item.drawnThisFrame = false
				item.setCellDimensions(ml.cellW, ml.cellH)
			}
		}
	}

	// Clear the background to prevent text overlapping when scrolling large distances.
	// We use spaces instead of screen.Clear() to avoid flickering and erasing Kitty images.
	x, y, width, height := ml.InnerRect()
	style := tcell.StyleDefault
	if ml.cfg != nil {
		style = ml.cfg.Theme.MessagesList.MessageStyle.Style
	}
	for row := 0; row < height; row++ {
		for col := 0; col < width; col++ {
			screen.SetContent(x+col, y+row, ' ', nil, style)
		}
	}

	ml.Model.Draw(screen)

	ml.scanAndDrawEmotes(screen)

	// Collect off-screen images for deletion in AfterDraw.
	if ml.cfg.InlineImages.Enabled && ml.useKitty {
		for _, item := range ml.imageItemByKey {
			if !item.drawnThisFrame && item.kittyPlaced {
				item.unlockRegion(screen)
				ml.pendingDeletes = append(ml.pendingDeletes, item.kittyID)
				item.invalidateKittyPlacement()
			}
		}
		for _, item := range ml.emoteItemByKey {
			if !item.drawnThisFrame && item.kittyPlaced {
				item.unlockRegion(screen)
				ml.pendingDeletes = append(ml.pendingDeletes, item.kittyID)
				item.invalidateKittyPlacement()
			}
		}
		for _, item := range ml.stickerItemByKey {
			if !item.drawnThisFrame && item.kittyPlaced {
				item.unlockRegion(screen)
				ml.pendingDeletes = append(ml.pendingDeletes, item.kittyID)
				item.invalidateKittyPlacement()
			}
		}
	}
}

func (ml *messagesList) scanAndDrawEmotes(screen tcell.Screen) {
	if !ml.cfg.InlineImages.Enabled {
		return
	}

	x, y, w, h := ml.InnerRect()
	for i := y; i < y+h; i++ {
		for j := x; j < x+w; j++ {
			_, style, _ := screen.Get(j, i)
			_, url := style.GetUrl()
			if !strings.HasPrefix(url, "https://cdn.discordapp.com/emojis/") && !strings.Contains(url, "twemoji") {
				continue
			}

			// Key includes coordinates so multiple instances of the same emoji don't collide.
			key := fmt.Sprintf("%s@%d,%d", url, j, i)
			item, ok := ml.emoteItemByKey[key]
			if !ok {
				item = newImageItem(ml.imageCache, url, inlineEmoteWidth, 1, ml.currentUseKitty(), ml.nextKittyID, ml.InnerRect, ml.scheduleAnimatedRedraw)
				ml.nextKittyID++
				if ml.currentUseKitty() && ml.cellW > 0 {
					item.setCellDimensions(ml.cellW, ml.cellH)
				}
				ml.emoteItemByKey[key] = item

				// Trigger async download so the emote image actually loads.
				ml.imageCache.Request(url, 0, 0, func() {
					if ml.chat != nil && ml.chat.app != nil {
						triggerRedraw(ml.chat.app)
					}
				})
			}

			// SetRect is needed for GetInnerRect used inside imageItem.Draw
			item.SetRect(j, i, inlineEmoteWidth, 1)
			item.Draw(screen)

			// Custom emoji placeholders always occupy a fixed 2-cell slot. Stepping
			// by width instead of collapsing the full URL run preserves adjacent
			// identical emoji as separate occurrences.
			if ml.useKitty {
				// Continuation cell for wide characters might not need manual clearing
				// if LockRegion covers it, but we do it for safety.
				for offset := 1; offset < inlineEmoteWidth && j+offset < x+w; offset++ {
					screen.SetContent(j+offset, i, ' ', nil, tcell.StyleDefault)
				}
			}
			j += inlineEmoteWidth - 1
		}
	}
}

// AfterDraw writes all pending Kitty protocol commands to the TTY.
// Must be called AFTER screen.Show() to avoid corrupting tcell's output.
func (ml *messagesList) AfterDraw(screen tcell.Screen) {
	if !ml.cfg.InlineImages.Enabled || !ml.useKitty {
		return
	}
	tty, ok := screen.Tty()
	if !ok {
		return
	}

	// Save cursor position so we restore it after our TTY writes,
	// keeping tcell's cursor tracking in sync.
	fmt.Fprint(tty, "\x1b7")

	if ml.kittySuspended {
		// Only delete images owned by this component so overlays (e.g.
		// mentionsList emote previews) keep their Kitty images.
		for _, id := range ml.pendingDeletes {
			_ = imgpkg.DeleteKittyByID(tty, id)
		}
		ml.pendingDeletes = ml.pendingDeletes[:0]
		fmt.Fprint(tty, "\x1b8")
		return
	}

	// Delete off-screen images.
	for _, id := range ml.pendingDeletes {
		_ = imgpkg.DeleteKittyByID(tty, id)
	}
	ml.pendingDeletes = ml.pendingDeletes[:0]

	// Place on-screen images.
	for _, item := range ml.imageItemByKey {
		item.flushKittyPlace(tty)
	}
	for _, item := range ml.emoteItemByKey {
		item.flushKittyPlace(tty)
	}
	for _, item := range ml.stickerItemByKey {
		item.flushKittyPlace(tty)
	}

	// Restore cursor position.
	fmt.Fprint(tty, "\x1b8")
}

func (ml *messagesList) currentUseKitty() bool {
	return ml.useKitty && !ml.kittySuspended
}

func (ml *messagesList) setKittySuspended(screen tcell.Screen, suspended bool) {
	if !ml.useKitty {
		return
	}

	wasSuspended := ml.kittySuspended
	ml.kittySuspended = suspended
	useKitty := ml.currentUseKitty()

	// Only queue deletes on the transition into suspended state, not every frame.
	needsCleanup := suspended && !wasSuspended

	for _, item := range ml.imageItemByKey {
		item.useKitty = useKitty
		if needsCleanup {
			item.pendingPlace = false
			item.unlockRegion(screen)
			if item.kittyPlaced {
				ml.pendingDeletes = append(ml.pendingDeletes, item.kittyID)
			}
			item.invalidateKittyPlacement()
		}
	}
	for _, item := range ml.emoteItemByKey {
		item.useKitty = useKitty
		if needsCleanup {
			item.pendingPlace = false
			item.unlockRegion(screen)
			if item.kittyPlaced {
				ml.pendingDeletes = append(ml.pendingDeletes, item.kittyID)
			}
			item.invalidateKittyPlacement()
		}
	}
	for _, item := range ml.stickerItemByKey {
		item.useKitty = useKitty
		if needsCleanup {
			item.pendingPlace = false
			item.unlockRegion(screen)
			if item.kittyPlaced {
				ml.pendingDeletes = append(ml.pendingDeletes, item.kittyID)
			}
			item.invalidateKittyPlacement()
		}
	}
}

func (ml *messagesList) updateCellDimensions(screen tcell.Screen) {
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
	if cw != ml.cellW || ch != ml.cellH {
		ml.cellW = cw
		ml.cellH = ch
		// Cell dimensions changed (e.g. font size change) — invalidate all cached payloads.
		for _, item := range ml.imageItemByKey {
			item.kittyPayload = ""
		}
	}
}

func (ml *messagesList) scheduleAnimatedRedraw(after time.Duration) {
	if ml == nil || !ml.canQueueDraw() {
		return
	}
	if after <= 0 {
		after = 100 * time.Millisecond
	}
	if after < 20*time.Millisecond {
		after = 20 * time.Millisecond
	}

	due := time.Now().Add(after)
	ml.animationMu.Lock()
	if ml.animationTimer != nil && !due.Before(ml.animationDue) {
		ml.animationMu.Unlock()
		return
	}
	if ml.animationTimer != nil {
		ml.animationTimer.Stop()
	}

	var timer *time.Timer
	timer = time.AfterFunc(after, func() {
		ml.animationMu.Lock()
		if ml.animationTimer == timer {
			ml.animationTimer = nil
			ml.animationDue = time.Time{}
		}
		ml.animationMu.Unlock()
		ml.queueAnimatedDraw()
	})
	ml.animationTimer = timer
	ml.animationDue = due
	ml.animationMu.Unlock()
}

func (ml *messagesList) stopAnimatedRedraw() {
	ml.animationMu.Lock()
	if ml.animationTimer != nil {
		ml.animationTimer.Stop()
		ml.animationTimer = nil
	}
	ml.animationDue = time.Time{}
	ml.animationMu.Unlock()
}

func (ml *messagesList) canQueueDraw() bool {
	return ml != nil && (ml.queueDraw != nil || (ml.chat != nil && ml.chat.app != nil))
}

func (ml *messagesList) queueAnimatedDraw() {
	if ml == nil {
		return
	}
	if ml.queueDraw != nil {
		ml.queueDraw()
		return
	}
	if ml.chat == nil || ml.chat.app == nil {
		return
	}
	triggerRedraw(ml.chat.app)
}

func resolveKittyMode(renderer string) bool {
	switch renderer {
	case "kitty":
		return true
	case "halfblock":
		return false
	default: // "auto" or empty
		return imgpkg.IsKittySupported()
	}
}

func (ml *messagesList) setTitle(channel discord.Channel) {
	title := ui.ChannelToString(channel, ml.cfg.Icons, ml.chat.state)
	if topic := channel.Topic; topic != "" {
		title += " - " + topic
	}

	ml.SetTitle(title)
}

func (ml *messagesList) setMessages(messages []discord.Message) {
	ml.stopAnimatedRedraw()
	ml.messages = slices.Clone(messages)
	slices.Reverse(ml.messages)
	ml.invalidateRows()
	// New channel payload / refetch: replace the cache wholesale to keep it in
	// lockstep with the current message slice.
	clear(ml.itemByID)
	ml.kittyNeedsFullClear = true
}

func (ml *messagesList) addMessage(message discord.Message) {
	ml.messages = append(ml.messages, message)
	ml.invalidateRows()
	// Defensive invalidation for ID reuse/edits delivered out-of-order.
	delete(ml.itemByID, message.ID)
}

func (ml *messagesList) setMessage(index int, message discord.Message) {
	if index < 0 || index >= len(ml.messages) {
		return
	}

	ml.messages[index] = message
	delete(ml.itemByID, message.ID)
	ml.invalidateRows()
}

func (ml *messagesList) deleteMessage(index int) {
	if index < 0 || index >= len(ml.messages) {
		return
	}

	delete(ml.itemByID, ml.messages[index].ID)
	ml.messages = append(ml.messages[:index], ml.messages[index+1:]...)
	ml.invalidateRows()
}

func (ml *messagesList) clearSelection() {
	ml.SetCursor(-1)
}

func (ml *messagesList) buildItem(index int, cursor int) list.Item {
	ml.ensureRows()

	if index < 0 || index >= len(ml.rows) {
		return nil
	}

	row := ml.rows[index]
	if row.kind == messagesListRowSeparator {
		return ml.buildSeparatorItem(row.timestamp)
	}

	if row.kind == messagesListRowImage {
		return ml.buildImageItem(row)
	}

	if row.kind == messagesListRowSticker {
		return ml.buildStickerItem(row)
	}

	if row.kind == messagesListRowEmbedImage {
		return ml.buildEmbedImageItem(row)
	}

	message := ml.messages[row.messageIndex]
	if index == cursor {
		return tview.NewTextView().
			SetWrap(true).
			SetWordWrap(true).
			SetLines(ml.renderMessage(message, ml.cfg.Theme.MessagesList.SelectedMessageStyle.Style, false))
	}

	item, ok := ml.itemByID[message.ID]
	if !ok {
		item = tview.NewTextView().
			SetWrap(true).
			SetWordWrap(true).
			SetLines(ml.renderMessage(message, ml.cfg.Theme.MessagesList.MessageStyle.Style, true))
		ml.itemByID[message.ID] = item
	}
	return item
}

func (ml *messagesList) renderMessage(message discord.Message, baseStyle tcell.Style, hideSpoilers bool) []tview.Line {
	builder := tview.NewLineBuilder()
	ml.writeMessage(builder, message, baseStyle, hideSpoilers)
	return builder.Finish()
}

func (ml *messagesList) buildSeparatorItem(ts discord.Timestamp) *tview.TextView {
	builder := tview.NewLineBuilder()
	ml.drawDateSeparator(builder, ts, ml.cfg.Theme.MessagesList.MessageStyle.Style)
	return tview.NewTextView().
		SetScrollable(false).
		SetWrap(false).
		SetWordWrap(false).
		SetLines(builder.Finish())
}

func (ml *messagesList) buildImageItem(row messagesListRow) *imageItem {
	msg := ml.messages[row.messageIndex]
	var a discord.Attachment
	var key string
	if len(msg.MessageSnapshots) > 0 {
		a = msg.MessageSnapshots[row.snapshotIndex].Message.Attachments[row.attachmentIndex]
		key = fmt.Sprintf("%s-snapshot-%d-%d", msg.ID, row.snapshotIndex, row.attachmentIndex)
	} else {
		a = msg.Attachments[row.attachmentIndex]
		key = fmt.Sprintf("%s-%d", msg.ID, row.attachmentIndex)
	}

	url := string(a.URL)

	if item, ok := ml.imageItemByKey[key]; ok {
		return item
	}

	cfg := ml.cfg.InlineImages
	kittyID := ml.nextKittyID
	ml.nextKittyID++

	item := newImageItem(ml.imageCache, url, cfg.MaxWidth, cfg.MaxHeight, ml.currentUseKitty(), kittyID, ml.InnerRect, ml.scheduleAnimatedRedraw)
	if ml.currentUseKitty() && ml.cellW > 0 {
		item.setCellDimensions(ml.cellW, ml.cellH)
	}
	ml.imageItemByKey[key] = item

	// Request async download if not already cached.
	ml.imageCache.Request(url, cfg.MaxFileSize, a.Size, func() {
		triggerRedraw(ml.chat.app)
	})

	return item
}

func (ml *messagesList) buildEmbedImageItem(row messagesListRow) *imageItem {
	msg := ml.messages[row.messageIndex]
	var e discord.Embed
	var key string
	if len(msg.MessageSnapshots) > 0 {
		e = msg.MessageSnapshots[row.snapshotIndex].Message.Embeds[row.embedIndex]
		if row.isThumbnail {
			key = fmt.Sprintf("%s-snapshot-%d-embed-%d-thumbnail", msg.ID, row.snapshotIndex, row.embedIndex)
		} else {
			key = fmt.Sprintf("%s-snapshot-%d-embed-%d-image", msg.ID, row.snapshotIndex, row.embedIndex)
		}
	} else {
		e = msg.Embeds[row.embedIndex]
		if row.isThumbnail {
			key = fmt.Sprintf("%s-embed-%d-thumbnail", msg.ID, row.embedIndex)
		} else {
			key = fmt.Sprintf("%s-embed-%d-image", msg.ID, row.embedIndex)
		}
	}

	var url string
	if row.isThumbnail {
		url = string(e.Thumbnail.URL)
	} else {
		url = string(e.Image.URL)
	}

	if item, ok := ml.imageItemByKey[key]; ok {
		return item
	}

	cfg := ml.cfg.InlineImages
	kittyID := ml.nextKittyID
	ml.nextKittyID++

	item := newImageItem(ml.imageCache, url, cfg.MaxWidth, cfg.MaxHeight, ml.currentUseKitty(), kittyID, ml.InnerRect, ml.scheduleAnimatedRedraw)
	if ml.currentUseKitty() && ml.cellW > 0 {
		item.setCellDimensions(ml.cellW, ml.cellH)
	}
	ml.imageItemByKey[key] = item

	// Request async download if not already cached.
	ml.imageCache.Request(url, cfg.MaxFileSize, 0, func() {
		triggerRedraw(ml.chat.app)
	})

	return item
}

func (ml *messagesList) buildStickerItem(row messagesListRow) *imageItem {
	msg := ml.messages[row.messageIndex]
	var s discord.StickerItem
	var key string
	if len(msg.MessageSnapshots) > 0 {
		s = msg.MessageSnapshots[row.snapshotIndex].Message.Stickers[row.stickerIndex]
		key = fmt.Sprintf("%s-snapshot-%d-sticker-%d", msg.ID, row.snapshotIndex, row.stickerIndex)
	} else {
		s = msg.Stickers[row.stickerIndex]
		key = fmt.Sprintf("%s-%d", msg.ID, row.stickerIndex)
	}

	url := ui.StickerURL(s)

	if item, ok := ml.stickerItemByKey[key]; ok {
		return item
	}

	cfg := ml.cfg.InlineImages
	kittyID := ml.nextKittyID
	ml.nextKittyID++

	// Stickers are usually 320x320. We scale them to 40% of the configured inline image size.
	maxW := int(float64(cfg.MaxWidth) * 0.4)
	maxH := int(float64(cfg.MaxHeight) * 0.4)
	item := newImageItem(ml.imageCache, url, maxW, maxH, ml.currentUseKitty(), kittyID, ml.InnerRect, ml.scheduleAnimatedRedraw)
	if ml.currentUseKitty() && ml.cellW > 0 {
		item.setCellDimensions(ml.cellW, ml.cellH)
	}
	ml.stickerItemByKey[key] = item

	// Stickers don't have a size field in StickerItem, so we use 0 (unlimited for now or we can pick a sensible default).
	ml.imageCache.Request(url, cfg.MaxFileSize, 0, func() {
		triggerRedraw(ml.chat.app)
	})

	return item
}

func (ml *messagesList) drawDateSeparator(builder *tview.LineBuilder, ts discord.Timestamp, baseStyle tcell.Style) {
	date := ts.Time().In(time.Local).Format(ml.cfg.DateSeparator.Format)
	label := " " + date + " "
	fillChar := ml.cfg.DateSeparator.Character
	dimStyle := baseStyle.Dim(true)
	_, _, width, _ := ml.InnerRect()
	if width <= 0 {
		builder.Write(strings.Repeat(fillChar, 8)+label+strings.Repeat(fillChar, 8), dimStyle)
		return
	}

	labelWidth := utf8.RuneCountInString(label)
	if width <= labelWidth {
		builder.Write(date, dimStyle)
		return
	}

	fillWidth := width - labelWidth
	left := fillWidth / 2
	right := fillWidth - left
	builder.Write(strings.Repeat(fillChar, left)+label+strings.Repeat(fillChar, right), dimStyle)
}

func (ml *messagesList) rebuildRows() {
	rows := make([]messagesListRow, 0, len(ml.messages)*2)

	for i := range ml.messages {
		// Always show a date separator before the first message, and between messages on different days.
		if ml.cfg.DateSeparator.Enabled && (i == 0 || !sameLocalDate(ml.messages[i-1].Timestamp, ml.messages[i].Timestamp)) {
			rows = append(rows, messagesListRow{
				kind:      messagesListRowSeparator,
				timestamp: ml.messages[i].Timestamp,
			})
		}

		rows = append(rows, messagesListRow{
			kind:         messagesListRowMessage,
			messageIndex: i,
		})

		if ml.cfg.InlineImages.Enabled {
			seenURLs := make(map[string]struct{})
			for j, a := range ml.messages[i].Attachments {
				if strings.HasPrefix(a.ContentType, "image/") {
					seenURLs[string(a.URL)] = struct{}{}
					rows = append(rows, messagesListRow{
						kind:            messagesListRowImage,
						messageIndex:    i,
						attachmentIndex: j,
					})
				}
			}

			for j := range ml.messages[i].Stickers {
				rows = append(rows, messagesListRow{
					kind:         messagesListRowSticker,
					messageIndex: i,
					stickerIndex: j,
				})
			}

			for j, e := range ml.messages[i].Embeds {
				if ml.cfg.InlineImages.EmbedImages && e.Image != nil && e.Image.URL != "" {
					u := string(e.Image.URL)
					if _, ok := seenURLs[u]; !ok && !strings.HasPrefix(u, "https://cdn.discordapp.com/emojis/") {
						seenURLs[u] = struct{}{}
						rows = append(rows, messagesListRow{
							kind:         messagesListRowEmbedImage,
							messageIndex: i,
							embedIndex:   j,
							isThumbnail:  false,
						})
					}
				}
				if ml.cfg.InlineImages.EmbedThumbnails && e.Thumbnail != nil && e.Thumbnail.URL != "" {
					u := string(e.Thumbnail.URL)
					if _, ok := seenURLs[u]; !ok {
						seenURLs[u] = struct{}{}
						rows = append(rows, messagesListRow{
							kind:         messagesListRowEmbedImage,
							messageIndex: i,
							embedIndex:   j,
							isThumbnail:  true,
						})
					}
				}
			}

			for j, s := range ml.messages[i].MessageSnapshots {
				for k, a := range s.Message.Attachments {
					if strings.HasPrefix(a.ContentType, "image/") {
						seenURLs[string(a.URL)] = struct{}{}
						rows = append(rows, messagesListRow{
							kind:            messagesListRowImage,
							messageIndex:    i,
							snapshotIndex:   j,
							attachmentIndex: k,
						})
					}
				}

				for k := range s.Message.Stickers {
					rows = append(rows, messagesListRow{
						kind:          messagesListRowSticker,
						messageIndex:  i,
						snapshotIndex: j,
						stickerIndex:  k,
					})
				}

				for k, e := range s.Message.Embeds {
					if ml.cfg.InlineImages.EmbedImages && e.Image != nil && e.Image.URL != "" {
						u := string(e.Image.URL)
						if _, ok := seenURLs[u]; !ok && !strings.HasPrefix(u, "https://cdn.discordapp.com/emojis/") {
							seenURLs[u] = struct{}{}
							rows = append(rows, messagesListRow{
								kind:          messagesListRowEmbedImage,
								messageIndex:  i,
								snapshotIndex: j,
								embedIndex:    k,
								isThumbnail:   false,
							})
						}
					}
					if ml.cfg.InlineImages.EmbedThumbnails && e.Thumbnail != nil && e.Thumbnail.URL != "" {
						u := string(e.Thumbnail.URL)
						if _, ok := seenURLs[u]; !ok {
							seenURLs[u] = struct{}{}
							rows = append(rows, messagesListRow{
								kind:          messagesListRowEmbedImage,
								messageIndex:  i,
								snapshotIndex: j,
								embedIndex:    k,
								isThumbnail:   true,
							})
						}
					}
				}
			}
		}
	}

	ml.rows = rows
	ml.rowsDirty = false
}

func (ml *messagesList) invalidateRows() {
	ml.rowsDirty = true
}

// ensureRows lazily rebuilds list rows. This avoids repeated O(n) row rebuild
// work when multiple message mutations happen close together.
func (ml *messagesList) ensureRows() {
	if !ml.rowsDirty {
		return
	}

	ml.rebuildRows()
}

func sameLocalDate(a discord.Timestamp, b discord.Timestamp) bool {
	ta := a.Time().In(time.Local)
	tb := b.Time().In(time.Local)
	return ta.Year() == tb.Year() && ta.YearDay() == tb.YearDay()
}

// Cursor returns the selected message index, skipping separator rows.
func (ml *messagesList) Cursor() int {
	ml.ensureRows()
	rowIndex := ml.Model.Cursor()
	if rowIndex < 0 || rowIndex >= len(ml.rows) {
		return -1
	}

	row := ml.rows[rowIndex]
	if row.kind == messagesListRowSeparator {
		return -1
	}
	return row.messageIndex
}

// SetCursor selects a message index and maps it to the corresponding row.
func (ml *messagesList) SetCursor(index int) {
	ml.Model.SetCursor(ml.messageToRowIndex(index))
}

func (ml *messagesList) messageToRowIndex(messageIndex int) int {
	ml.ensureRows()
	if messageIndex < 0 || messageIndex >= len(ml.messages) {
		return -1
	}

	for i, row := range ml.rows {
		if row.kind == messagesListRowMessage && row.messageIndex == messageIndex {
			return i
		}
	}

	return -1
}

func (ml *messagesList) onRowCursorChanged(rowIndex int) {
	ml.ensureRows()
	if rowIndex < 0 || rowIndex >= len(ml.rows) || ml.rows[rowIndex].kind != messagesListRowSeparator {
		return
	}

	target := ml.nearestMessageRowIndex(rowIndex)
	ml.Model.SetCursor(target)
}

// nearestMessageRowIndex expects rowIndex to be within bounds.
func (ml *messagesList) nearestMessageRowIndex(rowIndex int) int {
	for i := rowIndex - 1; i >= 0; i-- {
		if ml.rows[i].kind == messagesListRowMessage {
			return i
		}
	}
	for i := rowIndex + 1; i < len(ml.rows); i++ {
		if ml.rows[i].kind == messagesListRowMessage {
			return i
		}
	}
	return -1
}

func (ml *messagesList) writeMessage(builder *tview.LineBuilder, message discord.Message, baseStyle tcell.Style, hideSpoilers bool) {
	if ml.cfg.HideBlockedUsers {
		isBlocked := ml.chat.state.UserIsBlocked(message.Author.ID)
		if isBlocked {
			builder.Write("Blocked message", baseStyle.Foreground(color.Red).Bold(true))
			return
		}
	}

	switch message.Type {
	case discord.DefaultMessage:
		if message.Reference != nil && message.Reference.Type == discord.MessageReferenceTypeForward {
			ml.drawForwardedMessage(builder, message, baseStyle, hideSpoilers)
		} else {
			ml.drawDefaultMessage(builder, message, baseStyle, hideSpoilers)
		}
	case discord.GuildMemberJoinMessage:
		ml.drawTimestamps(builder, message.Timestamp, baseStyle)
		ml.drawAuthor(builder, message, baseStyle)
		builder.Write("joined the server.", baseStyle)
	case discord.InlinedReplyMessage:
		ml.drawReplyMessage(builder, message, baseStyle, hideSpoilers)
	case discord.ChannelPinnedMessage:
		ml.drawPinnedMessage(builder, message, baseStyle, hideSpoilers)
	default:
		ml.drawTimestamps(builder, message.Timestamp, baseStyle)
		ml.drawAuthor(builder, message, baseStyle)
	}
}

func (ml *messagesList) formatTimestamp(ts discord.Timestamp) string {
	return ts.Time().In(time.Local).Format(ml.cfg.Timestamps.Format)
}

func (ml *messagesList) drawTimestamps(builder *tview.LineBuilder, ts discord.Timestamp, baseStyle tcell.Style) {
	dimStyle := baseStyle.Dim(true)
	builder.Write(ml.formatTimestamp(ts)+" ", dimStyle)
}

func (ml *messagesList) drawAuthor(builder *tview.LineBuilder, message discord.Message, baseStyle tcell.Style) {
	name := message.Author.DisplayOrUsername()
	foreground := tcell.ColorDefault

	if member := ml.memberForMessage(message); member != nil {
		if member.Nick != "" {
			name = member.Nick
		}

		color, ok := state.MemberColor(member, func(id discord.RoleID) *discord.Role {
			r, _ := ml.chat.state.Cabinet.Role(message.GuildID, id)
			return r
		})
		if ok {
			foreground = tcell.NewHexColor(int32(color))
		}
	}

	style := baseStyle.Foreground(foreground).Bold(true)
	builder.Write(name+" ", style)
}

func (ml *messagesList) memberForMessage(message discord.Message) *discord.Member {
	// Webhooks do not have nicknames or roles.
	if !message.GuildID.IsValid() || message.WebhookID.IsValid() {
		return nil
	}

	member, err := ml.chat.state.Cabinet.Member(message.GuildID, message.Author.ID)
	if err != nil {
		slog.Error("failed to get member from state", "guild_id", message.GuildID, "member_id", message.Author.ID, "err", err)
		return nil
	}
	return member
}

func (ml *messagesList) drawContent(builder *tview.LineBuilder, message discord.Message, baseStyle tcell.Style, hideSpoilers bool) {
	lines, root := ml.renderContentLines(message, baseStyle, hideSpoilers)
	if ml.cfg.Markdown.Enabled && builder.HasCurrentLine() {
		startsWithCodeBlock := false
		if root != nil {
			if first := root.FirstChild(); first != nil {
				_, startsWithCodeBlock = first.(*ast.FencedCodeBlock)
			}
		}

		if startsWithCodeBlock {
			// Keep code blocks visually separate from "timestamp + author".
			builder.NewLine()
		}
		lines = trimLeadingContentLines(lines, startsWithCodeBlock)
	}
	builder.AppendLines(lines)
}

func trimLeadingContentLines(lines []tview.Line, startsWithCodeBlock bool) []tview.Line {
	if startsWithCodeBlock {
		for len(lines) > 0 && len(lines[0]) == 0 {
			lines = lines[1:]
		}
		return lines
	}
	for len(lines) > 1 && len(lines[0]) == 0 {
		lines = lines[1:]
	}
	return lines
}

func (ml *messagesList) renderContentLines(message discord.Message, baseStyle tcell.Style, hideSpoilers bool) ([]tview.Line, ast.Node) {
	return ml.renderContentLinesWithMarkdown(message, baseStyle, false, hideSpoilers)
}

func (ml *messagesList) renderContentLinesWithMarkdown(message discord.Message, baseStyle tcell.Style, forceMarkdown bool, hideSpoilers bool) ([]tview.Line, ast.Node) {
	// Keep one rendering path for both normal messages and embed fragments so we preserve mention/link parsing behavior consistently across both.
	if forceMarkdown || ml.cfg.Markdown.Enabled {
		content := strings.ReplaceAll(message.Content, "||", "\uFEFF||\uFEFF")
		c := []byte(content)
		root := discordmd.ParseWithMessage(c, *ml.chat.state.Cabinet, &message, false)
		renderer := markdown.NewRenderer(ml.cfg)
		renderer.HideSpoilers = hideSpoilers
		return renderer.RenderLines(c, root, baseStyle), root
	}

	b := tview.NewLineBuilder()
	if hideSpoilers {
		var sb strings.Builder
		for range message.Content {
			sb.WriteString("█")
		}
		b.Write(sb.String(), baseStyle)
	} else {
		b.Write(message.Content, baseStyle)
	}
	return b.Finish(), nil
}

func (ml *messagesList) drawSnapshotContent(builder *tview.LineBuilder, parent discord.Message, snapshot discord.MessageSnapshotMessage, baseStyle tcell.Style, hideSpoilers bool) {
	// Convert discord.MessageSnapshotMessage to discord.Message with common fields.
	message := discord.Message{
		Type:            snapshot.Type,
		Content:         snapshot.Content,
		Embeds:          snapshot.Embeds,
		Attachments:     snapshot.Attachments,
		Timestamp:       snapshot.Timestamp,
		EditedTimestamp: snapshot.EditedTimestamp,
		Flags:           snapshot.Flags,
		Mentions:        snapshot.Mentions,
		MentionRoleIDs:  snapshot.MentionRoleIDs,
		Stickers:        snapshot.Stickers,
		Components:      snapshot.Components,
		ChannelID:       parent.ChannelID,
		GuildID:         parent.GuildID,
	}

	ml.drawContent(builder, message, baseStyle, hideSpoilers)

	if message.EditedTimestamp.IsValid() {
		dimStyle := baseStyle.Dim(true)
		builder.Write(" (edited)", dimStyle)
	}

	ml.drawEmbeds(builder, message, baseStyle)

	for _, s := range message.Stickers {
		if ml.cfg.InlineImages.Enabled {
			continue
		}
		builder.NewLine()
		builder.Write("[Sticker: "+s.Name+"]", baseStyle.Italic(true))
	}

	attachmentStyle := ui.MergeStyle(baseStyle, ml.cfg.Theme.MessagesList.AttachmentStyle.Style)
	for _, a := range message.Attachments {
		if ml.cfg.InlineImages.Enabled && strings.HasPrefix(a.ContentType, "image/") {
			continue
		}

		builder.NewLine()
		if ml.cfg.ShowAttachmentLinks {
			builder.Write(a.Filename+":\n"+a.URL, attachmentStyle)
		} else {
			builder.Write(a.Filename, attachmentStyle)
		}
	}
}

func (ml *messagesList) drawDefaultMessage(builder *tview.LineBuilder, message discord.Message, baseStyle tcell.Style, hideSpoilers bool) {
	if ml.cfg.Timestamps.Enabled {
		ml.drawTimestamps(builder, message.Timestamp, baseStyle)
	}

	ml.drawAuthor(builder, message, baseStyle)
	ml.drawContent(builder, message, baseStyle, hideSpoilers)

	if message.EditedTimestamp.IsValid() {
		dimStyle := baseStyle.Dim(true)
		builder.Write(" (edited)", dimStyle)
	}

	ml.drawEmbeds(builder, message, baseStyle)

	ml.drawReactions(builder, message, baseStyle)

	for _, s := range message.Stickers {
		if ml.cfg.InlineImages.Enabled {
			continue
		}
		builder.NewLine()
		builder.Write("[Sticker: "+s.Name+"]", baseStyle.Italic(true))
	}

	attachmentStyle := ui.MergeStyle(baseStyle, ml.cfg.Theme.MessagesList.AttachmentStyle.Style)
	for _, a := range message.Attachments {
		if ml.cfg.InlineImages.Enabled && strings.HasPrefix(a.ContentType, "image/") {
			// We skip the visible text but ensure the scanner finds the URL in the background.
			// However, attachments have their own kind (messagesListRowImage) handled in buildItem.
			// But rich embed images (like tenor) are different.
			continue
		}

		builder.NewLine()
		if ml.cfg.ShowAttachmentLinks {
			builder.Write(a.Filename+":\n"+a.URL, attachmentStyle)
		} else {
			builder.Write(a.Filename, attachmentStyle)
		}
	}
}

func (ml *messagesList) drawEmbeds(builder *tview.LineBuilder, message discord.Message, baseStyle tcell.Style) {
	if len(message.Embeds) == 0 {
		return
	}

	contentListURLs := extractURLs(message.Content)
	contentURLs := make(map[string]struct{}, len(contentListURLs))
	for _, u := range contentListURLs {
		contentURLs[u] = struct{}{}
	}

	lineStyles := embedLineStyles(baseStyle, ml.cfg.Theme.MessagesList.Embeds)
	defaultBarStyle := baseStyle.Dim(true)
	prefixText := "  ▎ "
	prefixWidth := tview.TaggedStringWidth(prefixText)
	_, _, innerWidth, _ := ml.InnerRect()
	// Wrap against the current list viewport. This keeps embed wrapping stable even when sidebars/panes are resized.
	wrapWidth := max(innerWidth-prefixWidth, 1)

	seen := make(map[embedLineDedupKey]struct{})

	for _, embed := range message.Embeds {
		lines := embedLines(embed, contentURLs, ml.cfg.InlineImages.Enabled, seen)
		if len(lines) == 0 {
			continue
		}

		embedContentLines := make([]tview.Line, 0, len(lines)*2)
		barStyle := defaultBarStyle
		if embed.Color != discord.NullColor && embed.Color != 0 {
			barStyle = barStyle.Foreground(tcell.NewHexColor(int32(embed.Color)))
		}
		prefix := tview.NewSegment(prefixText, barStyle)
		for _, line := range lines {
			if strings.TrimSpace(line.Text) == "" {
				continue
			}
			msg := message
			msg.Content = line.Text
			lineStyle := lineStyles[line.Kind]
			// Embed descriptions are always markdown-rendered to match Discord's rich embed semantics, even when message markdown is globally disabled.
			rendered, _ := ml.renderContentLinesWithMarkdown(msg, lineStyle, line.Kind == embedLineDescription, false)
			for _, renderedLine := range rendered {
				if line.URL != "" {
					renderedLine = lineWithURL(renderedLine, line.URL)
				}
				// Prefix must be applied after wrapping so every visual line keeps the embed bar marker ("▎"), not only the first logical line.
				for _, wrapped := range wrapStyledLine(renderedLine, wrapWidth) {
					prefixed := make(tview.Line, 0, len(wrapped)+1)
					prefixed = append(prefixed, prefix)
					prefixed = append(prefixed, wrapped...)
					embedContentLines = append(embedContentLines, prefixed)
				}
			}
		}

		if len(embedContentLines) > 0 {
			builder.NewLine()
			builder.AppendLines(embedContentLines)
		}
	}
}

func (ml *messagesList) drawReactions(builder *tview.LineBuilder, message discord.Message, baseStyle tcell.Style) {
	if len(message.Reactions) == 0 {
		return
	}

	builder.NewLine()
	for i, r := range message.Reactions {
		if i > 0 {
			builder.Write(" ", baseStyle)
		}

		reactionStyle := baseStyle.Bold(r.Me)
		emojiStyle := ui.MergeStyle(reactionStyle, ml.cfg.Theme.MessagesList.EmojiStyle.Style)

		var url string
		if r.Emoji.ID != 0 {
			url = r.Emoji.EmojiURL()
		} else {
			url = markdown.TwemojiURL(r.Emoji.Name)
		}

		if url != "" && ml.cfg.InlineImages.Enabled {
			if ml.imageCache != nil {
				ml.imageCache.Request(url, 0, 0, func() {
					if ml.chat != nil && ml.chat.app != nil {
						triggerRedraw(ml.chat.app)
					}
				})
			}
			builder.Write(markdown.CustomEmojiText(r.Emoji.Name, true), emojiStyle.Url(url))
		} else {
			builder.Write(r.Emoji.Name, emojiStyle)
		}

		builder.Write(" ", reactionStyle.Url(""))
		builder.Write(strconv.Itoa(r.Count), reactionStyle.Url(""))
	}
}

func wrapStyledLine(line tview.Line, width int) []tview.Line {
	if width <= 0 {
		return []tview.Line{line}
	}
	if len(line) == 0 {
		return []tview.Line{line}
	}

	lines := make([]tview.Line, 0, 2)
	current := make(tview.Line, 0, len(line))
	currentWidth := 0

	pushSegment := func(text string, style tcell.Style) {
		if n := len(current); n > 0 && current[n-1].Style == style {
			current[n-1].Text += text
			return
		}
		current = append(current, tview.Segment{Text: text, Style: style})
	}

	flush := func() {
		lineCopy := make(tview.Line, len(current))
		copy(lineCopy, current)
		lines = append(lines, lineCopy)
		current = current[:0]
		currentWidth = 0
	}

	for _, segment := range line {
		state := -1
		rest := segment.Text
		for len(rest) > 0 {
			cluster, nextRest, boundaries, nextState := uniseg.StepString(rest, state)
			state = nextState
			rest = nextRest

			// Use grapheme width (not rune count) so wrapping stays correct with wide glyphs, emoji, and combining characters.
			clusterWidth := graphemeClusterWidth(boundaries)
			if currentWidth > 0 && currentWidth+clusterWidth > width {
				flush()
			}
			pushSegment(cluster, segment.Style)
			currentWidth += clusterWidth

			if currentWidth >= width {
				flush()
			}
		}
	}

	if len(current) > 0 {
		flush()
	}
	if len(lines) == 0 {
		return []tview.Line{{}}
	}
	return lines
}

func graphemeClusterWidth(boundaries int) int {
	return boundaries >> uniseg.ShiftWidth
}

func lineWithURL(line tview.Line, rawURL string) tview.Line {
	out := make(tview.Line, len(line))
	for i, segment := range line {
		out[i] = segment
		out[i].Style = out[i].Style.Url(rawURL)
	}
	return out
}

type embedLine struct {
	Text string
	Kind embedLineKind
	URL  string
}

type embedLineKind uint8

const (
	// Keep this ordering stable: drawEmbeds indexes precomputed style slots by this enum.
	embedLineProvider embedLineKind = iota
	embedLineAuthor
	embedLineTitle
	embedLineDescription
	embedLineFieldName
	embedLineFieldValue
	embedLineFooter
	embedLineURL
)

func embedLineStyles(baseStyle tcell.Style, theme config.MessagesListEmbedsTheme) [8]tcell.Style {
	styles := [8]tcell.Style{}
	styles[embedLineProvider] = ui.MergeStyle(baseStyle, theme.ProviderStyle.Style)
	styles[embedLineAuthor] = ui.MergeStyle(baseStyle, theme.AuthorStyle.Style)
	styles[embedLineTitle] = ui.MergeStyle(baseStyle, theme.TitleStyle.Style)
	styles[embedLineDescription] = ui.MergeStyle(baseStyle, theme.DescriptionStyle.Style)
	styles[embedLineFieldName] = ui.MergeStyle(baseStyle, theme.FieldNameStyle.Style)
	styles[embedLineFieldValue] = ui.MergeStyle(baseStyle, theme.FieldValueStyle.Style)
	styles[embedLineFooter] = ui.MergeStyle(baseStyle, theme.FooterStyle.Style)
	styles[embedLineURL] = ui.MergeStyle(baseStyle, theme.URLStyle.Style)
	return styles
}

type embedLineDedupKey struct {
	kind embedLineKind
	text string
}

func embedLines(embed discord.Embed, contentURLs map[string]struct{}, inlineImagesEnabled bool, seen map[embedLineDedupKey]struct{}) []embedLine {
	lines := make([]embedLine, 0, 8)

	appendUnique := func(s string, kind embedLineKind, rawURL string) {
		s = strings.TrimSpace(s)
		if s == "" {
			return
		}
		// Deduplicate by kind+text so the same value can intentionally appear in multiple semantic slots with different styles (e.g. title vs. field).
		key := embedLineDedupKey{kind: kind, text: s}
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		lines = append(lines, embedLine{
			Text: s,
			Kind: kind,
			URL:  rawURL,
		})
	}

	appendURL := func(url discord.URL) {
		u := strings.TrimSpace(string(url))
		if u == "" {
			return
		}
		// Avoid duplicating links that already appear in message body content.
		if _, ok := contentURLs[u]; ok {
			return
		}
		appendUnique(linkDisplayText(u), embedLineURL, u)
	}

	if embed.Provider != nil {
		appendUnique(embed.Provider.Name, embedLineProvider, "")
	}
	if embed.Author != nil {
		appendUnique(embed.Author.Name, embedLineAuthor, "")
	}
	appendUnique(embed.Title, embedLineTitle, string(embed.URL))
	// Some Discord embeds include markdown-escaped punctuation in raw payload text (e.g. "\."), so normalize for display.
	appendUnique(unescapeMarkdownEscapes(embed.Description), embedLineDescription, "")

	for _, field := range embed.Fields {
		switch {
		case field.Name != "" && field.Value != "":
			appendUnique(field.Name, embedLineFieldName, "")
			appendUnique(field.Value, embedLineFieldValue, "")
		case field.Name != "":
			appendUnique(field.Name, embedLineFieldName, "")
		default:
			appendUnique(field.Value, embedLineFieldValue, "")
		}
	}

	if embed.Footer != nil {
		appendUnique(embed.Footer.Text, embedLineFooter, "")
	}

	// Prefer media URLs after textual fields so previews read top-to-bottom before jumping to link targets.
	// When a title exists, embed.URL is represented by title Style.Url metadata instead of a separate URL row.
	if embed.Title == "" {
		appendURL(embed.URL)
	}
	if embed.Image != nil {
		if !inlineImagesEnabled {
			appendURL(embed.Image.URL)
		} else {
			u := string(embed.Image.URL)
			if strings.HasPrefix(u, "https://cdn.discordapp.com/emojis/") {
				// We need a single-cell placeholder to attach the metadata to.
				lines = append(lines, embedLine{
					Text: " ",
					Kind: embedLineDescription,
					URL:  u,
				})
			}
		}
	}
	if embed.Video != nil {
		if !inlineImagesEnabled {
			appendURL(embed.Video.URL)
		}
	}

	return lines
}

func linkDisplayText(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" {
		return raw
	}

	path := strings.TrimSpace(parsed.EscapedPath())
	switch {
	case path == "", path == "/":
		return parsed.Host
	case len(path) > 48:
		return parsed.Host + path[:45] + "..."
	default:
		return parsed.Host + path
	}
}

func unescapeMarkdownEscapes(s string) string {
	if !strings.ContainsRune(s, '\\') {
		return s
	}

	var b strings.Builder
	b.Grow(len(s))

	for i := range len(s) {
		if s[i] == '\\' && i+1 < len(s) && isMarkdownEscapable(s[i+1]) {
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

func isMarkdownEscapable(c byte) bool {
	switch c {
	case '\\', '`', '*', '_', '{', '}', '[', ']', '(', ')', '#', '+', '-', '.', '!', '|', '>', '~':
		return true
	default:
		return false
	}
}

func (ml *messagesList) drawForwardedMessage(builder *tview.LineBuilder, message discord.Message, baseStyle tcell.Style, hideSpoilers bool) {
	dimStyle := baseStyle.Dim(true)
	ml.drawTimestamps(builder, message.Timestamp, baseStyle)
	ml.drawAuthor(builder, message, baseStyle)
	builder.Write(ml.cfg.Theme.MessagesList.ForwardedIndicator+" ", dimStyle)

	for i, s := range message.MessageSnapshots {
		if i > 0 {
			builder.NewLine()
			builder.Write("  ", dimStyle)
		}
		ml.drawSnapshotContent(builder, message, s.Message, baseStyle, hideSpoilers)
		builder.Write(" ("+ml.formatTimestamp(s.Message.Timestamp)+") ", dimStyle)
	}
}

func (ml *messagesList) drawReplyMessage(builder *tview.LineBuilder, message discord.Message, baseStyle tcell.Style, hideSpoilers bool) {
	dimStyle := baseStyle.Dim(true)
	// indicator
	builder.Write(ml.cfg.Theme.MessagesList.ReplyIndicator+" ", dimStyle)

	if m := message.ReferencedMessage; m != nil {
		m.GuildID = message.GuildID
		ml.drawAuthor(builder, *m, dimStyle)
		ml.drawContent(builder, *m, dimStyle, hideSpoilers)
	} else {
		builder.Write("Original message was deleted", dimStyle)
	}

	builder.NewLine()
	// main
	ml.drawDefaultMessage(builder, message, baseStyle, hideSpoilers)
}

func (ml *messagesList) drawPinnedMessage(builder *tview.LineBuilder, message discord.Message, baseStyle tcell.Style, hideSpoilers bool) {
	builder.Write(message.Author.DisplayOrUsername(), baseStyle)
	builder.Write(" pinned a message.", baseStyle)
}

func (ml *messagesList) selectedMessage() (*discord.Message, error) {
	if len(ml.messages) == 0 {
		return nil, errors.New("no messages available")
	}

	cursor := ml.Cursor()
	if cursor == -1 || cursor >= len(ml.messages) {
		return nil, errors.New("no message is currently selected")
	}

	return &ml.messages[cursor], nil
}

func (ml *messagesList) Update(msg tview.Msg) tview.Cmd {
	switch msg := msg.(type) {
	case *tview.MouseMsg:
		if msg.Action != tview.MouseLeftClick {
			break
		}

		x, y := msg.Position()
		if !ml.InRect(x, y) {
			break
		}

		if ml.lastScreen != nil {
			_, style, _ := ml.lastScreen.Get(x, y)
			_, url := style.GetUrl()
			if url != "" {
				go ml.openURL(url)
				return nil
			}
		}
	case *tview.KeyMsg:
		switch {
		case keybind.Matches(msg, ml.cfg.Keybinds.MessagesList.Cancel.Keybind):
			ml.clearSelection()
			return nil
		case keybind.Matches(msg, ml.cfg.Keybinds.MessagesList.SelectUp.Keybind):
			return ml.selectUp()
		case keybind.Matches(msg, ml.cfg.Keybinds.MessagesList.SelectDown.Keybind):
			ml.selectDown()
			return nil
		case keybind.Matches(msg, ml.cfg.Keybinds.MessagesList.SelectTop.Keybind):
			ml.selectTop()
			return nil
		case keybind.Matches(msg, ml.cfg.Keybinds.MessagesList.SelectBottom.Keybind):
			ml.selectBottom()
			return nil
		case keybind.Matches(msg, ml.cfg.Keybinds.MessagesList.SelectReply.Keybind):
			ml.selectReply()
			return nil
		case keybind.Matches(msg, ml.cfg.Keybinds.MessagesList.YankID.Keybind):
			return ml.yankMessageID()
		case keybind.Matches(msg, ml.cfg.Keybinds.MessagesList.YankContent.Keybind):
			return ml.yankContent()
		case keybind.Matches(msg, ml.cfg.Keybinds.MessagesList.YankURL.Keybind):
			return ml.yankURL()
		case keybind.Matches(msg, ml.cfg.Keybinds.MessagesList.Open.Keybind) || msg.Key() == tcell.KeyEnter:
			return ml.open()
		case keybind.Matches(msg, ml.cfg.Keybinds.MessagesList.React.Keybind) || (msg.Key() == tcell.KeyRune && msg.Str() == "+"):
			return ml.showReactionPicker()
		case keybind.Matches(msg, ml.cfg.Keybinds.MessagesList.Pin.Keybind):
			ml.confirmPin()
			return nil
		case keybind.Matches(msg, ml.cfg.Keybinds.MessagesList.Reply.Keybind):
			return ml.reply(false)
		case keybind.Matches(msg, ml.cfg.Keybinds.MessagesList.ReplyMention.Keybind):
			return ml.reply(true)
		case keybind.Matches(msg, ml.cfg.Keybinds.MessagesList.Edit.Keybind):
			return ml.editSelectedMessage()
		case keybind.Matches(msg, ml.cfg.Keybinds.MessagesList.Delete.Keybind):
			return ml.deleteSelectedMessage()
		case keybind.Matches(msg, ml.cfg.Keybinds.MessagesList.DeleteConfirm.Keybind):
			ml.confirmDelete()
			return nil
		}
		return ml.Model.Update(msg)

	case *olderMessagesLoadedMsg:
		ml.fetchingOlder = false
		selectedChannel := ml.chat.SelectedChannel()
		if selectedChannel == nil || selectedChannel.ID != msg.ChannelID || len(msg.Older) == 0 {
			return nil
		}
		prevCursor := ml.Cursor()

		// Defensive invalidation if Discord returns overlapping windows.
		for _, message := range msg.Older {
			delete(ml.itemByID, message.ID)
		}
		ml.messages = slices.Concat(msg.Older, ml.messages)
		ml.invalidateRows()

		switch {
		case prevCursor == 0:
			// Preserve "SelectUp at top" semantics: move to the next older message.
			ml.SetCursor(len(msg.Older) - 1)
		case prevCursor > 0:
			// Keep selection on the same message after prepend shifts indexes.
			ml.SetCursor(prevCursor + len(msg.Older))
		default:
			ml.SetCursor(prevCursor)
		}
		return nil
	}
	return ml.Model.Update(msg)
}

func (ml *messagesList) selectUp() tview.Cmd {
	messages := ml.messages
	if len(messages) == 0 {
		return nil
	}

	cursor := ml.Cursor()
	switch {
	case cursor == -1:
		cursor = len(messages) - 1
	case cursor > 0:
		cursor--
	case cursor == 0:
		return ml.fetchOlderMessages()
	}

	ml.SetCursor(cursor)
	return nil
}

func (ml *messagesList) selectDown() {
	messages := ml.messages
	if len(messages) == 0 {
		return
	}

	cursor := ml.Cursor()
	switch {
	case cursor == -1:
		cursor = len(messages) - 1
	case cursor < len(messages)-1:
		cursor++
	}

	ml.SetCursor(cursor)
}

func (ml *messagesList) selectTop() {
	if len(ml.messages) == 0 {
		return
	}
	ml.SetCursor(0)
}

func (ml *messagesList) selectBottom() {
	if len(ml.messages) == 0 {
		return
	}
	ml.SetCursor(len(ml.messages) - 1)
}

func (ml *messagesList) selectReply() {
	messages := ml.messages
	if len(messages) == 0 {
		return
	}

	cursor := ml.Cursor()
	if cursor == -1 || cursor >= len(messages) {
		return
	}

	if ref := messages[cursor].ReferencedMessage; ref != nil {
		refIdx := slices.IndexFunc(messages, func(m discord.Message) bool {
			return m.ID == ref.ID
		})
		if refIdx != -1 {
			ml.SetCursor(refIdx)
		}
	}
}

func (ml *messagesList) fetchOlderMessages() tview.Cmd {
	if ml.fetchingOlder {
		return nil
	}
	selectedChannel := ml.chat.SelectedChannel()
	if selectedChannel == nil {
		return nil
	}

	ml.fetchingOlder = true
	channelID := selectedChannel.ID
	before := ml.messages[0].ID
	limit := uint(ml.cfg.MessagesLimit)
	return func() tview.Msg {
		messages, err := ml.chat.state.MessagesBefore(channelID, before, limit)
		if err != nil {
			slog.Error("failed to fetch older messages", "err", err)
			return newOlderMessagesLoadedMsg(channelID, nil)
		}
		if len(messages) == 0 {
			return newOlderMessagesLoadedMsg(channelID, nil)
		}

		if guildID := selectedChannel.GuildID; guildID.IsValid() {
			ml.requestGuildMembers(guildID, messages)
		}

		older := slices.Clone(messages)
		slices.Reverse(older)
		return newOlderMessagesLoadedMsg(channelID, older)
	}
}

func (ml *messagesList) prependOlderMessages() int {
	cmd := ml.fetchOlderMessages()
	if cmd == nil {
		return 0
	}

	msg, ok := cmd().(*olderMessagesLoadedMsg)
	if !ok || msg == nil {
		return 0
	}

	ml.Update(msg)
	return len(msg.Older)
}

func (ml *messagesList) jumpToMessage(channel discord.Channel, messageID discord.MessageID) error {
	if !channel.ID.IsValid() || !messageID.IsValid() {
		return errors.New("invalid channel or message id")
	}

	limit := uint(max(ml.cfg.MessagesLimit, 100))
	messages, err := ml.chat.state.MessagesAround(channel.ID, messageID, limit)
	if err != nil {
		return err
	}
	if len(messages) == 0 {
		return errors.New("message not found")
	}

	if guildID := channel.GuildID; guildID.IsValid() {
		ml.requestGuildMembers(guildID, messages)
	}

	ml.chat.SetSelectedChannel(&channel)
	ml.chat.clearTypers()
	ml.setTitle(channel)
	ml.setMessages(messages)

	target := slices.IndexFunc(ml.messages, func(message discord.Message) bool {
		return message.ID == messageID
	})
	if target == -1 {
		return errors.New("message not present in loaded window")
	}

	ml.SetCursor(target)
	return nil
}

func (ml *messagesList) yankMessageID() tview.Cmd {
	msg, err := ml.selectedMessage()
	if err != nil {
		slog.Error("failed to get selected message", "err", err)
		return nil
	}

	return func() tview.Msg {
		if err := clipboardWrite(clipboard.FmtText, []byte(msg.ID.String())); err != nil {
			slog.Error("failed to copy message id", "err", err)
		}
		return nil
	}
}

func (ml *messagesList) messageText(msg discord.Message, row messagesListRow) string {
	switch row.kind {
	case messagesListRowImage:
		if len(msg.MessageSnapshots) > 0 {
			return string(msg.MessageSnapshots[row.snapshotIndex].Message.Attachments[row.attachmentIndex].URL)
		}
		return string(msg.Attachments[row.attachmentIndex].URL)
	case messagesListRowSticker:
		if len(msg.MessageSnapshots) > 0 {
			return ui.StickerURL(msg.MessageSnapshots[row.snapshotIndex].Message.Stickers[row.stickerIndex])
		}
		return ui.StickerURL(msg.Stickers[row.stickerIndex])
	case messagesListRowEmbedImage:
		if len(msg.MessageSnapshots) > 0 {
			e := msg.MessageSnapshots[row.snapshotIndex].Message.Embeds[row.embedIndex]
			if row.isThumbnail {
				return string(e.Thumbnail.URL)
			}
			return string(e.Image.URL)
		}
		if row.isThumbnail {
			return string(msg.Embeds[row.embedIndex].Thumbnail.URL)
		}
		return string(msg.Embeds[row.embedIndex].Image.URL)
	}

	if msg.Content != "" {
		return msg.Content
	}

	if len(msg.MessageSnapshots) > 0 {
		snapshot := msg.MessageSnapshots[0].Message
		if snapshot.Content != "" {
			return snapshot.Content
		}
		if len(snapshot.Attachments) > 0 {
			return string(snapshot.Attachments[0].URL)
		}
		if len(snapshot.Stickers) > 0 {
			return ui.StickerURL(snapshot.Stickers[0])
		}
		if len(snapshot.Embeds) > 0 {
			if snapshot.Embeds[0].Image != nil {
				return string(snapshot.Embeds[0].Image.URL)
			}
			if snapshot.Embeds[0].Thumbnail != nil {
				return string(snapshot.Embeds[0].Thumbnail.URL)
			}
		}
	}

	switch msg.Type {
	case discord.GuildMemberJoinMessage:
		return msg.Author.DisplayOrUsername() + " joined the server."
	case discord.ChannelPinnedMessage:
		return msg.Author.DisplayOrUsername() + " pinned a message."
	}

	return ""
}

func (ml *messagesList) yankContent() tview.Cmd {
	ml.ensureRows()
	rowIndex := ml.Model.Cursor()
	if rowIndex < 0 || rowIndex >= len(ml.rows) {
		return nil
	}

	row := ml.rows[rowIndex]
	if row.kind == messagesListRowSeparator {
		return nil
	}

	msg := ml.messages[row.messageIndex]
	text := ml.messageText(msg, row)

	return func() tview.Msg {
		if err := clipboardWrite(clipboard.FmtText, []byte(text)); err != nil {
			slog.Error("failed to copy message content", "err", err)
		}
		return nil
	}
}

func (ml *messagesList) yankURL() tview.Cmd {
	ml.ensureRows()
	rowIndex := ml.Model.Cursor()
	if rowIndex < 0 || rowIndex >= len(ml.rows) {
		return nil
	}

	row := ml.rows[rowIndex]
	if row.kind == messagesListRowSeparator {
		return nil
	}

	msg := ml.messages[row.messageIndex]
	var url string
	switch row.kind {
	case messagesListRowImage:
		if len(msg.MessageSnapshots) > 0 {
			url = string(msg.MessageSnapshots[row.snapshotIndex].Message.Attachments[row.attachmentIndex].URL)
		} else {
			url = string(msg.Attachments[row.attachmentIndex].URL)
		}
	case messagesListRowSticker:
		if len(msg.MessageSnapshots) > 0 {
			url = ui.StickerURL(msg.MessageSnapshots[row.snapshotIndex].Message.Stickers[row.stickerIndex])
		} else {
			url = ui.StickerURL(msg.Stickers[row.stickerIndex])
		}
	case messagesListRowEmbedImage:
		var e discord.Embed
		if len(msg.MessageSnapshots) > 0 {
			e = msg.MessageSnapshots[row.snapshotIndex].Message.Embeds[row.embedIndex]
		} else {
			e = msg.Embeds[row.embedIndex]
		}

		if row.isThumbnail {
			url = string(e.Thumbnail.URL)
		} else {
			url = string(e.Image.URL)
		}
	default:
		url = msg.URL()
	}

	return func() tview.Msg {
		if err := clipboardWrite(clipboard.FmtText, []byte(url)); err != nil {
			slog.Error("failed to copy message url", "err", err)
		}
		return nil
	}
}

func (ml *messagesList) open() tview.Cmd {
	ml.ensureRows()
	rowIndex := ml.Model.Cursor()
	if rowIndex < 0 || rowIndex >= len(ml.rows) {
		return nil
	}
	row := ml.rows[rowIndex]

	msg, err := ml.selectedMessage()
	if err != nil {
		slog.Error("failed to get selected message", "err", err)
		return nil
	}

	if msg.Reference != nil && msg.Reference.MessageID.IsValid() {
		if row.kind == messagesListRowMessage {
			channelID := msg.Reference.ChannelID
			if !channelID.IsValid() {
				channelID = msg.ChannelID
			}

			var channel *discord.Channel
			if sc := ml.chat.SelectedChannel(); sc != nil && sc.ID == channelID {
				channel = sc
			} else {
				channel, _ = ml.chat.state.Cabinet.Channel(channelID)
			}

			if channel != nil {
				return func() tview.Msg {
					if err := ml.jumpToMessage(*channel, msg.Reference.MessageID); err != nil {
						slog.Error("failed to jump to referenced message", "err", err, "channel_id", channel.ID, "message_id", msg.Reference.MessageID)
					}
					return nil
				}
			}
		}
	}

	urls := messageURLs(*msg)
	var attachments []discord.Attachment
	attachments = append(attachments, msg.Attachments...)
	for _, snapshot := range msg.MessageSnapshots {
		urls = append(urls, extractURLs(snapshot.Message.Content)...)
		urls = append(urls, extractEmbedURLs(snapshot.Message.Embeds)...)
		attachments = append(attachments, snapshot.Message.Attachments...)
	}

	if len(urls) == 0 && len(attachments) == 0 {
		return nil
	}

	if len(urls)+len(attachments) == 1 {
		if len(urls) == 1 {
			go ml.openURL(urls[0])
		} else {
			attachment := attachments[0]
			if strings.HasPrefix(attachment.ContentType, "image/") {
				go ml.openAttachment(attachments[0])
			} else {
				go ml.openURL(attachment.URL)
			}
		}
	} else {
		return ml.showAttachmentsList(urls, attachments)
	}
	return nil
}

func extractURLs(content string) []string {
	src := []byte(content)
	node := parser.NewParser(
		parser.WithBlockParsers(discordmd.BlockParsers()...),
		parser.WithInlineParsers(discordmd.InlineParserWithLink()...),
	).Parse(text.NewReader(src))

	var urls []string
	ast.Walk(node, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if entering {
			switch n := n.(type) {
			case *ast.AutoLink:
				urls = append(urls, string(n.URL(src)))
			case *ast.Link:
				urls = append(urls, string(n.Destination))
			}
		}

		return ast.WalkContinue, nil
	})
	return urls
}

func extractEmbedURLs(embeds []discord.Embed) []string {
	urls := make([]string, 0, len(embeds)*3)
	for _, embed := range embeds {
		if embed.URL != "" {
			urls = append(urls, string(embed.URL))
		}
		if embed.Image != nil && embed.Image.URL != "" {
			urls = append(urls, string(embed.Image.URL))
		}
		if embed.Thumbnail != nil && embed.Thumbnail.URL != "" {
			urls = append(urls, string(embed.Thumbnail.URL))
		}
		if embed.Video != nil && embed.Video.URL != "" {
			urls = append(urls, string(embed.Video.URL))
		}
	}
	return urls
}

func messageURLs(msg discord.Message) []string {
	combined := extractURLs(msg.Content)
	combined = append(combined, extractEmbedURLs(msg.Embeds)...)

	urls := make([]string, 0, len(combined))
	seen := make(map[string]struct{}, len(combined))
	for _, u := range combined {
		u = strings.TrimSpace(u)
		if u == "" {
			continue
		}
		if _, ok := seen[u]; ok {
			continue
		}
		seen[u] = struct{}{}
		urls = append(urls, u)
	}
	return urls
}

func (ml *messagesList) showAttachmentsList(urls []string, attachments []discord.Attachment) tview.Cmd {
	var items []attachmentItem
	for _, a := range attachments {
		attachment := a
		action := func() {
			if strings.HasPrefix(attachment.ContentType, "image/") {
				go ml.openAttachment(attachment)
			} else {
				go ml.openURL(attachment.URL)
			}
		}
		items = append(items, attachmentItem{
			label: attachment.Filename,
			open:  action,
		})
	}
	for _, u := range urls {
		url := u
		items = append(items, attachmentItem{
			label: url,
			open:  func() { go ml.openURL(url) },
		})
	}
	ml.attachmentsPicker.SetItems(items)

	ml.chat.
		AddLayer(
			ui.Centered(ml.attachmentsPicker, ml.cfg.Picker.Width, ml.cfg.Picker.Height),
			layers.WithName(attachmentsPickerLayerName),
			layers.WithResize(true),
			layers.WithVisible(true),
			layers.WithOverlay(),
		).
		SendToFront(attachmentsPickerLayerName)
	return tview.SetFocus(ml.attachmentsPicker)
}

func (ml *messagesList) showReactionPicker() tview.Cmd {
	if _, err := ml.selectedMessage(); err != nil {
		slog.Error("failed to get selected message", "err", err)
		return nil
	}

	selected := ml.chat.SelectedChannel()
	if selected == nil {
		return nil
	}

	if ml.chat.HasLayer(reactionPickerLayerName) {
		ml.chat.RemoveLayer(reactionPickerLayerName)
	}

	// Load all emojis synchronously (same as ':' autocomplete path).
	emojis := availableEmojisForChannel(ml.chat.state, selected)
	ml.reactionPicker.SetItems(emojis)

	ml.chat.
		AddLayer(
			ui.Centered(ml.reactionPicker, ml.cfg.Picker.Width, ml.cfg.Picker.Height),
			layers.WithName(reactionPickerLayerName),
			layers.WithResize(true),
			layers.WithVisible(true),
			layers.WithOverlay(),
		).
		SendToFront(reactionPickerLayerName)

	return tview.SetFocus(ml.reactionPicker)
}

func (ml *messagesList) openAttachment(attachment discord.Attachment) {
	resp, err := httpGetAttachment(attachment.URL)
	if err != nil {
		slog.Error("failed to fetch the attachment", "err", err, "url", attachment.URL)
		return
	}
	defer resp.Body.Close()

	path := filepath.Join(consts.CacheDir(), "attachments")
	if err := mkdirAllAttachment(path, os.ModePerm); err != nil {
		slog.Error("failed to create attachments dir", "err", err, "path", path)
		return
	}

	path = filepath.Join(path, attachment.Filename)
	file, err := createAttachmentFile(path)
	if err != nil {
		slog.Error("failed to create attachment file", "err", err, "path", path)
		return
	}
	defer file.Close()

	if _, err := copyAttachmentData(file, resp.Body); err != nil {
		slog.Error("failed to copy attachment to file", "err", err)
		return
	}

	if err := openStart(path); err != nil {
		slog.Error("failed to open attachment file", "err", err, "path", path)
		return
	}
}

func (ml *messagesList) openURL(url string) {
	if err := openStart(url); err != nil {
		slog.Error("failed to open URL", "err", err, "url", url)
	}
}

func (ml *messagesList) reply(mention bool) tview.Cmd {
	message, err := ml.selectedMessage()
	if err != nil {
		slog.Error("failed to get selected message", "err", err)
		return nil
	}

	name := message.Author.DisplayOrUsername()
	if member := ml.memberForMessage(*message); member != nil && member.Nick != "" {
		name = member.Nick
	}

	data := ml.chat.messageInput.sendMessageData
	data.Reference = &discord.MessageReference{MessageID: message.ID}
	data.AllowedMentions = &api.AllowedMentions{RepliedUser: option.False}

	title := "Replying to "
	if mention {
		data.AllowedMentions.RepliedUser = option.True
		title = "[@] " + title
	}

	ml.chat.messageInput.sendMessageData = data
	ml.chat.messageInput.SetTitle(title + name)
	return tview.SetFocus(ml.chat.messageInput)
}

func (ml *messagesList) editSelectedMessage() tview.Cmd {
	message, err := ml.selectedMessage()
	if err != nil {
		slog.Error("failed to get selected message", "err", err)
		return nil
	}

	me, _ := ml.chat.state.Cabinet.Me()
	if message.Author.ID != me.ID {
		slog.Error("failed to edit message; not the author", "channel_id", message.ChannelID, "message_id", message.ID)
		return nil
	}

	ml.chat.messageInput.SetTitle("Editing")
	ml.chat.messageInput.edit = true
	ml.chat.messageInput.SetText(message.Content, true)
	return tview.SetFocus(ml.chat.messageInput)
}

func (ml *messagesList) edit() {
	ml.editSelectedMessage()
}

func (ml *messagesList) canManagePins() bool {
	selected := ml.chat.SelectedChannel()
	if selected == nil {
		return false
	}

	if selected.Type == discord.DirectMessage || selected.Type == discord.GroupDM {
		return true
	}

	return ml.chat.state.HasPermissions(selected.ID, discord.PermissionManageMessages)
}

func (ml *messagesList) canPinMessage(message *discord.Message) bool {
	return message != nil && ml.canManagePins()
}

func (ml *messagesList) setMessagePinned(channelID discord.ChannelID, messageID discord.MessageID, pinned bool) {
	for i := range ml.messages {
		if ml.messages[i].ID != messageID {
			continue
		}

		ml.messages[i].Pinned = pinned
		_ = ml.chat.state.Cabinet.MessageStore.MessageSet(&ml.messages[i], true)
		delete(ml.itemByID, messageID)
		return
	}

	cached, err := ml.chat.state.Cabinet.MessageStore.Message(channelID, messageID)
	if err != nil || cached == nil {
		return
	}

	cached.Pinned = pinned
	_ = ml.chat.state.Cabinet.MessageStore.MessageSet(cached, true)
	delete(ml.itemByID, messageID)
}

func (ml *messagesList) confirmPin() {
	message, err := ml.selectedMessage()
	if err != nil {
		slog.Error("failed to get selected message", "err", err)
		return
	}
	if !ml.canPinMessage(message) {
		slog.Error("failed to pin message; missing relevant permissions", "channel_id", message.ChannelID, "message_id", message.ID)
		return
	}

	onChoice := func(choice string) {
		if choice == "yes" {
			ml.pin()
		}
	}

	ml.chat.showPinConfirmDialog(ml.renderMessage(*message, ml.cfg.Theme.MessagesList.SelectedMessageStyle.Style, false), onChoice)
}

func (ml *messagesList) pin() {
	msg, err := ml.selectedMessage()
	if err != nil {
		slog.Error("failed to get selected message", "err", err)
		return
	}

	if !ml.canPinMessage(msg) {
		slog.Error("failed to pin message; missing relevant permissions", "channel_id", msg.ChannelID, "message_id", msg.ID)
		return
	}

	selected := ml.chat.SelectedChannel()
	if err := pinMessageFunc(ml.chat.state.State, selected.ID, msg.ID, ""); err != nil {
		slog.Error("failed to pin message", "channel_id", selected.ID, "message_id", msg.ID, "err", err)
		return
	}

	ml.setMessagePinned(selected.ID, msg.ID, true)
}

func (ml *messagesList) confirmDelete() {
	onChoice := func(choice string) {
		if choice == "Yes" {
			if cmd := ml.deleteSelectedMessage(); cmd != nil {
				cmd()
			}
		}
	}

	ml.chat.showConfirmModal(
		"Are you sure you want to delete this message?",
		[]string{"Yes", "No"},
		onChoice,
	)
}

func (ml *messagesList) deleteSelectedMessage() tview.Cmd {
	selectedMessage, err := ml.selectedMessage()
	if err != nil {
		slog.Error("failed to get selected message", "err", err)
		return nil
	}

	return func() tview.Msg {
		if selectedMessage.GuildID.IsValid() {
			me, _ := ml.chat.state.Cabinet.Me()
			if selectedMessage.Author.ID != me.ID && !ml.chat.state.HasPermissions(selectedMessage.ChannelID, discord.PermissionManageMessages) {
				slog.Error("failed to delete message; missing relevant permissions", "channel_id", selectedMessage.ChannelID, "message_id", selectedMessage.ID)
				return nil
			}
		}

		if err := deleteMessageFunc(ml.chat.state.State, selectedMessage.ChannelID, selectedMessage.ID, ""); err != nil {
			slog.Error("failed to delete message", "channel_id", selectedMessage.ChannelID, "message_id", selectedMessage.ID, "err", err)
			return nil
		}

		if err := messageRemoveFunc(ml.chat.state.State, selectedMessage.ChannelID, selectedMessage.ID); err != nil {
			slog.Error("failed to delete message", "channel_id", selectedMessage.ChannelID, "message_id", selectedMessage.ID, "err", err)
			return nil
		}
		return nil
	}
}

func (ml *messagesList) delete() {
	if command := ml.deleteSelectedMessage(); command != nil {
		command()
	}
}

func (ml *messagesList) requestGuildMembers(guildID discord.GuildID, messages []discord.Message) {
	usersToFetch := make([]discord.UserID, 0, len(messages))
	seen := make(map[discord.UserID]struct{}, len(messages))

	for _, message := range messages {
		// Do not fetch member for a webhook message.
		if message.WebhookID.IsValid() {
			continue
		}

		if member, _ := ml.chat.state.Cabinet.Member(guildID, message.Author.ID); member == nil {
			userID := message.Author.ID
			if _, ok := seen[userID]; !ok {
				seen[userID] = struct{}{}
				usersToFetch = append(usersToFetch, userID)
			}
		}
	}

	if len(usersToFetch) > 0 {
		err := sendGatewayFunc(ml.chat.state.State, context.Background(), &gateway.RequestGuildMembersCommand{
			GuildIDs: []discord.GuildID{guildID},
			UserIDs:  usersToFetch,
		})
		if err != nil {
			slog.Error("failed to request guild members", "guild_id", guildID, "err", err)
			return
		}

		ml.setFetchingChunk(true, 0)
		ml.waitForChunkEvent()
	}
}

func (ml *messagesList) setFetchingChunk(value bool, count uint) {
	ml.fetchingMembers.mu.Lock()
	defer ml.fetchingMembers.mu.Unlock()

	if ml.fetchingMembers.value == value {
		return
	}

	ml.fetchingMembers.value = value

	if value {
		ml.fetchingMembers.done = make(chan struct{})
	} else {
		ml.fetchingMembers.count = count
		close(ml.fetchingMembers.done)
	}
}

func (ml *messagesList) waitForChunkEvent() uint {
	ml.fetchingMembers.mu.Lock()
	if !ml.fetchingMembers.value {
		ml.fetchingMembers.mu.Unlock()
		return 0
	}
	ml.fetchingMembers.mu.Unlock()

	<-ml.fetchingMembers.done
	return ml.fetchingMembers.count
}

func (ml *messagesList) ShortHelp() []keybind.Keybind {
	cfg := ml.cfg.Keybinds.MessagesList
	help := []keybind.Keybind{
		cfg.SelectUp.Keybind,
		cfg.SelectDown.Keybind,
		cfg.Cancel.Keybind,
	}

	if msg, err := ml.selectedMessage(); err == nil {
		me, _ := ml.chat.state.Cabinet.Me()
		if msg.Author.ID != me.ID {
			help = append(help, cfg.Reply.Keybind)
		}
		help = append(help, cfg.React.Keybind)
		if ml.canPinMessage(msg) {
			help = append(help, cfg.Pin.Keybind)
		}
	}

	return help
}

func (ml *messagesList) FullHelp() [][]keybind.Keybind {
	cfg := ml.cfg.Keybinds.MessagesList

	canSelectReply := false
	canReply := false
	canEdit := false
	canDelete := false
	canOpen := false
	if msg, err := ml.selectedMessage(); err == nil {
		canSelectReply = msg.ReferencedMessage != nil
		canOpen = len(messageURLs(*msg)) != 0 || len(msg.Attachments) != 0

		me, _ := ml.chat.state.Cabinet.Me()
		canReply = msg.Author.ID != me.ID
		canEdit = msg.Author.ID == me.ID
		canDelete = canEdit
		if !canDelete {
			selected := ml.chat.SelectedChannel()
			canDelete = selected != nil && ml.chat.state.HasPermissions(selected.ID, discord.PermissionManageMessages)
		}
	}

	actions := make([]keybind.Keybind, 0, 4)
	if canReply {
		actions = append(actions, cfg.Reply.Keybind, cfg.ReplyMention.Keybind)
	}
	if selected, err := ml.selectedMessage(); err == nil && selected != nil {
		actions = append(actions, cfg.React.Keybind)
	}
	if canSelectReply {
		actions = append(actions, cfg.SelectReply.Keybind)
	}
	actions = append(actions, cfg.Cancel.Keybind)

	manage := make([]keybind.Keybind, 0, 4)
	if canEdit {
		manage = append(manage, cfg.Edit.Keybind)
	}
	if selected, err := ml.selectedMessage(); err == nil && ml.canPinMessage(selected) {
		manage = append(manage, cfg.Pin.Keybind)
	}
	if canDelete {
		manage = append(manage, cfg.DeleteConfirm.Keybind, cfg.Delete.Keybind)
	}
	if canOpen {
		manage = append(manage, cfg.Open.Keybind)
	}

	return [][]keybind.Keybind{
		{cfg.SelectUp.Keybind, cfg.SelectDown.Keybind, cfg.SelectTop.Keybind, cfg.SelectBottom.Keybind},
		{cfg.ScrollUp.Keybind, cfg.ScrollDown.Keybind, cfg.ScrollTop.Keybind, cfg.ScrollBottom.Keybind},
		actions,
		manage,
		{cfg.YankContent.Keybind, cfg.YankURL.Keybind, cfg.YankID.Keybind},
	}
}
