package chat

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/ayn2op/discordo/internal/markdown"
	"github.com/diamondburned/arikawa/v3/discord"
)

func TestAvailableEmojisForChannel(t *testing.T) {
	if emojis := availableEmojisForChannel(nil, nil); emojis != nil {
		t.Fatalf("expected nil state/channel to return nil, got %#v", emojis)
	}

	t.Run("non nitro keeps current guild emojis only", func(t *testing.T) {
		m := newTestModel()
		channel := &discord.Channel{ID: 10, GuildID: 100, Type: discord.GuildText}
		m.state.Cabinet.EmojiSet(channel.GuildID, []discord.Emoji{{ID: 1, Name: "guild"}}, false)
		m.state.Cabinet.MeStore.MyselfSet(discord.User{ID: 1, Username: "me", Nitro: discord.NoUserNitro}, true)

		emojis := availableEmojisForChannel(m.state, channel)
		if len(emojis) == 0 || emojis[0].Name != "guild" {
			t.Fatalf("expected current guild emojis, got %#v", emojis)
		}
		if len(emojis) != 1+len(markdown.StandardEmojis) {
			t.Fatalf("expected standard emojis appended, got %d emojis", len(emojis))
		}
	})

	t.Run("fetches current guild and appends nitro cross guild emojis", func(t *testing.T) {
		transport := &mockTransport{}
		m := newTestModelWithTransport(transport)
		channel := &discord.Channel{ID: 10, GuildID: 100, Type: discord.GuildText}
		otherGuildID := discord.GuildID(200)

		m.state.Cabinet.GuildStore.GuildSet(&discord.Guild{ID: channel.GuildID, Name: "Current"}, false)
		m.state.Cabinet.GuildStore.GuildSet(&discord.Guild{ID: otherGuildID, Name: "Other"}, false)
		m.state.Cabinet.MeStore.MyselfSet(discord.User{ID: 1, Username: "me", Nitro: discord.NitroFull}, true)
		m.state.Cabinet.EmojiSet(otherGuildID, []discord.Emoji{
			{ID: 2, Name: "other"},
			{ID: 1, Name: "guild"},
		}, false)

		transport.roundTrip = func(req *http.Request) (*http.Response, error) {
			if req.URL.Path != "/api/v9/guilds/100/emojis" {
				t.Fatalf("unexpected emoji fetch path %q", req.URL.Path)
			}

			data, err := json.Marshal([]discord.Emoji{{ID: 1, Name: "guild"}})
			if err != nil {
				return nil, err
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewReader(data)),
				Header:     make(http.Header),
			}, nil
		}

		emojis := availableEmojisForChannel(m.state, channel)
		if len(emojis) == 0 {
			t.Fatalf("expected deduplicated current+cross-guild emojis, got %#v", emojis)
		}
		if emojis[0].ID != 1 || emojis[1].ID != 2 {
			t.Fatalf("expected current guild emoji then other guild emoji, got %#v", emojis)
		}
		if len(emojis) != 2+len(markdown.StandardEmojis) {
			t.Fatalf("expected standard emojis appended, got %d", len(emojis))
		}
	})

	t.Run("nitro user with unavailable guild list keeps current guild emojis", func(t *testing.T) {
		m := newTestModel()
		channel := &discord.Channel{ID: 10, GuildID: 100, Type: discord.GuildText}
		m.state.Cabinet.MeStore.MyselfSet(discord.User{ID: 1, Username: "me", Nitro: discord.NitroFull}, true)
		m.state.Cabinet.EmojiSet(channel.GuildID, []discord.Emoji{{ID: 1, Name: "guild"}}, false)

		emojis := availableEmojisForChannel(m.state, channel)
		if len(emojis) == 0 || emojis[0].ID != 1 {
			t.Fatalf("expected current guild emojis to be preserved when guild list is unavailable, got %#v", emojis)
		}
		if len(emojis) != 1+len(markdown.StandardEmojis) {
			t.Fatalf("expected standard emojis appended, got %d", len(emojis))
		}
	})

	t.Run("current guild fetch failure returns no emojis", func(t *testing.T) {
		transport := &mockTransport{}
		m := newTestModelWithTransport(transport)
		channel := &discord.Channel{ID: 10, GuildID: 100, Type: discord.GuildText}
		m.state.Cabinet.MeStore.MyselfSet(discord.User{ID: 1, Username: "me", Nitro: discord.NoUserNitro}, true)

		transport.roundTrip = func(req *http.Request) (*http.Response, error) {
			if req.URL.Path != "/api/v9/guilds/100/emojis" {
				t.Fatalf("unexpected emoji fetch path %q", req.URL.Path)
			}
			return &http.Response{
				StatusCode: http.StatusInternalServerError,
				Body:       io.NopCloser(bytes.NewReader([]byte(`{"message":"boom"}`))),
				Header:     make(http.Header),
			}, nil
		}

		emojis := availableEmojisForChannel(m.state, channel)
		if len(emojis) != len(markdown.StandardEmojis) {
			t.Fatalf("expected fetch failure to return only standard emojis, got %d", len(emojis))
		}
	})
}
