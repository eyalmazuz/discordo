package chat

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/ayn2op/discordo/internal/config"
	"github.com/eyalmazuz/tview"
	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/diamondburned/arikawa/v3/gateway"
	"github.com/diamondburned/arikawa/v3/utils/httputil/httpdriver"
	"github.com/diamondburned/arikawa/v3/utils/ws"
	"github.com/diamondburned/ningen/v3"
)

type dummyRequest struct{}

func (dummyRequest) GetPath() string              { return "/" }
func (dummyRequest) GetContext() context.Context  { return context.Background() }
func (dummyRequest) AddHeader(http.Header)        {}
func (dummyRequest) AddQuery(url.Values)          {}
func (dummyRequest) WithBody(io.ReadCloser)       {}

func TestState_Events(t *testing.T) {
	m := newTestModel()
	
	t.Run("onMessageCreate", func(t *testing.T) {
		cid := discord.ChannelID(123)
		m.SetSelectedChannel(&discord.Channel{ID: cid})
		
		ev := &gateway.MessageCreateEvent{
			Message: discord.Message{
				ID:        1,
				ChannelID: cid,
				Content:   "hello",
				Author:    discord.User{ID: 2, Username: "other"},
			},
		}
		
		m.onMessageCreate(ev)
		time.Sleep(10 * time.Millisecond)
	})

	t.Run("onMessageUpdate", func(t *testing.T) {
		cid := discord.ChannelID(123)
		m.SetSelectedChannel(&discord.Channel{ID: cid})
		m.messagesList.messages = []discord.Message{{ID: 1}}
		
		ev := &gateway.MessageUpdateEvent{
			Message: discord.Message{
				ID:        1,
				ChannelID: cid,
				Content:   "hello edited",
			},
		}
		
		m.onMessageUpdate(ev)
		time.Sleep(10 * time.Millisecond)
	})

	t.Run("onMessageDelete", func(t *testing.T) {
		cid := discord.ChannelID(123)
		m.SetSelectedChannel(&discord.Channel{ID: cid})
		m.messagesList.messages = []discord.Message{{ID: 1}}
		
		ev := &gateway.MessageDeleteEvent{
			ID:        1,
			ChannelID: cid,
		}
		
		m.onMessageDelete(ev)
		time.Sleep(10 * time.Millisecond)
	})

	t.Run("onTypingStart_Branches", func(t *testing.T) {
		m.SetSelectedChannel(nil)
		m.onTypingStart(&gateway.TypingStartEvent{}) // nil selectedChannel

		cid := discord.ChannelID(123)
		m.SetSelectedChannel(&discord.Channel{ID: cid})
		m.onTypingStart(&gateway.TypingStartEvent{ChannelID: 456}) // ID mismatch

		m.onTypingStart(&gateway.TypingStartEvent{ChannelID: cid, UserID: 1}) // me.ID match (me.ID=1)
		m.onTypingStart(&gateway.TypingStartEvent{ChannelID: cid, UserID: 2}) // success branch
	})

		t.Run("onMessageDelete_CursorMapping", func(t *testing.T) {
			oldQueueUpdateDraw := queueUpdateDraw
			queueCalls := 0
			queueUpdateDraw = func(_ *tview.Application, f func()) {
				queueCalls++
				f()
			}
			t.Cleanup(func() { queueUpdateDraw = oldQueueUpdateDraw })

			cid := discord.ChannelID(123)
			m.SetSelectedChannel(&discord.Channel{ID: cid})
		
		// 1. prevCursor == deletedIndex, newCursor = deletedIndex - 1
			m.messagesList.setMessages([]discord.Message{{ID: 1}, {ID: 2}})
			m.messagesList.SetCursor(1)
			m.onMessageDelete(&gateway.MessageDeleteEvent{ID: 2, ChannelID: cid})
			
			// 2. prevCursor == deletedIndex == 0, newCursor = deletedIndex (if len > 0) -> newCursor == prevCursor
			m.messagesList.setMessages([]discord.Message{{ID: 3}, {ID: 4}})
			m.messagesList.SetCursor(0)
			m.onMessageDelete(&gateway.MessageDeleteEvent{ID: 3, ChannelID: cid})

			// 3. prevCursor == deletedIndex == 0, newCursor = -1 (if len == 0)
			m.messagesList.setMessages([]discord.Message{{ID: 5}})
			m.messagesList.SetCursor(0)
			m.onMessageDelete(&gateway.MessageDeleteEvent{ID: 5, ChannelID: cid})

			// 4. prevCursor > deletedIndex
			m.messagesList.setMessages([]discord.Message{{ID: 6}, {ID: 7}})
			m.messagesList.SetCursor(1)
			beforeCalls := queueCalls
			m.onMessageDelete(&gateway.MessageDeleteEvent{ID: 6, ChannelID: cid})
			if got := queueCalls - beforeCalls; got != 2 {
				t.Fatalf("expected delete+cursor update callbacks after deleting an earlier message, got %d", got)
			}
			
			// 5. deletedIndex < 0 (not found)
			m.onMessageDelete(&gateway.MessageDeleteEvent{ID: 99, ChannelID: cid})
		})

	t.Run("onReady", func(t *testing.T) {
		ev := &gateway.ReadyEvent{
			User: discord.User{ID: 1},
			Guilds: []gateway.GuildCreateEvent{
				{Guild: discord.Guild{ID: 10, Name: "G1"}},
				{Guild: discord.Guild{ID: 20, Name: "G2"}},
				{Guild: discord.Guild{ID: 30, Name: "G3"}},
				{Guild: discord.Guild{ID: 40, Name: "Orphan"}},
			},
		}
		ev.ReadyEventExtras.UserSettings = &gateway.UserSettings{
			GuildPositions: []discord.GuildID{10},
			GuildFolders: []gateway.GuildFolder{
				{ID: 0, GuildIDs: []discord.GuildID{10}},
				{ID: 1, Name: "folder", GuildIDs: []discord.GuildID{20, 30}},
			},
		}
		
		m.onReady(ev)
		time.Sleep(10 * time.Millisecond)
	})

	t.Run("onReady_FallbackPositions", func(t *testing.T) {
		ev := &gateway.ReadyEvent{
			Guilds: []gateway.GuildCreateEvent{
				{Guild: discord.Guild{ID: 10}},
			},
		}
		ev.ReadyEventExtras.UserSettings = &gateway.UserSettings{
			GuildPositions: nil, // trigger fallback
		}
		m.onReady(ev)
		time.Sleep(10 * time.Millisecond)
	})

	t.Run("onRaw", func(t *testing.T) {
		m.onRaw(&ws.RawEvent{OriginalCode: 1, OriginalType: "TEST"})
	})

	t.Run("onGuildMembersChunk", func(t *testing.T) {
		m.onGuildMembersChunk(&gateway.GuildMembersChunkEvent{
			Members: []discord.Member{{User: discord.User{ID: 2}}},
		})
	})

	t.Run("onRequest", func(t *testing.T) {
		m.onRequest(&httpdriver.DefaultRequest{})
		m.onRequest(dummyRequest{})
	})

	t.Run("onMessageCreate_Notify", func(t *testing.T) {
		m.SetSelectedChannel(&discord.Channel{ID: 123})
		ev := &gateway.MessageCreateEvent{
			Message: discord.Message{
				ID:        1,
				ChannelID: 456, // mismatch
				Author:    discord.User{ID: 2},
			},
		}
		m.onMessageCreate(ev)
	})

	t.Run("onMessageCreate_NotifyError", func(t *testing.T) {
		oldNotifyMessage := notifyMessage
		t.Cleanup(func() { notifyMessage = oldNotifyMessage })
		notifyMessage = func(*ningen.State, *gateway.MessageCreateEvent, *config.Config) error {
			return errors.New("notify fail")
		}

		m.SetSelectedChannel(&discord.Channel{ID: 123})
		ev := &gateway.MessageCreateEvent{
			Message: discord.Message{
				ID:        2,
				ChannelID: 456,
				Author:    discord.User{ID: 2},
			},
		}
		m.onMessageCreate(ev)
	})

	t.Run("onMessageUpdate_NotFound", func(t *testing.T) {
		cid := discord.ChannelID(123)
		m.SetSelectedChannel(&discord.Channel{ID: cid})
		m.messagesList.messages = []discord.Message{{ID: 1}}
		m.onMessageUpdate(&gateway.MessageUpdateEvent{Message: discord.Message{ID: 99, ChannelID: cid}})
	})

	t.Run("onMessageReaction_Error", func(t *testing.T) {
		cid := discord.ChannelID(123)
		m.SetSelectedChannel(&discord.Channel{ID: cid})
		m.messagesList.messages = []discord.Message{{ID: 1}}
		
		m.onMessageReactionAdd(&gateway.MessageReactionAddEvent{MessageID: 99, ChannelID: cid})
		m.onMessageReactionRemove(&gateway.MessageReactionRemoveEvent{MessageID: 99, ChannelID: cid})
		
		m.onMessageReactionAdd(&gateway.MessageReactionAddEvent{MessageID: 1, ChannelID: cid})
		m.onMessageReactionRemove(&gateway.MessageReactionRemoveEvent{MessageID: 1, ChannelID: cid})
	})

	t.Run("CloseState_Nil", func(t *testing.T) {
		m2 := &Model{}
		m2.CloseState()
	})
}

