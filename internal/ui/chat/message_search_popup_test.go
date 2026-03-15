package chat

import (
	"testing"
	"time"

	"github.com/ayn2op/discordo/internal/config"
	"github.com/ayn2op/tview/layers"
	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/gdamore/tcell/v3"
)

func TestModel_HandleEvent_SearchKeyOpensPopup(t *testing.T) {
	m := newMockChatModel()
	channel := &discord.Channel{ID: 200, GuildID: 100, Type: discord.GuildText, Name: "general"}
	m.SetSelectedChannel(channel)

	m.HandleEvent(tcell.NewEventKey(tcell.KeyCtrlF, "", tcell.ModNone))

	if !m.HasLayer(messageSearchLayerName) {
		t.Fatal("expected message search layer to be visible")
	}

	if m.app.GetFocus() != m.messageSearch.input {
		t.Fatalf("expected focus on search input, got %T", m.app.GetFocus())
	}
}

func TestMessageSearchPopup_EnterRunsSearch(t *testing.T) {
	cfg, _ := config.Load("")
	m := newMockChatModel()
	channel := discord.Channel{ID: 200, GuildID: 100, Type: discord.GuildText, Name: "general"}
	m.SetSelectedChannel(&channel)

	sp := newMessageSearchPopup(cfg, m, m.messagesList)
	sp.queueUpdateDraw = func(f func()) { f() }
	called := make(chan struct{}, 1)
	sp.searchMessages = func(gotChannel discord.Channel, query string) ([]messageSearchResult, error) {
		if gotChannel.ID != channel.ID {
			t.Fatalf("expected search channel %v, got %v", channel.ID, gotChannel.ID)
		}
		if query != "hello" {
			t.Fatalf("expected search query %q, got %q", "hello", query)
		}
		called <- struct{}{}
		return []messageSearchResult{
			{
				Message: discord.Message{
					ID:        300,
					ChannelID: channel.ID,
					GuildID:   channel.GuildID,
					Content:   "hello world",
					Timestamp: discord.NewTimestamp(time.Unix(0, 0)),
					Author:    discord.User{ID: 10, Username: "user"},
				},
			},
		}, nil
	}

	sp.Prepare(channel, m.messagesList)
	sp.input.SetText("hello")
	m.app.SetFocus(sp.input)

	sp.HandleEvent(tcell.NewEventKey(tcell.KeyEnter, "", tcell.ModNone))

	select {
	case <-called:
	case <-time.After(300 * time.Millisecond):
		t.Fatal("expected Enter on the input to trigger a search")
	}

	deadline := time.Now().Add(300 * time.Millisecond)
	for len(sp.results) != 1 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}

	if len(sp.results) != 1 {
		t.Fatalf("expected 1 search result, got %d", len(sp.results))
	}
	if sp.results[0].Message.ID != 300 {
		t.Fatalf("expected result message 300, got %v", sp.results[0].Message.ID)
	}
}

func TestMessageSearchPopup_SelectCurrentJumpsToMessage(t *testing.T) {
	cfg, _ := config.Load("")
	m := newMockChatModel()
	channel := discord.Channel{ID: 200, GuildID: 100, Type: discord.GuildText, Name: "general"}
	m.SetSelectedChannel(&channel)

	sp := newMessageSearchPopup(cfg, m, m.messagesList)
	m.messageSearch = sp
	sp.Prepare(channel, m.messageInput)
	sp.results = []messageSearchResult{
		{
			Message: discord.Message{
				ID:        300,
				ChannelID: channel.ID,
				GuildID:   channel.GuildID,
				Content:   "hello world",
				Timestamp: discord.NewTimestamp(time.Unix(0, 0)),
				Author:    discord.User{ID: 10, Username: "user"},
			},
		},
	}
	sp.list.SetCursor(0)

	var (
		gotChannel discord.ChannelID
		gotMessage discord.MessageID
	)
	sp.jumpToMessage = func(got discord.Channel, messageID discord.MessageID) error {
		gotChannel = got.ID
		gotMessage = messageID
		return nil
	}

	m.AddLayer(sp, layers.WithName(messageSearchLayerName), layers.WithVisible(true))
	m.app.SetFocus(sp.list)

	sp.selectCurrent()

	if gotChannel != channel.ID {
		t.Fatalf("expected jump channel %v, got %v", channel.ID, gotChannel)
	}
	if gotMessage != 300 {
		t.Fatalf("expected jump message 300, got %v", gotMessage)
	}
	if m.HasLayer(messageSearchLayerName) {
		t.Fatal("expected search popup to be closed after selecting a result")
	}
	if m.app.GetFocus() != m.messagesList {
		t.Fatalf("expected focus to move to messages list, got %T", m.app.GetFocus())
	}
}
