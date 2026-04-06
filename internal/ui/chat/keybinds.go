package chat

import (
	"github.com/eyalmazuz/tview/help"
	"github.com/eyalmazuz/tview/keybind"
)

var _ help.KeyMap = (*Model)(nil)

func (m *Model) ShortHelp() []keybind.Keybind {
	short := make([]keybind.Keybind, 0, 16)
	if active := m.activeKeyMap(); active != nil {
		short = append(short, active.ShortHelp()...)
	}
	short = append(short, m.baseShortHelp()...)
	return short
}

func (m *Model) FullHelp() [][]keybind.Keybind {
	full := make([][]keybind.Keybind, 0, 8)
	if active := m.activeKeyMap(); active != nil {
		full = append(full, active.FullHelp()...)
	}
	full = append(full, m.baseFullHelp()...)
	return full
}

func (m *Model) activeKeyMap() help.KeyMap {
	if m.GetVisible(channelsPickerLayerName) {
		return m.channelsPicker
	}
	if v.GetVisible(messageSearchLayerName) {
		return v.messageSearch
	}
	if v.GetVisible(pinnedMessagesLayerName) {
		return v.pinnedMessages
	}
	if v.GetVisible(reactionPickerLayerName) {
		return v.messagesList.reactionPicker
	}
	if v.GetVisible(attachmentsListLayerName) {
		return v.messagesList.attachmentsPicker
	}

	switch m.app.Focused() {
	case m.guildsTree:
		return m.guildsTree
	case m.messagesList:
		return m.messagesList
	case m.messageInput:
		return m.messageInput
	default:
		return nil
	}
}

func (m *Model) baseShortHelp() []keybind.Keybind {
	cfg := m.cfg.Keybinds
	short := []keybind.Keybind{cfg.FocusGuildsTree.Keybind, cfg.FocusMessagesList.Keybind}
	if !m.messageInput.GetDisabled() {
		short = append(short, cfg.FocusMessageInput.Keybind)
	}
	short = append(short, cfg.ToggleGuildsTree.Keybind, cfg.ToggleChannelsPicker.Keybind)
	if v.SelectedChannel() != nil {
		short = append(short, cfg.ToggleMessageSearch.Keybind, cfg.TogglePinnedMessages.Keybind)
	}
	return short
}

func (m *Model) baseFullHelp() [][]keybind.Keybind {
	cfg := m.cfg.Keybinds
	focus := []keybind.Keybind{cfg.FocusGuildsTree.Keybind, cfg.FocusMessagesList.Keybind}
	if !m.messageInput.GetDisabled() {
		focus = append(focus, cfg.FocusMessageInput.Keybind)
	}
	toggles := []keybind.Keybind{cfg.ToggleGuildsTree.Keybind, cfg.ToggleChannelsPicker.Keybind}
	if v.SelectedChannel() != nil {
		toggles = append(toggles, cfg.ToggleMessageSearch.Keybind, cfg.TogglePinnedMessages.Keybind)
	}
	return [][]keybind.Keybind{
		focus,
		{cfg.FocusPrevious.Keybind, cfg.FocusNext.Keybind},
		toggles,
		{cfg.Logout.Keybind},
	}
}
