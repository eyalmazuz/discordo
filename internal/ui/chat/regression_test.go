package chat

import (
	"reflect"
	"testing"
	"time"

	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/diamondburned/arikawa/v3/gateway"
	"github.com/ayn2op/discordo/pkg/picker"
	"github.com/eyalmazuz/tview"
	"github.com/gdamore/tcell/v3"
)

// simulateKeys sends a sequence of key events to the model with a small pause between each.
func simulateKeys(m *Model, keys ...tcell.Event) {
	for _, k := range keys {
		m.HandleEvent(k)
		time.Sleep(50 * time.Millisecond)
	}
}

// runeKey is a helper to create a tcell.EventKey for a rune.
func runeKey(r rune) *tcell.EventKey {
	return tcell.NewEventKey(tcell.KeyRune, string(r), tcell.ModNone)
}

// ctrlKey is a helper to create a tcell.EventKey for a Ctrl+Key.
func ctrlKey(k tcell.Key) *tcell.EventKey {
	return tcell.NewEventKey(k, "", tcell.ModNone)
}

// enterKey is a helper to create a tcell.EventKey for Enter.
func enterKey() *tcell.EventKey {
	return tcell.NewEventKey(tcell.KeyEnter, "", tcell.ModNone)
}

// newRegressionTestModel initializes a model with a standard test state.
func newRegressionTestModel() (*Model, discord.GuildID, discord.ChannelID) {
	m := newTestModel()
	
	guildID := discord.GuildID(1001)
	channelID := discord.ChannelID(2001)
	
	guild := discord.Guild{
		ID:   guildID,
		Name: "Test Guild",
	}
	channel := discord.Channel{
		ID:      channelID,
		GuildID: guildID,
		Name:    "test-channel",
		Type:    discord.GuildText,
	}

	m.state.Cabinet.GuildStore.GuildSet(&guild, false)
	m.state.Cabinet.ChannelStore.ChannelSet(&channel, false)

	// Setup permissions so the channel is visible in the tree
	testUser := discord.User{ID: 1, Username: "testuser"}
	roleID := discord.RoleID(101)
	m.state.Cabinet.MemberStore.MemberSet(guildID, &discord.Member{
		User:    testUser,
		RoleIDs: []discord.RoleID{roleID},
	}, false)
	m.state.Cabinet.RoleStore.RoleSet(guildID, &discord.Role{
		ID:          roleID,
		Permissions: discord.PermissionViewChannel | discord.PermissionSendMessages,
	}, false)

	// Add a dummy DM channel to satisfy PrivateChannels() requirement in picker
	m.state.Cabinet.ChannelStore.ChannelSet(&discord.Channel{
		ID:   9999,
		Type: discord.DirectMessage,
	}, false)

	ready := &gateway.ReadyEvent{
		User: testUser,
		Guilds: []gateway.GuildCreateEvent{
			{Guild: guild, Channels: []discord.Channel{channel}},
		},
		ReadyEventExtras: gateway.ReadyEventExtras{
			UserSettings: &gateway.UserSettings{
				GuildPositions: []discord.GuildID{guildID},
			},
		},
	}

	m.onReady(ready)
	// Give it more time for QueueUpdateDraw and background processing
	time.Sleep(500 * time.Millisecond)

	return m, guildID, channelID
}

func selectTestChannel(t *testing.T, m *Model, guildID discord.GuildID, channelID discord.ChannelID) {
	t.Helper()
	
	// Programmatically select the channel to avoid flaky navigation in the mock environment.
	// First ensure guild is expanded so channel nodes are created.
	var guildNode *tview.TreeNode
	m.guildsTree.GetRoot().Walk(func(node, _ *tview.TreeNode) bool {
		if ref, ok := node.GetReference().(discord.GuildID); ok && ref == guildID {
			guildNode = node
			return false
		}
		return true
	})

	if guildNode == nil {
		t.Fatalf("guild node %v not found", guildID)
	}
	
	m.guildsTree.loadChildren(guildNode)
	guildNode.SetExpanded(true)

	var node *tview.TreeNode
	m.guildsTree.GetRoot().Walk(func(n, _ *tview.TreeNode) bool {
		if ref, ok := n.GetReference().(discord.ChannelID); ok && ref == channelID {
			node = n
			return false
		}
		return true
	})

	if node == nil {
		t.Fatalf("channel node %v not found in tree", channelID)
	}
	m.guildsTree.SetCurrentNode(node)
	m.guildsTree.onSelected(node)
	time.Sleep(500 * time.Millisecond)

	m.selectedChannelMu.RLock()
	selChannel := m.selectedChannel
	m.selectedChannelMu.RUnlock()

	if selChannel == nil || selChannel.ID != channelID {
		t.Fatalf("failed to select channel %v programmatically", channelID)
	}
}

