package chat

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/diamondburned/arikawa/v3/api"
	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/eyalmazuz/tview"
	"github.com/gdamore/tcell/v3"
)

func TestMessageSearchPopupHelpers(t *testing.T) {
	channel := discord.Channel{ID: 55, GuildID: 77}

	group := []discord.Message{
		{ID: 0, ChannelID: channel.ID, Content: "ignored invalid id"},
		{ID: 1, ChannelID: 999, Content: "wrong channel"},
		{ID: 2, ChannelID: channel.ID, Content: "fallback result"},
		{ID: 3, ChannelID: channel.ID, Content: "contains needle"},
	}

	got, ok := pickSearchResultMessage(group, channel, "needle")
	if !ok || got.ID != 3 {
		t.Fatalf("expected matching message ID 3, got %v ok=%v", got.ID, ok)
	}

	got, ok = pickSearchResultMessage(group, channel, "missing")
	if !ok || got.ID != 2 {
		t.Fatalf("expected fallback message ID 2, got %v ok=%v", got.ID, ok)
	}

	if _, ok := pickSearchResultMessage([]discord.Message{{ID: 0}}, channel, "missing"); ok {
		t.Fatal("expected no valid message when all candidates are invalid")
	}

	if preview := compactMessagePreview(discord.Message{Content: " hello   world "}); preview != "hello world" {
		t.Fatalf("expected compacted whitespace preview, got %q", preview)
	}
	if preview := compactMessagePreview(discord.Message{Attachments: []discord.Attachment{{Filename: "a.txt"}}}); preview != "[attachment]" {
		t.Fatalf("expected attachment preview, got %q", preview)
	}
	if preview := compactMessagePreview(discord.Message{Embeds: []discord.Embed{{Title: "embed"}}}); preview != "[embed]" {
		t.Fatalf("expected embed preview, got %q", preview)
	}
	if preview := compactMessagePreview(discord.Message{}); preview != "[no text]" {
		t.Fatalf("expected empty preview fallback, got %q", preview)
	}

	line := tview.Line{{Text: "segment", Style: tcell.StyleDefault.Url("https://example.com")}}
	reversed := reverseSearchLine(line)
	if len(reversed) != 1 {
		t.Fatalf("expected one reversed segment, got %d", len(reversed))
	}
	if _, url := reversed[0].Style.GetUrl(); url != "https://example.com" {
		t.Fatalf("expected URL to be preserved, got %q", url)
	}
}

func TestMessageSearchPopupSearchBranches(t *testing.T) {
	channel := discord.Channel{ID: 200, GuildID: 100, Type: discord.GuildText, Name: "general"}
	m := newMockChatModel()
	sp := newMessageSearchPopup(m.cfg, m, m.messagesList)
	sp.queueUpdateDraw = func(f func()) { f() }

	sp.Prepare(discord.Channel{}, nil)
	sp.input.SetText("ignored")
	sp.search()
	if sp.lastSubmitted != "" {
		t.Fatalf("expected invalid channel search to keep lastSubmitted empty, got %q", sp.lastSubmitted)
	}

	sp.Prepare(channel, nil)
	sp.input.SetText("   ")
	sp.search()
	if sp.status != "Type a query and press Enter" {
		t.Fatalf("expected blank search to reset prompt, got %q", sp.status)
	}

	sp.searchMessages = func(discord.Channel, string) ([]messageSearchResult, error) {
		return nil, errors.New("search boom")
	}
	sp.input.SetText("boom")
	sp.search()
	deadline := time.Now().Add(300 * time.Millisecond)
	for sp.status != "Search failed" && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if sp.status != "Search failed" {
		t.Fatalf("expected failed search status, got %q", sp.status)
	}
	if len(sp.results) != 0 {
		t.Fatalf("expected failed search to keep zero results, got %d", len(sp.results))
	}
}

