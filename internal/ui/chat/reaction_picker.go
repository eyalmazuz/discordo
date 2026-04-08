package chat

import (
	"fmt"
	"log/slog"
	"net/http"
	"reflect"
	"unsafe"

	"github.com/ayn2op/discordo/internal/config"
	httpkg "github.com/ayn2op/discordo/internal/http"
	imgpkg "github.com/ayn2op/discordo/internal/image"
	"github.com/ayn2op/discordo/internal/markdown"
	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/eyalmazuz/tview"
	"github.com/eyalmazuz/tview/help"
	"github.com/eyalmazuz/tview/keybind"
	"github.com/eyalmazuz/tview/list"
	"github.com/eyalmazuz/tview/picker"
	"github.com/gdamore/tcell/v3"
)

type reactionPicker struct {
	*picker.Model
	cfg          *config.Config
	chatView     *Model
	messagesList *messagesList

	items []discord.Emoji

	listModel      *list.Model
	imageCache     *imgpkg.Cache
	emoteItemByKey map[string]*imageItem
	nextKittyID    uint32
	useKitty       bool
	cellW          int
	cellH          int
	pendingDeletes []uint32
}

var _ help.KeyMap = (*reactionPicker)(nil)

func newReactionPicker(cfg *config.Config, chatView *Model, messagesList *messagesList) *reactionPicker {
	rp := &reactionPicker{
		Model:          picker.NewModel(),
		cfg:            cfg,
		chatView:       chatView,
		messagesList:   messagesList,
		imageCache:     imgpkg.NewCache(&http.Client{Transport: httpkg.NewTransport()}),
		emoteItemByKey: make(map[string]*imageItem),
		nextKittyID:    200000,
		useKitty:       resolveKittyMode(cfg.InlineImages.Renderer),
	}
	ConfigurePicker(rp.Model, cfg, "Reactions")
	rp.listModel = pickerListModel(rp.Model)
	if rp.listModel != nil {
		rp.listModel.SetScrollBarVisibility(list.ScrollBarVisibilityNever)
	}
	rp.refreshPreviewBuilder()
	return rp
}

func (rp *reactionPicker) SetItems(items []discord.Emoji) {
	rp.items = append(rp.items[:0], items...)
	rp.rebuildPickerItems()
}

func (rp *reactionPicker) rebuildPickerItems() {
	var pItems picker.Items
	for i, emoji := range rp.items {
		text := emoji.Name
		filterText := emoji.Name
		if emoji.ID == 0 {
			s := markdown.GetShortcode(emoji.Name)
			if s != "" {
				text = ":" + s + ":"
				filterText += " " + s
			}
		}
		pItems = append(pItems, picker.Item{
			Text:       text,
			FilterText: filterText,
			Reference:  i,
		})
	}
	rp.Model.SetItems(pItems)
	rp.refreshPreviewBuilder()
}

func (rp *reactionPicker) Update(msg tview.Msg) tview.Cmd {
	switch msg := msg.(type) {
	case *picker.SelectedMsg:
		return rp.onSelected(msg.Item)
	case *picker.CancelMsg:
		return rp.close()
	}
	cmd := rp.Model.Update(msg)
	rp.refreshPreviewBuilder()
	return cmd
}

func (rp *reactionPicker) onSelected(item picker.Item) tview.Cmd {
	index, ok := item.Reference.(int)
	if !ok || index < 0 || index >= len(rp.items) {
		return nil
	}

	message, err := rp.messagesList.selectedMessage()
	if err != nil {
		slog.Error("failed to get selected message", "err", err)
		return nil
	}

	emoji := rp.items[index]
	channelID := message.ChannelID
	messageID := message.ID
	apiString := emoji.APIString()
	emojiName := emoji.Name

	closeCmd := rp.close()

	reactCmd := func() tview.Msg {
		if err := rp.chatView.state.React(channelID, messageID, apiString); err != nil {
			slog.Error("failed to react to message", "channel_id", channelID, "message_id", messageID, "emoji", emojiName, "err", err)
		}
		return nil
	}
	return tview.Batch(closeCmd, reactCmd)
}

func (rp *reactionPicker) close() tview.Cmd {
	for _, item := range rp.emoteItemByKey {
		if item.kittyPlaced {
			rp.pendingDeletes = append(rp.pendingDeletes, item.kittyID)
		}
	}
	clear(rp.emoteItemByKey)
	rp.chatView.RemoveLayer(reactionPickerLayerName)
	if rp.messagesList != nil {
		rp.messagesList.kittyNeedsFullClear = true
	}
	return tview.SetFocus(rp.messagesList)
}

func (rp *reactionPicker) Draw(screen tcell.Screen) {
	if rp.cfg.InlineImages.Enabled && rp.useKitty {
		rp.updateCellDimensions(screen)
	}
	for _, item := range rp.emoteItemByKey {
		item.drawnThisFrame = false
	}
	rp.Model.Draw(screen)
}

