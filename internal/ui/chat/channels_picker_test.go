package chat

import (
	"reflect"
	"strings"
	"testing"
	"unsafe"

	"github.com/ayn2op/discordo/internal/ui"
	"github.com/ayn2op/discordo/pkg/picker"
	"github.com/eyalmazuz/tview"
	"github.com/eyalmazuz/tview/keybind"
	"github.com/eyalmazuz/tview/layers"
	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/diamondburned/arikawa/v3/session"
	arikawastate "github.com/diamondburned/arikawa/v3/state"
	"github.com/diamondburned/arikawa/v3/state/store"
	"github.com/diamondburned/arikawa/v3/state/store/defaultstore"
	"github.com/diamondburned/ningen/v3"
)

func pickerPrivateField[T any](t *testing.T, cp *channelsPicker, name string) T {
	t.Helper()

	field := reflect.ValueOf(cp.Picker).Elem().FieldByName(name)
	if !field.IsValid() {
		t.Fatalf("picker field %q not found", name)
	}
	if !field.CanAddr() {
		t.Fatalf("picker field %q is not addressable", name)
	}

	return *(*T)(unsafe.Pointer(field.UnsafeAddr()))
}

func pickerItems(t *testing.T, cp *channelsPicker) []picker.Item {
	t.Helper()
	items := pickerPrivateField[[]picker.Item](t, cp, "items")
	return append([]picker.Item(nil), items...)
}

func helpSummaries(bindings []keybind.Keybind) []string {
	out := make([]string, len(bindings))
	for i, binding := range bindings {
		help := binding.Help()
		out[i] = strings.Join(binding.Keys(), ",") + ":" + help.Desc
	}
	return out
}

func flattenHelp(groups [][]keybind.Keybind) []string {
	var out []string
	for _, group := range groups {
		out = append(out, helpSummaries(group)...)
	}
	return out
}

func newNoopState() *ningen.State {
	s := arikawastate.NewFromSession(session.New(""), store.NoopCabinet)
	return ningen.FromState(s)
}

func grantChannelPermissions(t *testing.T, m *Model, guildID discord.GuildID) {
	t.Helper()

	m.state.Cabinet.MeStore.MyselfSet(discord.User{ID: 1}, false)
	m.state.Cabinet.MemberStore.MemberSet(guildID, &discord.Member{
		User:    discord.User{ID: 1},
		RoleIDs: []discord.RoleID{discord.RoleID(guildID)},
	}, false)
	m.state.Cabinet.RoleStore.RoleSet(guildID, &discord.Role{
		ID:          discord.RoleID(guildID),
		Permissions: discord.PermissionViewChannel | discord.PermissionSendMessages,
	}, false)
}

func openChannelsPickerLayer(m *Model) {
	m.AddLayer(
		m.channelsPicker,
		layers.WithName(channelsPickerLayerName),
		layers.WithVisible(true),
	)
}

func TestNewChannelsPicker(t *testing.T) {
	m := newMockChatModel()
	cp := newChannelsPicker(m.cfg, m)

	if cp.chatView != m {
		t.Fatalf("expected chat view to be retained")
	}
	if cp.GetTitle() != "Channels" {
		t.Fatalf("expected title %q, got %q", "Channels", cp.GetTitle())
	}
	if cp.GetBorderSet() != m.cfg.Theme.Border.ActiveSet.BorderSet {
		t.Fatalf("expected active border set to be applied")
	}

	keyMap := pickerPrivateField[*picker.KeyMap](t, cp, "keyMap")
	if got, want := helpSummaries([]keybind.Keybind{
		keyMap.Up, keyMap.Down, keyMap.Top, keyMap.Bottom, keyMap.Select, keyMap.Cancel,
	}), helpSummaries([]keybind.Keybind{
		m.cfg.Keybinds.Picker.Up.Keybind,
		m.cfg.Keybinds.Picker.Down.Keybind,
		m.cfg.Keybinds.Picker.Top.Keybind,
		m.cfg.Keybinds.Picker.Bottom.Keybind,
		m.cfg.Keybinds.Picker.Select.Keybind,
		m.cfg.Keybinds.Picker.Cancel.Keybind,
	}); !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected keymap:\n got: %#v\nwant: %#v", got, want)
	}

	setFocus := pickerPrivateField[func(tview.Primitive)](t, cp, "setFocus")
	list := pickerPrivateField[*tview.List](t, cp, "list")
	setFocus(list)
	if m.app.GetFocus() != list {
		t.Fatalf("expected picker focus callback to delegate to the app")
	}
}