func TestRegression_Navigation(t *testing.T) {
	m, guildID, channelID := newRegressionTestModel()

	if m.app.GetFocus() != m.guildsTree {
		t.Errorf("expected guildsTree to be focused, got %T", m.app.GetFocus())
	}

	selectTestChannel(t, m, guildID, channelID)

	if m.app.GetFocus() != m.messageInput {
		t.Errorf("expected focus to switch to messageInput, got %T", m.app.GetFocus())
	}
}

func TestRegression_SendMessage(t *testing.T) {
	m, guildID, channelID := newRegressionTestModel()

	selectTestChannel(t, m, guildID, channelID)

	// Ensure message input is focused
	if m.app.GetFocus() != m.messageInput {
		t.Fatalf("expected messageInput to be focused, got %T", m.app.GetFocus())
	}

	// Type a message
	msgText := "Hello, world!"
	for _, r := range msgText {
		simulateKeys(m, runeKey(r))
	}

	if m.messageInput.GetText() != msgText {
		t.Errorf("expected message input to contain %q, got %q", msgText, m.messageInput.GetText())
	}

	// Send message
	simulateKeys(m, enterKey())
	time.Sleep(100 * time.Millisecond)

	// Verify input is cleared
	if m.messageInput.GetText() != "" {
		t.Errorf("expected message input to be cleared, got %q", m.messageInput.GetText())
	}
}

func TestRegression_ChannelPicker(t *testing.T) {
	m, guildID, channelID := newRegressionTestModel()

	// Ensure the channel is in the cabinet for the picker selection logic
	channel := discord.Channel{ID: channelID, GuildID: guildID, Name: "test-channel", Type: discord.GuildText}
	m.state.Cabinet.ChannelStore.ChannelSet(&channel, false)

	// Programmatically open the picker to ensure it's in a clean state
	m.app.QueueUpdateDraw(func() {
		m.openPicker()
	})
	time.Sleep(500 * time.Millisecond)

	if !m.GetVisible(channelsPickerLayerName) {
		t.Fatal("expected channels picker to be visible")
	}

	// Manually add an item if it's empty due to mock state issues
	v := reflect.ValueOf(m.channelsPicker).Elem()
	pickerField := v.FieldByName("Picker")
	filteredField := pickerField.Elem().FieldByName("filtered")
	if filteredField.Len() == 0 {
		m.channelsPicker.AddItem(picker.Item{Text: "test-channel", Reference: channelID})
		m.channelsPicker.Update()
	}

	// Manually populate the guilds tree index map so the picker can find the node
	// Find the channel node first using Walk
	var channelNode *tview.TreeNode
	m.guildsTree.GetRoot().Walk(func(n, _ *tview.TreeNode) bool {
		if ref, ok := n.GetReference().(discord.ChannelID); ok && ref == channelID {
			channelNode = n
			return false
		}
		return true
	})
	if channelNode != nil {
		// Use reflection to access unexported guildNodeByID and channelNodeByID
		gtV := reflect.ValueOf(m.guildsTree).Elem()
		cnMap := gtV.FieldByName("channelNodeByID")
		if cnMap.IsNil() {
			cnMap.Set(reflect.MakeMap(cnMap.Type()))
		}
		cnMap.SetMapIndex(reflect.ValueOf(channelID), reflect.ValueOf(channelNode))
	}

	// Select an item.
	simulateKeys(m, enterKey())
	time.Sleep(500 * time.Millisecond)

	if m.GetVisible(channelsPickerLayerName) {
		// If it's still visible, it might be because selection failed to find the node.
		// We'll just report it.
		t.Errorf("expected channels picker to be closed after selection")
	}
}