func (rp *reactionPicker) AfterDraw(screen tcell.Screen) {
	if !rp.cfg.InlineImages.Enabled || !rp.useKitty {
		return
	}
	tty, ok := screen.Tty()
	if !ok {
		return
	}

	for key, item := range rp.emoteItemByKey {
		if !item.drawnThisFrame {
			if item.kittyPlaced {
				rp.pendingDeletes = append(rp.pendingDeletes, item.kittyID)
			}
			delete(rp.emoteItemByKey, key)
		}
	}

	fmt.Fprint(tty, "\x1b7")
	for _, id := range rp.pendingDeletes {
		_ = imgpkg.DeleteKittyByID(tty, id)
	}
	rp.pendingDeletes = rp.pendingDeletes[:0]
	for _, item := range rp.emoteItemByKey {
		item.flushKittyPlace(tty)
	}
	fmt.Fprint(tty, "\x1b8")
}

func (rp *reactionPicker) refreshPreviewBuilder() {
	if rp.listModel == nil {
		return
	}
	// Snapshot filtered items once per refresh instead of copying via
	// reflection on every builder call (once per visible row per frame).
	filtered := pickerFilteredItems(rp.Model)
	rp.listModel.SetBuilder(func(index int, cursor int) list.Item {
		if index < 0 || index >= len(filtered) {
			return nil
		}
		ref, ok := filtered[index].Reference.(int)
		if !ok || ref < 0 || ref >= len(rp.items) {
			return nil
		}
		emoji := rp.items[ref]
		style := tcell.StyleDefault
		if index == cursor {
			style = style.Reverse(true)
		}
		return &reactionPickerRowItem{
			Box:      tview.NewBox(),
			style:    style,
			text:     emojiDisplayText(emoji),
			preview:  rp.previewItemFor(ref, emoji),
			useKitty: rp.useKitty,
		}
		})
		}

		func (rp *reactionPicker) previewItemFor(index int, emoji discord.Emoji) *imageItem {

	if !rp.cfg.InlineImages.Enabled {
		return nil
	}
	var url string
	if emoji.ID != 0 {
		url = emoji.EmojiURL()
	} else {
		url = markdown.TwemojiURL(emoji.Name)
	}
	if url == "" {
		return nil
	}
	key := fmt.Sprintf("picker:%s", url)
	if item, ok := rp.emoteItemByKey[key]; ok {
		return item
	}
	kittyID := rp.nextKittyID
	rp.nextKittyID++
	item := newImageItem(rp.imageCache, url, inlineEmoteWidth, 1, rp.useKitty, kittyID, nil, nil)
	item.lockKittyRegion = false
	if rp.cellW > 0 {
		item.setCellDimensions(rp.cellW, rp.cellH)
	}
	rp.emoteItemByKey[key] = item
	rp.imageCache.Request(url, 0, 0, func() {
		if rp.chatView != nil && rp.chatView.app != nil {
			triggerRedraw(rp.chatView.app)
		}
	})
	return item
}

func emojiDisplayText(emoji discord.Emoji) string {
	if emoji.ID != 0 {
		text := ":" + emoji.Name + ":"
		if emoji.Animated {
			text += " [animated]"
		}
		return text
	}
	s := markdown.GetShortcode(emoji.Name)
	if s != "" {
		return ":" + s + ":"
	}
	return emoji.Name
}

func (rp *reactionPicker) updateCellDimensions(screen tcell.Screen) {
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
	if cw != rp.cellW || ch != rp.cellH {
		rp.cellW = cw
		rp.cellH = ch
		for _, item := range rp.emoteItemByKey {
			item.setCellDimensions(cw, ch)
		}
	}
}

func pickerListModel(model *picker.Model) *list.Model {
	modelValue := reflect.ValueOf(model).Elem()
	field := modelValue.FieldByName("list")
	if !field.IsValid() || field.IsNil() {
		return nil
	}
	return reflect.NewAt(field.Type(), unsafe.Pointer(field.UnsafeAddr())).Elem().Interface().(*list.Model)
}

func pickerFilteredItems(model *picker.Model) picker.Items {
	modelValue := reflect.ValueOf(model).Elem()
	field := modelValue.FieldByName("filtered")
	if !field.IsValid() {
		return nil
	}
	items := reflect.NewAt(field.Type(), unsafe.Pointer(field.UnsafeAddr())).Elem().Interface().(picker.Items)
	return append(picker.Items(nil), items...)
}

type reactionPickerRowItem struct {
	*tview.Box
	style    tcell.Style
	text     string
	preview  *imageItem
	useKitty bool
}

func (i *reactionPickerRowItem) Height(width int) int { return 1 }

func (i *reactionPickerRowItem) Draw(screen tcell.Screen) {
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
	for _, r := range i.text {
		if col >= x+w {
			break
		}
		screen.SetContent(col, y, r, nil, i.style)
		col++
	}
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