func TestMessageSearchPopupSearchIgnoresStaleResponses(t *testing.T) {
	m := newMockChatModel()
	channel := discord.Channel{ID: 200, GuildID: 100, Type: discord.GuildText, Name: "general"}
	sp := newMessageSearchPopup(m.cfg, m, m.messagesList)
	sp.Prepare(channel, m.messagesList)
	sp.queueUpdateDraw = func(f func()) { f() }

	release := map[string]chan struct{}{
		"first":  make(chan struct{}),
		"second": make(chan struct{}),
	}
	started := make(chan string, 2)
	sp.searchMessages = func(_ discord.Channel, query string) ([]messageSearchResult, error) {
		started <- query
		<-release[query]
		id := discord.MessageID(1)
		if query == "second" {
			id = 2
		}
		return []messageSearchResult{{
			Message: discord.Message{
				ID:        id,
				ChannelID: channel.ID,
				GuildID:   channel.GuildID,
				Content:   query,
				Timestamp: discord.NewTimestamp(time.Unix(0, 0)),
				Author:    discord.User{Username: query},
			},
		}}, nil
	}

	sp.input.SetText("first")
	sp.search()
	<-started

	sp.input.SetText("second")
	sp.search()
	<-started

	close(release["second"])
	deadline := time.Now().Add(300 * time.Millisecond)
	for len(sp.results) != 1 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if len(sp.results) != 1 || sp.results[0].Message.ID != 2 {
		t.Fatalf("expected second search result to win, got %+v", sp.results)
	}

	close(release["first"])
	time.Sleep(20 * time.Millisecond)
	if len(sp.results) != 1 || sp.results[0].Message.ID != 2 {
		t.Fatalf("expected stale first search result to be ignored, got %+v", sp.results)
	}
}

func TestMessageSearchPopupFetchSearchResults(t *testing.T) {
	jsonResponse := func(v any) (*http.Response, error) {
		data, err := json.Marshal(v)
		if err != nil {
			return nil, err
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(bytes.NewReader(data)),
			Header:     make(http.Header),
		}, nil
	}

	transport := &mockTransport{}
	m := newTestModelWithTransport(transport)
	sp := newMessageSearchPopup(m.cfg, m, m.messagesList)

	transport.roundTrip = func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/api/v9/guilds/100/messages/search":
			switch req.URL.Query().Get("offset") {
			case "":
				return jsonResponse(api.SearchResponse{
					Messages: [][]discord.Message{
						{{ID: 1, ChannelID: 200, Content: "needle first", Author: discord.User{Username: "one"}}},
						{{ID: 2, ChannelID: 999, Content: "wrong"}, {ID: 3, ChannelID: 200, Content: "fallback", Author: discord.User{Username: "two"}}},
					},
					TotalResults: 4,
				})
			case "2":
				return jsonResponse(api.SearchResponse{
					Messages: [][]discord.Message{
						{{ID: 1, ChannelID: 200, Content: "needle duplicate", Author: discord.User{Username: "dup"}}},
						{{ID: 4, ChannelID: 200, Content: "needle second page", Author: discord.User{Username: "three"}}},
					},
					TotalResults: 4,
				})
			default:
				t.Fatalf("unexpected guild search offset %q", req.URL.Query().Get("offset"))
			}
		case "/api/v9/channels/300/messages/search":
			return jsonResponse(api.SearchResponse{
				Messages: [][]discord.Message{
					{{ID: 10, ChannelID: 300, Content: "dm result", Author: discord.User{Username: "friend"}}},
				},
				TotalResults: 1,
			})
		default:
			t.Fatalf("unexpected search path %q", req.URL.Path)
		}
		return nil, nil
	}

	guildResults, err := sp.fetchSearchResults(discord.Channel{ID: 200, GuildID: 100, Type: discord.GuildText}, "needle")
	if err != nil {
		t.Fatalf("expected guild search to succeed, got %v", err)
	}
	if len(guildResults) != 3 {
		t.Fatalf("expected 3 deduplicated guild results, got %d", len(guildResults))
	}
	for i, want := range []discord.MessageID{1, 3, 4} {
		if guildResults[i].Message.ID != want {
			t.Fatalf("expected guild result %d to be message %v, got %v", i, want, guildResults[i].Message.ID)
		}
	}

	dmResults, err := sp.fetchSearchResults(discord.Channel{ID: 300, Type: discord.DirectMessage}, "dm")
	if err != nil {
		t.Fatalf("expected DM search to succeed, got %v", err)
	}
	if len(dmResults) != 1 || dmResults[0].Message.ID != 10 {
		t.Fatalf("expected one DM result with ID 10, got %+v", dmResults)
	}
}
