package chat

import (
	"fmt"
	"log/slog"
	"strings"
	"sync/atomic"

	"github.com/ayn2op/discordo/internal/config"
	"github.com/ayn2op/discordo/internal/ui"
	"github.com/diamondburned/arikawa/v3/api"
	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/eyalmazuz/tview"
	"github.com/eyalmazuz/tview/flex"
	"github.com/eyalmazuz/tview/help"
	"github.com/eyalmazuz/tview/keybind"
	"github.com/eyalmazuz/tview/list"
	"github.com/gdamore/tcell/v3"
)

type messageSearchResult struct {
	Message discord.Message
}

type messageSearchPopup struct {
	*flex.Model
	cfg          *config.Config
	chatView     *Model
	messagesList *messagesList
	input        *tview.InputField
	list         *list.Model

	channel           discord.Channel
	previousFocus     tview.Model
	results           []messageSearchResult
	status            string
	statusStyle       tcell.Style
	lastSubmitted     string
	activeSearchToken atomic.Uint64

	searchMessages  func(channel discord.Channel, query string) ([]messageSearchResult, error)
	jumpToMessage   func(channel discord.Channel, messageID discord.MessageID) error
	queueUpdateDraw func(func())
}

var _ help.KeyMap = (*messageSearchPopup)(nil)

func newMessageSearchPopup(cfg *config.Config, chatView *Model, messagesList *messagesList) *messageSearchPopup {
	sp := &messageSearchPopup{
		Model:        flex.NewModel(),
		cfg:          cfg,
		chatView:     chatView,
		messagesList: messagesList,
		input:        tview.NewInputField(),
		list:         list.NewModel(),
		statusStyle:  tcell.StyleDefault.Dim(true),
	}

	var borderSet tview.BorderSet
	borderSet.Bottom = tview.BoxDrawingsLightHorizontal
	borderSet.BottomLeft = borderSet.Bottom
	borderSet.BottomRight = borderSet.Bottom

	sp.input.
		SetLabel("> ").
		SetChangedFunc(sp.onInputChanged)
	sp.input.SetBorders(tview.BordersBottom)
	sp.input.SetBorderSet(borderSet)
	sp.input.SetBorderStyle(tcell.StyleDefault.Dim(true))

	sp.list.SetBuilder(sp.buildItem)
	sp.list.SetScrollBarVisibility(cfg.Theme.ScrollBar.Visibility.ScrollBarVisibility)
	sp.list.SetScrollBar(tview.NewScrollBar().
		SetTrackStyle(cfg.Theme.ScrollBar.TrackStyle.Style).
		SetThumbStyle(cfg.Theme.ScrollBar.ThumbStyle.Style).
		SetGlyphSet(cfg.Theme.ScrollBar.GlyphSet.GlyphSet))

	sp.SetDirection(flex.DirectionRow).
		AddItem(sp.input, 2, 0, true).
		AddItem(sp.list, 0, 1, false)

	sp.Box = ui.ConfigureBox(sp.Box, &cfg.Theme)
	sp.
		SetBlurFunc(nil).
		SetFocusFunc(nil).
		SetBorderSet(cfg.Theme.Border.ActiveSet.BorderSet).
		SetBorderStyle(cfg.Theme.Border.ActiveStyle.Style).
		SetTitleStyle(cfg.Theme.Title.ActiveStyle.Style).
		SetFooterStyle(cfg.Theme.Footer.ActiveStyle.Style)

	sp.resetPrompt()
	return sp
}

func (sp *messageSearchPopup) Prepare(channel discord.Channel, previousFocus tview.Model) {
	sp.channel = channel
	sp.previousFocus = previousFocus
	sp.results = nil
	sp.lastSubmitted = ""
	sp.input.SetText("")
	sp.SetTitle("Search in " + ui.ChannelToString(channel, sp.cfg.Icons, sp.chatView.state))
	sp.SetFooter("Enter search  Tab focus")
	sp.resetPrompt()
}

func (sp *messageSearchPopup) FocusInput() {
	if sp.chatView != nil && sp.chatView.app != nil {
		sendFocus(sp.chatView.app, sp.input)
	}
}

func (sp *messageSearchPopup) ShortHelp() []keybind.Keybind {
	cfg := sp.cfg.Keybinds.Picker
	return []keybind.Keybind{cfg.Up.Keybind, cfg.Down.Keybind, cfg.Select.Keybind, cfg.Cancel.Keybind}
}

func (sp *messageSearchPopup) FullHelp() [][]keybind.Keybind {
	cfg := sp.cfg.Keybinds.Picker
	return [][]keybind.Keybind{
		{cfg.Up.Keybind, cfg.Down.Keybind, cfg.Top.Keybind, cfg.Bottom.Keybind},
		{cfg.ToggleFocus.Keybind, cfg.Select.Keybind, cfg.Cancel.Keybind},
	}
}

