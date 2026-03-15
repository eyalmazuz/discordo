package chat

import (
	"fmt"
	"log/slog"
	"strings"

	"github.com/ayn2op/discordo/internal/config"
	imgpkg "github.com/ayn2op/discordo/internal/image"
	"github.com/ayn2op/discordo/internal/markdown"
	"github.com/ayn2op/discordo/internal/ui"
	"github.com/ayn2op/discordo/pkg/picker"
	"github.com/eyalmazuz/tview"
	"github.com/eyalmazuz/tview/help"
	"github.com/eyalmazuz/tview/keybind"
	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/gdamore/tcell/v3"
)

type reactionPicker struct {
	*picker.Picker
	cfg          *config.Config
	chatView     *Model
	messagesList *messagesList
	imageCache   *imgpkg.Cache
	items        []discord.Emoji

	emoteItemByKey map[string]*imageItem
	nextKittyID    uint32
}

var _ help.KeyMap = (*reactionPicker)(nil)

func newReactionPicker(cfg *config.Config, chatView *Model, messagesList *messagesList, imageCache *imgpkg.Cache) *reactionPicker {
	rp := &reactionPicker{
		Picker:         picker.New(),
		cfg:            cfg,
		chatView:       chatView,
		messagesList:   messagesList,
		imageCache:     imageCache,
		emoteItemByKey: make(map[string]*imageItem),
		nextKittyID:    1,
	}
	rp.SetFocusFunc(func(p tview.Primitive) {
		chatView.app.SetFocus(p)
	})
	rp.Box = ui.ConfigureBox(tview.NewBox(), &cfg.Theme)
	rp.
		SetBlurFunc(nil).
		SetFocusFunc(nil).
		SetBorderSet(cfg.Theme.Border.ActiveSet.BorderSet).
		SetBorderStyle(cfg.Theme.Border.ActiveStyle.Style).
		SetTitleStyle(cfg.Theme.Title.ActiveStyle.Style).
		SetFooterStyle(cfg.Theme.Footer.ActiveStyle.Style)

	rp.SetTitle("Reactions")
	rp.SetSelectedFunc(rp.onSelected)
	rp.SetCancelFunc(rp.close)
	rp.SetKeyMap(&picker.KeyMap{
		Cancel:      cfg.Keybinds.Picker.Cancel.Keybind,
		ToggleFocus: cfg.Keybinds.Picker.ToggleFocus.Keybind,
		Up:          cfg.Keybinds.Picker.Up.Keybind,
		Down:        cfg.Keybinds.Picker.Down.Keybind,
		Top:         cfg.Keybinds.Picker.Top.Keybind,
		Bottom:      cfg.Keybinds.Picker.Bottom.Keybind,
		Select:      cfg.Keybinds.Picker.Select.Keybind,
	})
	rp.SetScrollBarVisibility(cfg.Theme.ScrollBar.Visibility.ScrollBarVisibility)
	rp.SetScrollBar(tview.NewScrollBar().
		SetTrackStyle(cfg.Theme.ScrollBar.TrackStyle.Style).
		SetThumbStyle(cfg.Theme.ScrollBar.ThumbStyle.Style).
		SetGlyphSet(cfg.Theme.ScrollBar.GlyphSet.GlyphSet))
	return rp
}

func (rp *reactionPicker) SetItems(items []discord.Emoji) {
	rp.items = append(rp.items[:0], items...)
	rp.ClearItems()

	for i, emoji := range items {
		rp.AddItem(picker.Item{
			Text:       emoji.Name,
			Line:       rp.lineForEmoji(emoji),
			FilterText: emoji.Name,
			Reference:  i,
		})
	}

	rp.SetFooter("Tab focus")
	rp.Update()
}

func (rp *reactionPicker) onSelected(item picker.Item) {
	index, ok := item.Reference.(int)
	if !ok || index < 0 || index >= len(rp.items) {
		return
	}

	message, err := rp.messagesList.selectedMessage()
	if err != nil {
		slog.Error("failed to get selected message", "err", err)
		return
	}

	emoji := rp.items[index]
	if err := rp.chatView.state.React(message.ChannelID, message.ID, emoji.APIString()); err != nil {
		slog.Error("failed to react to message", "channel_id", message.ChannelID, "message_id", message.ID, "emoji", emoji.Name, "err", err)
		return
	}

	rp.close()
}

func (rp *reactionPicker) close() {
	rp.chatView.RemoveLayer(reactionPickerLayerName)
	if rp.messagesList != nil {
		rp.messagesList.pendingFullClear = true
	}
	rp.chatView.app.SetFocus(rp.messagesList)
}

func (rp *reactionPicker) previewItem(emoji discord.Emoji) *imageItem {
	if !emoji.ID.IsValid() {
		return nil
	}
	return rp.previewItemByURL(emoji.EmojiURL())
}