func TestState_ReactionEvents(t *testing.T) {
	m := newTestModel()
	cid := discord.ChannelID(123)
	m.SetSelectedChannel(&discord.Channel{ID: cid})
	m.messagesList.messages = []discord.Message{{ID: 1}}

	t.Run("onMessageReactionAdd", func(t *testing.T) {
		m.state.Cabinet.MessageStore.MessageSet(&discord.Message{ID: 1, ChannelID: cid}, false)
		ev := &gateway.MessageReactionAddEvent{
			MessageID: 1,
			ChannelID: cid,
			Emoji:     discord.Emoji{Name: "😀"},
		}
		m.onMessageReactionAdd(ev)
		time.Sleep(10 * time.Millisecond)
	})

	t.Run("onMessageReactionRemove", func(t *testing.T) {
		m.state.Cabinet.MessageStore.MessageSet(&discord.Message{ID: 1, ChannelID: cid}, false)
		ev := &gateway.MessageReactionRemoveEvent{
			MessageID: 1,
			ChannelID: cid,
			Emoji:     discord.Emoji{Name: "😀"},
		}
		m.onMessageReactionRemove(ev)
		time.Sleep(10 * time.Millisecond)
	})
}

func TestState_onMessageDelete_Empty(t *testing.T) {
	m := newTestModel()
	cid := discord.ChannelID(123)
	m.SetSelectedChannel(&discord.Channel{ID: cid})
	
	oldQueueUpdateDraw := queueUpdateDraw
	queueUpdateDraw = func(_ *tview.Application, f func()) { f() }
	t.Cleanup(func() { queueUpdateDraw = oldQueueUpdateDraw })

	m.messagesList.setMessages([]discord.Message{{ID: 1}})
	m.messagesList.SetCursor(0)
	m.onMessageDelete(&gateway.MessageDeleteEvent{ID: 1, ChannelID: cid})
	if got := m.messagesList.Cursor(); got != -1 {
		t.Errorf("expected cursor -1, got %d", got)
	}
}