func TestChannelsPickerAddChannel(t *testing.T) {
	m := newMockChatModel()
	cp := newChannelsPicker(m.cfg, m)

	dm := discord.Channel{ID: 11, Name: "buddy", Type: discord.DirectMessage}
	guild := discord.Guild{ID: 21, Name: "Discordo"}
	guildChannel := discord.Channel{ID: 22, Name: "general", GuildID: guild.ID, Type: discord.GuildText}

	cp.addChannel(nil, dm)
	cp.addChannel(&guild, guildChannel)

	items := pickerItems(t, cp)
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}

	expectedDM := ui.ChannelToString(dm, m.cfg.Icons, m.state)
	if items[0].Text != expectedDM || items[0].FilterText != expectedDM || items[0].Reference != dm.ID {
		t.Fatalf("unexpected DM item: %#v", items[0])
	}

	expectedGuild := ui.ChannelToString(guildChannel, m.cfg.Icons, m.state) + " - " + guild.Name
	if items[1].Text != expectedGuild || items[1].FilterText != expectedGuild || items[1].Reference != guildChannel.ID {
		t.Fatalf("unexpected guild item: %#v", items[1])
	}
}

func TestChannelsPickerUpdateNilStateClearsItems(t *testing.T) {
	m := newMockChatModel()
	cp := newChannelsPicker(m.cfg, m)
	cp.addChannel(nil, discord.Channel{ID: 1, Name: "stale", Type: discord.DirectMessage})
	m.state = nil

	cp.update()

	if got := len(pickerItems(t, cp)); got != 0 {
		t.Fatalf("expected items to be cleared when state is nil, got %d", got)
	}
}

func TestChannelsPickerUpdateErrorClearsItems(t *testing.T) {
	m := newMockChatModel()
	cp := newChannelsPicker(m.cfg, m)
	cp.addChannel(nil, discord.Channel{ID: 1, Name: "stale", Type: discord.DirectMessage})
	m.state = newNoopState()

	cp.update()

	if got := len(pickerItems(t, cp)); got != 0 {
		t.Fatalf("expected items to be cleared on cabinet error, got %d", got)
	}
}

func TestChannelsPickerUpdateSkipsGuildsWithChannelErrors(t *testing.T) {
	m := newMockChatModel()
	cp := newChannelsPicker(m.cfg, m)

	privateChannel := &discord.Channel{
		ID:            100,
		Name:          "latest-dm",
		Type:          discord.DirectMessage,
		LastMessageID: 99,
		DMRecipients:  []discord.User{{ID: 200, Username: "friend"}},
	}
	guild := &discord.Guild{ID: 300, Name: "Guild"}

	m.state.Cabinet.ChannelStore.ChannelSet(privateChannel, false)
	m.state.Cabinet.GuildStore.GuildSet(guild, false)

	cp.update()

	items := pickerItems(t, cp)
	if len(items) != 1 {
		t.Fatalf("expected only private channel items after guild channel lookup failure, got %d", len(items))
	}

	expected := ui.ChannelToString(*privateChannel, m.cfg.Icons, m.state)
	if items[0].Text != expected {
		t.Fatalf("expected private channel item %q, got %q", expected, items[0].Text)
	}
}

func TestChannelsPickerUpdateGuildLookupErrorKeepsPrivateChannels(t *testing.T) {
	m := newMockChatModel()
	cp := newChannelsPicker(m.cfg, m)

	privateChannel := &discord.Channel{
		ID:            100,
		Name:          "latest-dm",
		Type:          discord.DirectMessage,
		LastMessageID: 99,
		DMRecipients:  []discord.User{{ID: 200, Username: "friend"}},
	}
	m.state.Cabinet.ChannelStore.ChannelSet(privateChannel, false)
	m.state.Cabinet.GuildStore = store.Noop

	cp.update()

	items := pickerItems(t, cp)
	if len(items) != 1 {
		t.Fatalf("expected private channels to remain when guild lookup fails, got %d items", len(items))
	}
	expected := ui.ChannelToString(*privateChannel, m.cfg.Icons, m.state)
	if items[0].Text != expected {
		t.Fatalf("expected private channel item %q, got %q", expected, items[0].Text)
	}
}

