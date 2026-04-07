package chat

import (
	"testing"
	"time"

	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/diamondburned/arikawa/v3/gateway"
	"github.com/gdamore/tcell/v3"
)

func TestRegressionNavigationAndSearch(t *testing.T) {
	m := newTestModel()
	m.messageInput.SetDisabled(false)
	channel := &discord.Channel{ID: 2001, GuildID: 1001, Type: discord.GuildText, Name: "general"}
	m.SetSelectedChannel(channel)

	execCmdForTest(m.app, m.Update(tcell.NewEventKey(tcell.KeyCtrlG, "", tcell.ModNone)))
	if m.app.Focused() != m.guildsTree {
		t.Fatalf("expected ctrl+g to focus guild tree, got %T", m.app.Focused())
	}

	execCmdForTest(m.app, m.Update(tcell.NewEventKey(tcell.KeyCtrlT, "", tcell.ModNone)))
	if m.app.Focused() != m.messagesList {
		t.Fatalf("expected ctrl+t to focus messages list, got %T", m.app.Focused())
	}

	execCmdForTest(m.app, m.Update(tcell.NewEventKey(tcell.KeyCtrlI, "", tcell.ModNone)))
	if m.app.Focused() != m.messageInput {
		t.Fatalf("expected ctrl+i to focus message input, got %T", m.app.Focused())
	}

	m.Update(tcell.NewEventKey(tcell.KeyCtrlF, "", tcell.ModNone))
	if !m.HasLayer(messageSearchLayerName) {
		t.Fatal("expected ctrl+f to open search popup")
	}

	m.Update(tcell.NewEventKey(tcell.KeyCtrlP, "", tcell.ModNone))
	if !m.HasLayer(messageSearchLayerName) {
		t.Fatal("expected pinned shortcut not to displace visible search popup")
	}
}

func TestRegressionReadyAndChannelLoad(t *testing.T) {
	m := newTestModel()
	ready := &gateway.ReadyEvent{
		User: discord.User{ID: 1, Username: "me"},
		Guilds: []gateway.GuildCreateEvent{
			{Guild: discord.Guild{ID: 10, Name: "Guild"}, Channels: []discord.Channel{{ID: 20, GuildID: 10, Name: "general", Type: discord.GuildText}}},
		},
	}
	ready.ReadyEventExtras.UserSettings = &gateway.UserSettings{
		GuildPositions: []discord.GuildID{10},
	}

	cmd := m.onReady(ready)
	if cmd == nil {
		t.Fatal("expected ready event to return a focus command")
	}
	executeModelCommand(m, cmd)

	if m.guildsTree.findNodeByReference(discord.GuildID(10)) == nil {
		t.Fatal("expected ready event to build guild tree nodes")
	}

	channel := discord.Channel{ID: 20, GuildID: 10, Name: "general", Type: discord.GuildText}
	m.state.Cabinet.ChannelStore.ChannelSet(&channel, false)
	if cmd := m.guildsTree.loadChannel(channel); cmd == nil {
		t.Fatal("expected loadChannel to return a command")
	}
}

func TestRegressionTypingFooter(t *testing.T) {
	m := newTestModel()
	channel := &discord.Channel{ID: 123, Type: discord.DirectMessage, DMRecipients: []discord.User{{ID: 2, Username: "user2"}}}
	m.SetSelectedChannel(channel)

	m.addTyper(2)
	time.Sleep(20 * time.Millisecond)
	if footer := m.messagesList.GetFooter(); footer == "" {
		t.Fatal("expected typing footer to be populated")
	}
	m.clearTypers()
}