func (sp *messageSearchPopup) Update(msg tview.Msg) tview.Cmd {
	switch msg := msg.(type) {
	case *tview.KeyMsg:
		keys := sp.cfg.Keybinds.Picker

		switch {
		case keybind.Matches(msg, keys.ToggleFocus.Keybind):
			if sp.input.HasFocus() {
				sendFocus(sp.chatView.app, sp.list)
			} else {
				sendFocus(sp.chatView.app, sp.input)
			}
			return nil
		case keybind.Matches(msg, keys.Up.Keybind):
			sp.list.Update(tcell.NewEventKey(tcell.KeyUp, "", tcell.ModNone))
			return nil
		case keybind.Matches(msg, keys.Down.Keybind):
			sp.list.Update(tcell.NewEventKey(tcell.KeyDown, "", tcell.ModNone))
			return nil
		case keybind.Matches(msg, keys.Top.Keybind):
			sp.list.Update(tcell.NewEventKey(tcell.KeyHome, "", tcell.ModNone))
			return nil
		case keybind.Matches(msg, keys.Bottom.Keybind):
			sp.list.Update(tcell.NewEventKey(tcell.KeyEnd, "", tcell.ModNone))
			return nil
		case keybind.Matches(msg, keys.Select.Keybind):
			if sp.input.HasFocus() {
				sp.search()
			} else {
				sp.selectCurrent()
			}
			return nil
		case keybind.Matches(msg, keys.Cancel.Keybind):
			sp.close(sp.previousFocus)
			return nil
		}

		if sp.list.HasFocus() && msg.Key() == tcell.KeyRune {
			switch msg.Str() {
			case "j":
				sp.list.Update(tcell.NewEventKey(tcell.KeyDown, "", tcell.ModNone))
				return nil
			case "k":
				sp.list.Update(tcell.NewEventKey(tcell.KeyUp, "", tcell.ModNone))
				return nil
			case "g":
				sp.list.Update(tcell.NewEventKey(tcell.KeyHome, "", tcell.ModNone))
				return nil
			case "G":
				sp.list.Update(tcell.NewEventKey(tcell.KeyEnd, "", tcell.ModNone))
				return nil
			}
		}

		return sp.Model.Update(msg)
	}

	return sp.Model.Update(msg)
}

func (sp *messageSearchPopup) search() {
	query := strings.TrimSpace(sp.input.GetText())
	if query == "" {
		sp.resetPrompt()
		return
	}

	channel := sp.channel
	if !channel.ID.IsValid() {
		return
	}

	token := sp.activeSearchToken.Add(1)
	sp.lastSubmitted = query
	sp.results = nil
	sp.setStatus("Searching...", tcell.StyleDefault.Dim(true))

	go func(channel discord.Channel, query string, token uint64) {
		results, err := sp.fetchSearchResults(channel, query)
		sp.enqueueUpdateDraw(func() {
			if token != sp.activeSearchToken.Load() {
				return
			}
			if strings.TrimSpace(sp.input.GetText()) != query {
				return
			}
			if err != nil {
				slog.Error("failed to search messages", "channel_id", channel.ID, "query", query, "err", err)
				sp.results = nil
				sp.setStatus("Search failed", tcell.StyleDefault.Foreground(tcell.ColorRed))
				return
			}
			sp.setResults(results)
		})
	}(channel, query, token)
}

func (sp *messageSearchPopup) fetchSearchResults(channel discord.Channel, query string) ([]messageSearchResult, error) {
	if sp.searchMessages != nil {
		return sp.searchMessages(channel, query)
	}

	seen := make(map[discord.MessageID]struct{})
	results := make([]messageSearchResult, 0, 16)
	offset := uint(0)
	queryLower := strings.ToLower(query)

	for {
		data := api.SearchData{
			Offset:      offset,
			Content:     query,
			ChannelID:   channel.ID,
			IncludeNSFW: true,
		}

		var (
			resp api.SearchResponse
			err  error
		)
		if channel.GuildID.IsValid() {
			resp, err = sp.chatView.state.Search(channel.GuildID, data)
		} else {
			resp, err = sp.chatView.state.SearchDirectMessages(data)
		}
		if err != nil {
			return nil, err
		}
		if len(resp.Messages) == 0 {
			break
		}

		for _, group := range resp.Messages {
			message, ok := pickSearchResultMessage(group, channel, queryLower)
			if !ok {
				continue
			}
			if _, ok := seen[message.ID]; ok {
				continue
			}
			message.GuildID = channel.GuildID
			seen[message.ID] = struct{}{}
			results = append(results, messageSearchResult{Message: message})
		}

		offset += uint(len(resp.Messages))
		if resp.TotalResults > 0 && offset >= resp.TotalResults {
			break
		}
	}

	return results, nil
}

func pickSearchResultMessage(group []discord.Message, channel discord.Channel, queryLower string) (discord.Message, bool) {
	var fallback *discord.Message
	for i := range group {
		message := group[i]
		if !message.ID.IsValid() || message.ChannelID != channel.ID {
			continue
		}
		message.GuildID = channel.GuildID
		if fallback == nil {
			fallback = &message
		}
		if queryLower != "" && strings.Contains(strings.ToLower(message.Content), queryLower) {
			return message, true
		}
	}
	if fallback == nil {
		return discord.Message{}, false
	}
	return *fallback, true
}