func TestChannelsPickerUpdateSuccess(t *testing.T) {
	m := newMockChatModel()
	cp := newChannelsPicker(m.cfg, m)

	dmOlder := &discord.Channel{
		ID:            100,
		Name:          "older-dm",
		Type:          discord.DirectMessage,
		LastMessageID: 40,
		DMRecipients:  []discord.User{{ID: 200, Username: "older"}},
	}
	dmNewer := &discord.Channel{
		ID:            101,
		Name:          "newer-dm",
		Type:          discord.DirectMessage,
		LastMessageID: 50,
		DMRecipients:  []discord.User{{ID: 201, Username: "newer"}},
	}
	guild := &discord.Guild{ID: 300, Name: "Guild"}
	guildChannel := &discord.Channel{ID: 301, GuildID: guild.ID, Name: "general", Type: discord.GuildText}
	thread := &discord.Channel{ID: 302, GuildID: guild.ID, ParentID: guildChannel.ID, Name: "thread", Type: discord.GuildPublicThread}

	m.state.Cabinet.ChannelStore.ChannelSet(dmOlder, false)
	m.state.Cabinet.ChannelStore.ChannelSet(dmNewer, false)
	m.state.Cabinet.GuildStore.GuildSet(guild, false)
	m.state.Cabinet.ChannelStore.ChannelSet(guildChannel, false)
	m.state.Cabinet.ChannelStore.ChannelSet(thread, false)

	cp.update()

	items := pickerItems(t, cp)
	if len(items) != 4 {
		t.Fatalf("expected 4 items, got %d", len(items))
	}

	expected := []string{
		ui.ChannelToString(*dmNewer, m.cfg.Icons, m.state),
		ui.ChannelToString(*dmOlder, m.cfg.Icons, m.state),
		ui.ChannelToString(*guildChannel, m.cfg.Icons, m.state) + " - " + guild.Name,
		ui.ChannelToString(*thread, m.cfg.Icons, m.state) + " - " + guild.Name,
	}
	for i, want := range expected {
		if items[i].Text != want {
			t.Fatalf("item %d text mismatch: got %q want %q", i, items[i].Text, want)
		}
	}
}

func TestChannelsPickerOnSelectedNoopBranches(t *testing.T) {
	t.Run("invalid reference", func(t *testing.T) {
		m := newMockChatModel()
		cp := newChannelsPicker(m.cfg, m)
		m.channelsPicker = cp
		openChannelsPickerLayer(m)
		m.app.SetFocus(cp)

		cp.onSelected(picker.Item{Reference: "not-a-channel"})

		if !m.HasLayer(channelsPickerLayerName) {
			t.Fatalf("expected picker to remain open")
		}
	})

	t.Run("invalid channel id", func(t *testing.T) {
		m := newMockChatModel()
		cp := newChannelsPicker(m.cfg, m)
		m.channelsPicker = cp
		openChannelsPickerLayer(m)
		m.app.SetFocus(cp)

		cp.onSelected(picker.Item{Reference: discord.ChannelID(0)})

		if !m.HasLayer(channelsPickerLayerName) {
			t.Fatalf("expected picker to remain open")
		}
	})

	t.Run("missing channel", func(t *testing.T) {
		m := newMockChatModel()
		m.guildsTree = newGuildsTree(m.cfg, m)
		cp := newChannelsPicker(m.cfg, m)
		m.channelsPicker = cp
		openChannelsPickerLayer(m)
		m.app.SetFocus(cp)

		cp.onSelected(picker.Item{Reference: discord.ChannelID(999)})

		if !m.HasLayer(channelsPickerLayerName) {
			t.Fatalf("expected picker to remain open")
		}
	})

	t.Run("channel node not found", func(t *testing.T) {
		m := newMockChatModel()
		m.guildsTree = newGuildsTree(m.cfg, m)
		cp := newChannelsPicker(m.cfg, m)
		m.channelsPicker = cp
		openChannelsPickerLayer(m)
		m.app.SetFocus(cp)

		channel := &discord.Channel{ID: 100, GuildID: 200, Name: "general", Type: discord.GuildText}
		m.state.Cabinet.ChannelStore.ChannelSet(channel, false)

		cp.onSelected(picker.Item{Reference: channel.ID})

		if !m.HasLayer(channelsPickerLayerName) {
			t.Fatalf("expected picker to remain open")
		}
	})
}

