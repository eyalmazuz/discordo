package chat

import (
	"github.com/eyalmazuz/tview/help"
	"github.com/eyalmazuz/tview/keybind"
)

var _ help.KeyMap = (*Model)(nil)

func (v *Model) ShortHelp() []keybind.Keybind {
	short := make([]keybind.Keybind, 0, 16)
	if active := v.activeKeyMap(); active != nil {
		short = append(short, active.ShortHelp()...)
	}
	short = append(short, v.baseShortHelp()...)
	return short
}

func (v *Model) FullHelp() [][]keybind.Keybind {
	full := make([][]keybind.Keybind, 0, 8)
	if active := v.activeKeyMap(); active != nil {
		full = append(full, active.FullHelp()...)
	}
	full = append(full, v.baseFullHelp()...)
	return full
}

func (v *Model) activeKeyMap() help.KeyMap {
	if v.GetVisible(channelsPickerLayerName) {
		return v.channelsPicker
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

	if v.app == nil {
		return nil
	}

	switch v.app.GetFocus() {
	case v.guildsTree:
		return v.guildsTree
	case v.messagesList:
		return v.messagesList
	case v.messageInput:
		return v.messageInput
	default:
		return nil
	}
}

func (v *Model) baseShortHelp() []keybind.Keybind {
	cfg := v.cfg.Keybinds
	short := []keybind.Keybind{cfg.FocusGuildsTree.Keybind, cfg.FocusMessagesList.Keybind}
	if !v.messageInput.GetDisabled() {
		short = append(short, cfg.FocusMessageInput.Keybind)
	}
	short = append(short, cfg.ToggleGuildsTree.Keybind, cfg.ToggleChannelsPicker.Keybind)
	if v.SelectedChannel() != nil {
		short = append(short, cfg.ToggleMessageSearch.Keybind, cfg.TogglePinnedMessages.Keybind)
	}
	return short
}

func (v *Model) baseFullHelp() [][]keybind.Keybind {
	cfg := v.cfg.Keybinds
	focus := []keybind.Keybind{cfg.FocusGuildsTree.Keybind, cfg.FocusMessagesList.Keybind}
	if !v.messageInput.GetDisabled() {
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