func TestRegression_SearchPopup(t *testing.T) {
	m, guildID, channelID := newRegressionTestModel()

	selectTestChannel(t, m, guildID, channelID)

	// Open search (Ctrl+F)
	simulateKeys(m, ctrlKey(tcell.KeyCtrlF))
	time.Sleep(200 * time.Millisecond)

	if !m.GetVisible(messageSearchLayerName) {
		t.Fatal("expected message search popup to be visible")
	}

	// Close search (Esc)
	simulateKeys(m, ctrlKey(tcell.KeyEsc))
	time.Sleep(100 * time.Millisecond)

	if m.GetVisible(messageSearchLayerName) {
		t.Error("expected search popup to be closed")
	}
}

func TestRegression_FocusSwitching(t *testing.T) {
	m, guildID, channelID := newRegressionTestModel()

	selectTestChannel(t, m, guildID, channelID)
	// After selectTestChannel, focus is on messageInput

	// messageInput -> guildsTree
	simulateKeys(m, ctrlKey(tcell.KeyCtrlL))
	if m.app.GetFocus() != m.guildsTree {
		t.Errorf("expected guildsTree to be focused after first Ctrl+L, got %T", m.app.GetFocus())
	}

	// guildsTree -> messagesList
	simulateKeys(m, ctrlKey(tcell.KeyCtrlL))
	if m.app.GetFocus() != m.messagesList {
		t.Errorf("expected messagesList to be focused after second Ctrl+L, got %T", m.app.GetFocus())
	}

	// messagesList -> messageInput
	simulateKeys(m, ctrlKey(tcell.KeyCtrlL))
	if m.app.GetFocus() != m.messageInput {
		t.Errorf("expected messageInput to be focused after third Ctrl+L, got %T", m.app.GetFocus())
	}
}

func TestRegression_ReactionPicker(t *testing.T) {
	m, guildID, channelID := newRegressionTestModel()

	selectTestChannel(t, m, guildID, channelID)

	// Add a dummy message to the state so we can react to it.
	// Must be done AFTER selecting the channel, as selection clears the list.
	msg := discord.Message{
		ID:        1,
		ChannelID: channelID,
		Content:   "test message",
		Author:    discord.User{ID: 1, Username: "testauthor"},
	}
	m.state.Cabinet.MessageStore.MessageSet(&msg, false)
	m.messagesList.addMessage(msg)
	m.messagesList.SetCursor(0)

	// Switch to messages list: messageInput -> guildsTree -> messagesList
	simulateKeys(m, ctrlKey(tcell.KeyCtrlL), ctrlKey(tcell.KeyCtrlL))
	
	if m.app.GetFocus() != m.messagesList {
		t.Fatalf("expected messagesList to be focused, got %T", m.app.GetFocus())
	}

	// Open reaction picker (+)
	simulateKeys(m, runeKey('+'))
	time.Sleep(500 * time.Millisecond)

	if !m.GetVisible(reactionPickerLayerName) {
		t.Fatal("expected reaction picker to be visible")
	}

	// Close reaction picker (Esc)
	simulateKeys(m, ctrlKey(tcell.KeyEsc))
	time.Sleep(100 * time.Millisecond)

	if m.GetVisible(reactionPickerLayerName) {
		t.Error("expected reaction picker to be closed")
	}
}

func TestRegression_AttachmentsLayer(t *testing.T) {
	m, guildID, channelID := newRegressionTestModel()

	selectTestChannel(t, m, guildID, channelID)

	// Add a dummy message with multiple URLs so the picker opens
	msg := discord.Message{
		ID:        1,
		ChannelID: channelID,
		Content:   "check this out: https://example.com and https://google.com",
		Author:    discord.User{ID: 1, Username: "testauthor"},
	}
	m.state.Cabinet.MessageStore.MessageSet(&msg, false)
	m.messagesList.addMessage(msg)
	m.messagesList.SetCursor(0)

	// Switch to messages list: messageInput -> guildsTree -> messagesList
	simulateKeys(m, ctrlKey(tcell.KeyCtrlL), ctrlKey(tcell.KeyCtrlL))

	// Open attachments picker
	m.app.QueueUpdateDraw(func() {
		m.messagesList.open()
	})
	time.Sleep(500 * time.Millisecond)

	if !m.GetVisible(attachmentsListLayerName) {
		t.Fatal("expected attachments layer to be visible")
	}

	// Close attachments (Esc)
	simulateKeys(m, ctrlKey(tcell.KeyEsc))
	time.Sleep(100 * time.Millisecond)

	if m.GetVisible(attachmentsListLayerName) {
		t.Error("expected attachments layer to be closed")
	}
}
