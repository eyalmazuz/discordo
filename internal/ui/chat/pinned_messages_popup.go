package chat

import (
	"errors"
	"fmt"
	"log/slog"

	"github.com/ayn2op/discordo/internal/config"
	"github.com/ayn2op/discordo/internal/ui"
	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/eyalmazuz/tview"
	"github.com/eyalmazuz/tview/help"
	"github.com/eyalmazuz/tview/keybind"
	"github.com/eyalmazuz/tview/list"
	"github.com/gdamore/tcell/v3"
)

const (
	unpinConfirmPrompt = "Do you want to remove this pin"
	unpinConfirmHelper = "please verify again that this is the message you want to remove from pins"
)

type pinnedMessagesPopup struct {
	*list.Model
	cfg          *config.Config
	chat     *Model
	messagesList *messagesList

	channel       discord.Channel
	previousFocus tview.Model

	pins        []discord.Message
	status      string
	statusStyle tcell.Style

	fetchPinnedMessages func(channel discord.Channel) ([]discord.Message, error)
	jumpToMessage       func(channel discord.Channel, messageID discord.MessageID) error
	unpinMessage        func(channel discord.Channel, messageID discord.MessageID) error
}

var _ help.KeyMap = (*pinnedMessagesPopup)(nil)

func newPinnedMessagesPopup(cfg *config.Config, chat *Model, messagesList *messagesList) *pinnedMessagesPopup {
	pp := &pinnedMessagesPopup{
		Model:        list.NewModel(),
		cfg:          cfg,
		chat:     chat,
		messagesList: messagesList,
		statusStyle:  tcell.StyleDefault.Dim(true),
	}

	pp.Box = ui.ConfigureBox(pp.Box, &cfg.Theme)
	pp.
		SetBlurFunc(nil).
		SetFocusFunc(nil).
		SetBorderSet(cfg.Theme.Border.ActiveSet.BorderSet).
		SetBorderStyle(cfg.Theme.Border.ActiveStyle.Style).
		SetTitleStyle(cfg.Theme.Title.ActiveStyle.Style).
		SetFooterStyle(cfg.Theme.Footer.ActiveStyle.Style)

	pp.SetBuilder(pp.buildItem)
	pp.SetScrollBarVisibility(cfg.Theme.ScrollBar.Visibility.ScrollBarVisibility)
	pp.SetScrollBar(tview.NewScrollBar().
		SetTrackStyle(cfg.Theme.ScrollBar.TrackStyle.Style).
		SetThumbStyle(cfg.Theme.ScrollBar.ThumbStyle.Style).
		SetGlyphSet(cfg.Theme.ScrollBar.GlyphSet.GlyphSet))
	pp.setStatus("No pinned messages", tcell.StyleDefault.Dim(true))

	return pp
}

func (pp *pinnedMessagesPopup) Prepare(channel discord.Channel, previousFocus tview.Model) {
	pp.channel = channel
	pp.previousFocus = previousFocus
	pp.SetTitle("Pins in " + ui.ChannelToString(channel, pp.cfg.Icons, pp.chat.state))
	pp.refresh()
}

func (pp *pinnedMessagesPopup) FocusList() {
	if pp.chat != nil && pp.chat.app != nil {
		sendFocus(pp.chat.app, pp)
	}
}

func (pp *pinnedMessagesPopup) ShortHelp() []keybind.Keybind {
	cfg := pp.cfg.Keybinds.Picker
	return []keybind.Keybind{
		cfg.Up.Keybind,
		cfg.Down.Keybind,
		cfg.Select.Keybind,
		keybind.NewKeybind(keybind.WithKeys("d"), keybind.WithHelp("d", "unpin")),
		cfg.Cancel.Keybind,
	}
}

func (pp *pinnedMessagesPopup) FullHelp() [][]keybind.Keybind {
	cfg := pp.cfg.Keybinds.Picker
	return [][]keybind.Keybind{
		{cfg.Up.Keybind, cfg.Down.Keybind, cfg.Top.Keybind, cfg.Bottom.Keybind},
		{
			cfg.Select.Keybind,
			keybind.NewKeybind(keybind.WithKeys("d"), keybind.WithHelp("d", "unpin")),
			keybind.NewKeybind(keybind.WithKeys("D"), keybind.WithHelp("D", "force")),
			cfg.Cancel.Keybind,
		},
	}
}

