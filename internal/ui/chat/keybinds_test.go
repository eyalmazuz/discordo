package chat

import (
	"testing"

	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/eyalmazuz/tview"
	"github.com/eyalmazuz/tview/keybind"
	"github.com/eyalmazuz/tview/layers"
)

func containsKeybind(bindings []keybind.Keybind, want keybind.Keybind) bool {
	for _, binding := range bindings {
		if binding.Help() == want.Help() {
			return true
		}
	}
	return false
}

func containsKeybindGroup(groups [][]keybind.Keybind, want keybind.Keybind) bool {
	for _, group := range groups {
		if containsKeybind(group, want) {
			return true
		}
	}
	return false
}

func TestModelActiveKeyMap(t *testing.T) {
	m := newTestModel()

	if got := (&Model{Layers: layers.New()}).activeKeyMap(); got != nil {
		t.Fatalf("expected nil active key map without app, got %T", got)
	}

	setFocusForTest(m.app, m.guildsTree)
	if got := m.activeKeyMap(); got != m.guildsTree {
		t.Fatalf("expected guildsTree key map, got %T", got)
	}

	setFocusForTest(m.app, m.messagesList)
	if got := m.activeKeyMap(); got != m.messagesList {
		t.Fatalf("expected messagesList key map, got %T", got)
	}

	m.messageInput.SetDisabled(false)
	setFocusForTest(m.app, m.messageInput)
	if got := m.activeKeyMap(); got != m.messageInput {
		t.Fatalf("expected messageInput key map, got %T", got)
	}

	m.AddLayer(m.channelsPicker, layers.WithName(channelsPickerLayerName), layers.WithVisible(true))
	if got := m.activeKeyMap(); got != m.channelsPicker {
		t.Fatalf("expected channels picker key map, got %T", got)
	}
	m.RemoveLayer(channelsPickerLayerName)

	m.AddLayer(m.messageSearch, layers.WithName(messageSearchLayerName), layers.WithVisible(true))
	if got := m.activeKeyMap(); got != m.messageSearch {
		t.Fatalf("expected message search key map, got %T", got)
	}
	m.RemoveLayer(messageSearchLayerName)

	m.AddLayer(m.pinnedMessages, layers.WithName(pinnedMessagesLayerName), layers.WithVisible(true))
	if got := m.activeKeyMap(); got != m.pinnedMessages {
		t.Fatalf("expected pinned messages key map, got %T", got)
	}
	m.RemoveLayer(pinnedMessagesLayerName)

	m.AddLayer(tview.NewBox(), layers.WithName(reactionPickerLayerName), layers.WithVisible(true))
	if got := m.activeKeyMap(); got != m.messagesList.reactionPicker {
		t.Fatalf("expected reaction picker key map, got %T", got)
	}
	m.RemoveLayer(reactionPickerLayerName)

	m.AddLayer(tview.NewBox(), layers.WithName(attachmentsPickerLayerName), layers.WithVisible(true))
	if got := m.activeKeyMap(); got != m.messagesList.attachmentsPicker {
		t.Fatalf("expected attachments picker key map, got %T", got)
	}

	m.RemoveLayer(attachmentsPickerLayerName)
	setFocusForTest(m.app, tview.NewBox())
	if got := m.activeKeyMap(); got != nil {
		t.Fatalf("expected nil key map for unrelated focus target, got %T", got)
	}
}

func TestModelBaseHelp(t *testing.T) {
	m := newTestModel()

	short := m.baseShortHelp()
	if containsKeybind(short, m.cfg.Keybinds.FocusMessageInput.Keybind) {
		t.Fatal("focus message input should be omitted while input is disabled")
	}
	if containsKeybind(short, m.cfg.Keybinds.ToggleMessageSearch.Keybind) {
		t.Fatal("message search should be omitted when no channel is selected")
	}

	m.messageInput.SetDisabled(false)
	m.SetSelectedChannel(&discord.Channel{ID: 1, Type: discord.GuildText})

	short = m.baseShortHelp()
	if !containsKeybind(short, m.cfg.Keybinds.FocusMessageInput.Keybind) {
		t.Fatal("focus message input should be included when input is enabled")
	}
	if !containsKeybind(short, m.cfg.Keybinds.ToggleMessageSearch.Keybind) {
		t.Fatal("message search should be included when a channel is selected")
	}

	full := m.baseFullHelp()
	if len(full) != 4 {
		t.Fatalf("expected 4 help groups, got %d", len(full))
	}
	if !containsKeybindGroup(full, m.cfg.Keybinds.Logout.Keybind) {
		t.Fatal("logout should be present in full help")
	}
}

func TestModelShortHelpAndFullHelp(t *testing.T) {
	m := newTestModel()
	m.messageInput.SetDisabled(false)
	m.SetSelectedChannel(&discord.Channel{ID: 1, Type: discord.GuildText})
	setFocusForTest(m.app, m.messagesList)

	short := m.ShortHelp()
	if len(short) < len(m.baseShortHelp()) {
		t.Fatalf("expected short help to include base help, got %d items", len(short))
	}
	if !containsKeybind(short, m.cfg.Keybinds.FocusGuildsTree.Keybind) {
		t.Fatal("expected base keybind in short help")
	}

	full := m.FullHelp()
	if len(full) < len(m.baseFullHelp()) {
		t.Fatalf("expected full help to include base help groups, got %d", len(full))
	}
	if !containsKeybindGroup(full, m.cfg.Keybinds.FocusNext.Keybind) {
		t.Fatal("expected focus-next keybind in full help")
	}
}