func (sp *messageSearchPopup) selectCurrent() {
	cursor := sp.list.Cursor()
	if cursor < 0 || cursor >= len(sp.results) {
		return
	}

	result := sp.results[cursor]
	jump := sp.jumpToMessage
	if jump == nil {
		jump = func(channel discord.Channel, messageID discord.MessageID) error {
			return sp.messagesList.jumpToMessage(channel, messageID)
		}
	}
	if err := jump(sp.channel, result.Message.ID); err != nil {
		slog.Error("failed to jump to message", "channel_id", sp.channel.ID, "message_id", result.Message.ID, "err", err)
		return
	}

	sp.close(sp.messagesList)
}

func (sp *messageSearchPopup) close(nextFocus tview.Model) {
	sp.activeSearchToken.Add(1)
	if sp.chatView != nil && sp.chatView.HasLayer(messageSearchLayerName) {
		sp.chatView.RemoveLayer(messageSearchLayerName)
	}
	if sp.chatView != nil && sp.chatView.app != nil && nextFocus != nil {
		sendFocus(sp.chatView.app, nextFocus)
	}
	sp.previousFocus = nil
}

func (sp *messageSearchPopup) enqueueUpdateDraw(f func()) {
	if sp.queueUpdateDraw != nil {
		sp.queueUpdateDraw(f)
		return
	}
	if sp.chatView == nil || sp.chatView.app == nil {
		return
	}
	f()
	triggerRedraw(sp.chatView.app)
}

func (sp *messageSearchPopup) onInputChanged(text string) {
	query := strings.TrimSpace(text)
	if query == sp.lastSubmitted {
		return
	}
	sp.results = nil
	if query == "" {
		sp.resetPrompt()
		return
	}
	sp.setStatus("Press Enter to search this channel", tcell.StyleDefault.Dim(true))
}

func (sp *messageSearchPopup) setResults(results []messageSearchResult) {
	sp.results = results
	switch len(results) {
	case 0:
		sp.setStatus("No results found", tcell.StyleDefault.Dim(true))
	default:
		sp.status = ""
		sp.statusStyle = tcell.StyleDefault
		sp.list.SetCursor(0)
		sp.SetFooter(fmt.Sprintf("%d result(s)  Tab focus", len(results)))
	}
}

func (sp *messageSearchPopup) resetPrompt() {
	sp.lastSubmitted = ""
	sp.setStatus("Type a query and press Enter", tcell.StyleDefault.Dim(true))
}

func (sp *messageSearchPopup) setStatus(text string, style tcell.Style) {
	sp.status = text
	sp.statusStyle = style
	sp.list.SetCursor(-1)
	sp.SetFooter("Enter search  Tab focus")
}

func (sp *messageSearchPopup) buildItem(index int, cursor int) list.Item {
	if len(sp.results) == 0 {
		if sp.status == "" || index != 0 {
			return nil
		}
		return tview.NewTextView().
			SetScrollable(false).
			SetWrap(false).
			SetWordWrap(false).
			SetLines([]tview.Line{{{Text: sp.status, Style: sp.statusStyle}}})
	}

	if index < 0 || index >= len(sp.results) {
		return nil
	}

	line := sp.lineForResult(sp.results[index])
	if index == cursor {
		line = reverseSearchLine(line)
	}

	return tview.NewTextView().
		SetScrollable(false).
		SetWrap(false).
		SetWordWrap(false).
		SetLines([]tview.Line{line})
}

func (sp *messageSearchPopup) lineForResult(result messageSearchResult) tview.Line {
	message := result.Message
	preview := compactMessagePreview(message)
	baseStyle := sp.cfg.Theme.MessagesList.MessageStyle.Style
	return tview.Line{
		{
			Text:  sp.messagesList.formatTimestamp(message.Timestamp) + " ",
			Style: baseStyle.Dim(true),
		},
		{
			Text:  message.Author.DisplayOrUsername() + ": ",
			Style: baseStyle.Bold(true),
		},
		{
			Text:  preview,
			Style: baseStyle,
		},
	}
}

func compactMessagePreview(message discord.Message) string {
	preview := strings.Join(strings.Fields(message.Content), " ")
	if preview != "" {
		return preview
	}
	switch {
	case len(message.Stickers) > 0:
		return "[sticker]"
	case len(message.Attachments) > 0:
		return "[attachment]"
	case len(message.Embeds) > 0:
		return "[embed]"
	default:
		return "[no text]"
	}
}

func reverseSearchLine(line tview.Line) tview.Line {
	cloned := make(tview.Line, len(line))
	for i, segment := range line {
		cloned[i] = segment
		cloned[i].Style = cloned[i].Style.Reverse(true)
	}
	return cloned
}