func (pp *pinnedMessagesPopup) Update(msg tview.Msg) tview.Cmd {
	switch msg := msg.(type) {
	case *tview.KeyMsg:
		keys := pp.cfg.Keybinds.Picker

		switch {
		case keybind.Matches(msg, keys.Up.Keybind):
			pp.Model.Update(tcell.NewEventKey(tcell.KeyUp, "", tcell.ModNone))
			return nil
		case keybind.Matches(msg, keys.Down.Keybind):
			pp.Model.Update(tcell.NewEventKey(tcell.KeyDown, "", tcell.ModNone))
			return nil
		case keybind.Matches(msg, keys.Top.Keybind):
			pp.Model.Update(tcell.NewEventKey(tcell.KeyHome, "", tcell.ModNone))
			return nil
		case keybind.Matches(msg, keys.Bottom.Keybind):
			pp.Model.Update(tcell.NewEventKey(tcell.KeyEnd, "", tcell.ModNone))
			return nil
		case keybind.Matches(msg, keys.Select.Keybind):
			pp.selectCurrent()
			return nil
		case keybind.Matches(msg, keys.Cancel.Keybind):
			pp.close(pp.previousFocus)
			return nil
		}

		if msg.Key() == tcell.KeyRune {
			switch msg.Str() {
			case "j":
				pp.Model.Update(tcell.NewEventKey(tcell.KeyDown, "", tcell.ModNone))
				return nil
			case "k":
				pp.Model.Update(tcell.NewEventKey(tcell.KeyUp, "", tcell.ModNone))
				return nil
			case "g":
				pp.Model.Update(tcell.NewEventKey(tcell.KeyHome, "", tcell.ModNone))
				return nil
			case "G":
				pp.Model.Update(tcell.NewEventKey(tcell.KeyEnd, "", tcell.ModNone))
				return nil
			case "d":
				pp.confirmUnpin()
				return nil
			case "D":
				pp.unpinCurrent()
				return nil
			}
		}

		if msg.Key() == tcell.KeyEnter {
			pp.selectCurrent()
			return nil
		}
	}

	return pp.Model.Update(msg)
}

func (pp *pinnedMessagesPopup) refresh() {
	pins, err := pp.fetchPins(pp.channel)
	if err != nil {
		slog.Error("failed to fetch pinned messages", "channel_id", pp.channel.ID, "err", err)
		pp.pins = nil
		pp.setStatus("Failed to load pinned messages", tcell.StyleDefault.Foreground(tcell.ColorRed))
		return
	}

	if pp.channel.GuildID.IsValid() && len(pins) > 0 {
		pp.messagesList.requestGuildMembers(pp.channel.GuildID, pins)
	}

	pp.setPins(pins)
}

func (pp *pinnedMessagesPopup) fetchPins(channel discord.Channel) ([]discord.Message, error) {
	if pp.fetchPinnedMessages != nil {
		return pp.fetchPinnedMessages(channel)
	}
	return pp.chat.state.State.PinnedMessages(channel.ID)
}

func (pp *pinnedMessagesPopup) setPins(pins []discord.Message) {
	pp.pins = pins
	if len(pins) == 0 {
		pp.setStatus("No pinned messages", tcell.StyleDefault.Dim(true))
		return
	}

	pp.status = ""
	pp.statusStyle = tcell.StyleDefault
	pp.SetCursor(0)
	pp.SetFooter(fmt.Sprintf("%d pin(s)  Enter jump  d unpin  D force", len(pins)))
}

func (pp *pinnedMessagesPopup) setStatus(text string, style tcell.Style) {
	pp.status = text
	pp.statusStyle = style
	pp.SetCursor(-1)
	pp.SetFooter("Enter jump  d unpin  D force")
}

func (pp *pinnedMessagesPopup) buildItem(index int, cursor int) list.Item {
	if len(pp.pins) == 0 {
		if pp.status == "" || index != 0 {
			return nil
		}

		return tview.NewTextView().
			SetScrollable(false).
			SetWrap(false).
			SetWordWrap(false).
			SetLines([]tview.Line{{{Text: pp.status, Style: pp.statusStyle}}})
	}

	if index < 0 || index >= len(pp.pins) {
		return nil
	}

	line := pp.lineForPinnedMessage(pp.pins[index])
	if index == cursor {
		line = reverseSearchLine(line)
	}

	return tview.NewTextView().
		SetScrollable(false).
		SetWrap(false).
		SetWordWrap(false).
		SetLines([]tview.Line{line})
}