func (rp *reactionPicker) previewItemByURL(key string) *imageItem {
	if item, ok := rp.emoteItemByKey[key]; ok {
		item.useKitty = rp.useKitty()
		return item
	}

	item := newImageItem(rp.imageCache, key, reactionPickerEmojiWidth, reactionPickerEmojiHeight, rp.useKitty(), rp.nextKittyID, rp.GetInnerRect, rp.messagesList.scheduleAnimatedRedraw)
	rp.nextKittyID++
	rp.emoteItemByKey[key] = item
	rp.imageCache.Request(key, 0, 0, func() {
		if rp.chatView != nil && rp.chatView.app != nil {
			rp.chatView.app.QueueUpdateDraw(func() {})
		}
	})
	return item
}

func (rp *reactionPicker) ShortHelp() []keybind.Keybind {
	cfg := rp.cfg.Keybinds.Picker
	return []keybind.Keybind{cfg.Up.Keybind, cfg.Down.Keybind, cfg.Select.Keybind, cfg.Cancel.Keybind}
}

func (rp *reactionPicker) FullHelp() [][]keybind.Keybind {
	cfg := rp.cfg.Keybinds.Picker
	return [][]keybind.Keybind{
		{cfg.Up.Keybind, cfg.Down.Keybind, cfg.Top.Keybind, cfg.Bottom.Keybind},
		{cfg.ToggleFocus.Keybind, cfg.Select.Keybind, cfg.Cancel.Keybind},
	}
}

const (
	reactionPickerEmojiWidth  = inlineEmoteWidth
	reactionPickerEmojiHeight = 1
)

func (rp *reactionPicker) useKitty() bool {
	return rp.cfg.InlineImages.Enabled && rp.messagesList != nil && rp.messagesList.useKitty
}

func (rp *reactionPicker) Draw(screen tcell.Screen) {
	if rp.useKitty() && rp.messagesList != nil {
		rp.messagesList.updateCellDimensions(screen)
		rp.prepareKittyItemsForFrame(screen)
	}

	rp.Picker.Draw(screen)
	rp.scanAndDrawEmotes(screen)
}

func (rp *reactionPicker) prepareKittyItemsForFrame(screen tcell.Screen) {
	for _, item := range rp.emoteItemByKey {
		item.useKitty = true
		item.drawnThisFrame = false
		item.pendingPlace = false
		item.unlockRegion(screen)
		// The messages list clears all Kitty images from terminal memory while an
		// overlay is visible. Keep the cached payload, but force a fresh upload and
		// placement for the current frame.
		item.kittyPlaced = false
		item.kittyUploaded = false
		if rp.messagesList.cellW > 0 {
			item.setCellDimensions(rp.messagesList.cellW, rp.messagesList.cellH)
		}
	}
}

func (rp *reactionPicker) AfterDraw(screen tcell.Screen) {
	if !rp.useKitty() {
		return
	}
	tty, ok := screen.Tty()
	if !ok {
		return
	}

	fmt.Fprint(tty, "\x1b7")
	for _, item := range rp.emoteItemByKey {
		item.flushKittyPlace(tty)
	}
	fmt.Fprint(tty, "\x1b8")
}

func (rp *reactionPicker) lineForEmoji(emoji discord.Emoji) tview.Line {
	labelStyle := rp.cfg.Theme.MessagesList.MessageStyle.Style
	if !emoji.ID.IsValid() {
		return tview.Line{
			{Text: emoji.Name, Style: labelStyle},
		}
	}

	return tview.Line{
		{
			Text:  markdown.CustomEmojiText(emoji.Name, rp.cfg.InlineImages.Enabled),
			Style: rp.cfg.Theme.MessagesList.EmojiStyle.Style.Url(emoji.EmojiURL()),
		},
		{
			Text:  " " + emoji.Name,
			Style: labelStyle,
		},
	}
}

func (rp *reactionPicker) scanAndDrawEmotes(screen tcell.Screen) {
	if !rp.cfg.InlineImages.Enabled {
		return
	}

	x, y, w, h := rp.GetInnerRect()
	for row := y; row < y+h; row++ {
		for col := x; col < x+w; col++ {
			_, style, _ := screen.Get(col, row)
			_, url := style.GetUrl()
			if !strings.HasPrefix(url, "https://cdn.discordapp.com/emojis/") {
				continue
			}

			item := rp.previewItemByURL(url)
			item.SetRect(col, row, reactionPickerEmojiWidth, reactionPickerEmojiHeight)
			item.Draw(screen)

			for offset := 1; offset < reactionPickerEmojiWidth && col+offset < x+w; offset++ {
				screen.SetContent(col+offset, row, ' ', nil, tcell.StyleDefault)
			}
			col += reactionPickerEmojiWidth - 1
		}
	}
}