func TestChannelsPickerOnSelectedSelectsChannel(t *testing.T) {
	m := newMockChatModel()
	m.cfg.MessagesLimit = 1
	m.guildsTree = newGuildsTree(m.cfg, m)
	m.channelsPicker = newChannelsPicker(m.cfg, m)
	m.messageInput.SetDisabled(false)

	guild := &discord.Guild{ID: 200, Name: "Guild"}
	channel := &discord.Channel{ID: 201, GuildID: guild.ID, Name: "general", Type: discord.GuildText, LastMessageID: 1}

	m.state.Cabinet.GuildStore.GuildSet(guild, false)
	m.state.Cabinet.ChannelStore.ChannelSet(channel, false)
	m.state.Cabinet.MessageStore.MessageSet(&discord.Message{
		ID:        1,
		ChannelID: channel.ID,
		GuildID:   guild.ID,
		Author:    discord.User{ID: 1, Username: "me"},
		Content:   "hello",
	}, false)
	grantChannelPermissions(t, m, guild.ID)
	m.guildsTree.createGuildNode(m.guildsTree.GetRoot(), *guild)
	openChannelsPickerLayer(m)
	m.app.SetFocus(m.channelsPicker)

	m.channelsPicker.onSelected(picker.Item{Reference: channel.ID})

	if selected := m.SelectedChannel(); selected == nil || selected.ID != channel.ID {
		t.Fatalf("expected selected channel %v, got %#v", channel.ID, selected)
	}
	if m.HasLayer(channelsPickerLayerName) {
		t.Fatalf("expected picker layer to be closed")
	}
	if m.app.GetFocus() != m.messageInput {
		t.Fatalf("expected focus on message input, got %T", m.app.GetFocus())
	}

	node := m.guildsTree.findNodeByReference(channel.ID)
	if node == nil {
		t.Fatalf("expected channel node to be present after selection")
	}
	if m.guildsTree.GetCurrentNode() != node {
		t.Fatalf("expected selected tree node to become current")
	}
}

func TestChannelsPickerOnSelectedCategoryClosesWithoutOpeningChannel(t *testing.T) {
	m := newMockChatModel()
	m.guildsTree = newGuildsTree(m.cfg, m)
	m.channelsPicker = newChannelsPicker(m.cfg, m)
	m.messageInput.SetDisabled(false)

	guild := &discord.Guild{ID: 200, Name: "Guild"}
	category := &discord.Channel{ID: 201, GuildID: guild.ID, Name: "Projects", Type: discord.GuildCategory}
	child := &discord.Channel{ID: 202, GuildID: guild.ID, ParentID: category.ID, Name: "general", Type: discord.GuildText}

	m.state.Cabinet.GuildStore.GuildSet(guild, false)
	m.state.Cabinet.ChannelStore.ChannelSet(category, false)
	m.state.Cabinet.ChannelStore.ChannelSet(child, false)
	grantChannelPermissions(t, m, guild.ID)
	m.guildsTree.createGuildNode(m.guildsTree.GetRoot(), *guild)
	openChannelsPickerLayer(m)
	m.app.SetFocus(m.channelsPicker)

	m.channelsPicker.onSelected(picker.Item{Reference: category.ID})

	if m.SelectedChannel() != nil {
		t.Fatalf("expected category selection to avoid opening a channel")
	}
	if m.HasLayer(channelsPickerLayerName) {
		t.Fatalf("expected picker layer to be closed")
	}
	if m.app.GetFocus() != m.messageInput {
		t.Fatalf("expected focus on message input, got %T", m.app.GetFocus())
	}

	node := m.guildsTree.findNodeByReference(category.ID)
	if node == nil {
		t.Fatalf("expected category node to be present after selection")
	}
	if m.guildsTree.GetCurrentNode() != node {
		t.Fatalf("expected selected category node to become current")
	}
}

func TestChannelsPickerHelp(t *testing.T) {
	m := newMockChatModel()
	cp := newChannelsPicker(m.cfg, m)

	shortWant := helpSummaries([]keybind.Keybind{
		m.cfg.Keybinds.Picker.Up.Keybind,
		m.cfg.Keybinds.Picker.Down.Keybind,
		m.cfg.Keybinds.Picker.Select.Keybind,
		m.cfg.Keybinds.Picker.Cancel.Keybind,
	})
	if got := helpSummaries(cp.ShortHelp()); !reflect.DeepEqual(got, shortWant) {
		t.Fatalf("unexpected short help:\n got: %#v\nwant: %#v", got, shortWant)
	}

	fullWant := flattenHelp([][]keybind.Keybind{
		{
			m.cfg.Keybinds.Picker.Up.Keybind,
			m.cfg.Keybinds.Picker.Down.Keybind,
			m.cfg.Keybinds.Picker.Top.Keybind,
			m.cfg.Keybinds.Picker.Bottom.Keybind,
		},
		{
			m.cfg.Keybinds.Picker.Select.Keybind,
			m.cfg.Keybinds.Picker.Cancel.Keybind,
		},
	})
	if got := flattenHelp(cp.FullHelp()); !reflect.DeepEqual(got, fullWant) {
		t.Fatalf("unexpected full help:\n got: %#v\nwant: %#v", got, fullWant)
	}
}

func TestChannelsPickerUsesExistingDefaultStoreState(t *testing.T) {
	// Guard against helper regressions that would make picker tests silently use
	// a state backend with different lookup semantics.
	m := newMockChatModel()
	if _, ok := m.state.Cabinet.ChannelStore.(*defaultstore.Channel); !ok {
		t.Fatalf("expected default channel store in test helper")
	}
}