func (pp *pinnedMessagesPopup) lineForPinnedMessage(message discord.Message) tview.Line {
	baseStyle := pp.cfg.Theme.MessagesList.MessageStyle.Style
	return tview.Line{
		{
			Text:  pp.messagesList.formatTimestamp(message.Timestamp) + " ",
			Style: baseStyle.Dim(true),
		},
		{
			Text:  message.Author.DisplayOrUsername() + ": ",
			Style: baseStyle.Bold(true),
		},
		{
			Text:  compactMessagePreview(message),
			Style: baseStyle,
		},
	}
}

func (pp *pinnedMessagesPopup) selectedPin() (*discord.Message, error) {
	if len(pp.pins) == 0 {
		return nil, errors.New("no pinned messages available")
	}

	cursor := pp.Cursor()
	if cursor < 0 || cursor >= len(pp.pins) {
		return nil, errors.New("no pinned message is currently selected")
	}

	return &pp.pins[cursor], nil
}

func (pp *pinnedMessagesPopup) selectCurrent() {
	message, err := pp.selectedPin()
	if err != nil {
		return
	}

	jump := pp.jumpToMessage
	if jump == nil {
		jump = func(channel discord.Channel, messageID discord.MessageID) error {
			return pp.messagesList.jumpToMessage(channel, messageID)
		}
	}
	if err := jump(pp.channel, message.ID); err != nil {
		slog.Error("failed to jump to pinned message", "channel_id", pp.channel.ID, "message_id", message.ID, "err", err)
		return
	}

	pp.close(pp.messagesList)
}

func (pp *pinnedMessagesPopup) confirmUnpin() {
	message, err := pp.selectedPin()
	if err != nil {
		return
	}
	if !pp.messagesList.canManagePins() {
		slog.Error("failed to unpin message; missing relevant permissions", "channel_id", pp.channel.ID, "message_id", message.ID)
		return
	}

	pp.chat.showMessageConfirmDialog(
		unpinConfirmPrompt,
		unpinConfirmHelper,
		pp.messagesList.renderMessage(*message, pp.cfg.Theme.MessagesList.SelectedMessageStyle.Style, false),
		func(label string) {
			if label == "yes" {
				pp.unpinCurrent()
			}
		},
	)
}

func (pp *pinnedMessagesPopup) unpinCurrent() {
	message, err := pp.selectedPin()
	if err != nil {
		return
	}
	if !pp.messagesList.canManagePins() {
		slog.Error("failed to unpin message; missing relevant permissions", "channel_id", pp.channel.ID, "message_id", message.ID)
		return
	}

	unpin := pp.unpinMessage
	if unpin == nil {
		unpin = func(channel discord.Channel, messageID discord.MessageID) error {
			return unpinMessageFunc(pp.chat.state.State, channel.ID, messageID, "")
		}
	}
	if err := unpin(pp.channel, message.ID); err != nil {
		slog.Error("failed to unpin message", "channel_id", pp.channel.ID, "message_id", message.ID, "err", err)
		return
	}

	pp.messagesList.setMessagePinned(pp.channel.ID, message.ID, false)

	cursor := pp.Cursor()
	pp.pins = append(pp.pins[:cursor], pp.pins[cursor+1:]...)
	if len(pp.pins) == 0 {
		pp.setStatus("No pinned messages", tcell.StyleDefault.Dim(true))
		return
	}
	if cursor >= len(pp.pins) {
		cursor = len(pp.pins) - 1
	}
	pp.status = ""
	pp.statusStyle = tcell.StyleDefault
	pp.SetCursor(cursor)
	pp.SetFooter(fmt.Sprintf("%d pin(s)  Enter jump  d unpin  D force", len(pp.pins)))
}

func (pp *pinnedMessagesPopup) close(nextFocus tview.Model) {
	if pp.chat != nil && pp.chat.HasLayer(pinnedMessagesLayerName) {
		pp.chat.RemoveLayer(pinnedMessagesLayerName)
	}
	if pp.chat != nil && pp.chat.app != nil && nextFocus != nil {
		sendFocus(pp.chat.app, nextFocus)
	}
	pp.previousFocus = nil
}
