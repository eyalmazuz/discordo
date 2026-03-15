package chat

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/eyalmazuz/tview"
	"github.com/eyalmazuz/tview/layers"
	"github.com/diamondburned/arikawa/v3/api"
	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/gdamore/tcell/v3"
)

func TestMessageSearchPopupHandleEvent_NavigationAndCancel(t *testing.T) {
	m := newMockChatModel()
	channel := discord.Channel{ID: 200, GuildID: 100, Type: discord.GuildText, Name: "general"}
	sp := newMessageSearchPopup(m.cfg, m, m.messagesList)
	m.messageSearch = sp

	sp.Prepare(channel, m.messageInput)
	sp.setResults([]messageSearchResult{
		{Message: discord.Message{ID: 1, ChannelID: channel.ID, Content: "first", Timestamp: discord.NewTimestamp(time.Unix(0, 0)), Author: discord.User{Username: "one"}}},
		{Message: discord.Message{ID: 2, ChannelID: channel.ID, Content: "second", Timestamp: discord.NewTimestamp(time.Unix(0, 0)), Author: discord.User{Username: "two"}}},
	})

	m.AddLayer(sp, layers.WithName(messageSearchLayerName), layers.WithVisible(true))
	m.app.SetFocus(sp.input)

	sp.HandleEvent(tcell.NewEventKey(tcell.KeyTab, "", tcell.ModNone))
	if m.app.GetFocus() != sp.list {
		t.Fatalf("expected focus on results list, got %T", m.app.GetFocus())
	}

	sp.HandleEvent(tcell.NewEventKey(tcell.KeyEnd, "", tcell.ModNone))
	sp.HandleEvent(tcell.NewEventKey(tcell.KeyHome, "", tcell.ModNone))
	sp.HandleEvent(tcell.NewEventKey(tcell.KeyRune, "j", tcell.ModNone))
	sp.HandleEvent(tcell.NewEventKey(tcell.KeyRune, "k", tcell.ModNone))
	if m.app.GetFocus() != sp.list {
		t.Fatalf("expected focus to remain on results list, got %T", m.app.GetFocus())
	}

	sp.HandleEvent(tcell.NewEventKey(tcell.KeyEscape, "", tcell.ModNone))
	if m.HasLayer(messageSearchLayerName) {
		t.Fatal("expected popup layer to close on cancel")
	}
	if m.app.GetFocus() != m.messageInput {
		t.Fatalf("expected focus to return to previous primitive, got %T", m.app.GetFocus())
	}
}

func TestMessageSearchPopupHandleEvent_SelectFromList(t *testing.T) {
	m := newMockChatModel()
	channel := discord.Channel{ID: 200, GuildID: 100, Type: discord.GuildText, Name: "general"}
	sp := newMessageSearchPopup(m.cfg, m, m.messagesList)
	m.messageSearch = sp

	sp.Prepare(channel, m.messageInput)
	sp.setResults([]messageSearchResult{
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
	})
	m.AddLayer(sp, layers.WithName(messageSearchLayerName), layers.WithVisible(true))
	m.app.SetFocus(sp.list)

	selected := false
	sp.jumpToMessage = func(discord.Channel, discord.MessageID) error {
		selected = true
		return nil
	}

	sp.HandleEvent(tcell.NewEventKey(tcell.KeyEnter, "", tcell.ModNone))
	if !selected {
		t.Fatal("expected Enter on the results list to select the current item")
	}
}

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

func TestMessageSearchPopupBuildItemAndSelectionBranches(t *testing.T) {
	m := newMockChatModel()
	channel := discord.Channel{ID: 200, GuildID: 100, Type: discord.GuildText, Name: "general"}
	sp := newMessageSearchPopup(m.cfg, m, m.messagesList)
	m.messageSearch = sp

	sp.Prepare(channel, m.messagesList)
	sp.setStatus("Searching...", tcell.StyleDefault.Dim(true))

	if item := sp.buildItem(1, 0); item != nil {
		t.Fatal("expected nil item for out-of-range status row")
	}
	if item := sp.buildItem(0, 0); item == nil {
		t.Fatal("expected status item while no results are present")
	}

	result := messageSearchResult{
		Message: discord.Message{
			ID:        300,
			ChannelID: channel.ID,
			GuildID:   channel.GuildID,
			Content:   "hello world",
			Timestamp: discord.NewTimestamp(time.Unix(0, 0)),
			Author:    discord.User{ID: 10, Username: "user"},
		},
	}
	sp.setResults([]messageSearchResult{result})
	if item := sp.buildItem(0, 0); item == nil {
		t.Fatal("expected rendered item for search result")
	}
	if item := sp.buildItem(2, 0); item != nil {
		t.Fatal("expected nil item for out-of-range result row")
	}

	line := sp.lineForResult(result)
	if len(line) != 3 {
		t.Fatalf("expected timestamp, author, and preview segments, got %d", len(line))
	}
	if line[1].Text != "user: " {
		t.Fatalf("expected author segment, got %q", line[1].Text)
	}

	m.AddLayer(sp, layers.WithName(messageSearchLayerName), layers.WithVisible(true))
	sp.jumpToMessage = func(discord.Channel, discord.MessageID) error {
		return errors.New("jump failed")
	}
	sp.list.SetCursor(0)
	sp.selectCurrent()
	if !m.HasLayer(messageSearchLayerName) {
		t.Fatal("expected popup to stay open when jump fails")
	}

	sp.list.SetCursor(-1)
	sp.selectCurrent()
}

func TestMessageSearchPopupOnInputChanged(t *testing.T) {
	m := newMockChatModel()
	channel := discord.Channel{ID: 200, GuildID: 100, Type: discord.GuildText, Name: "general"}
	sp := newMessageSearchPopup(m.cfg, m, m.messagesList)

	sp.Prepare(channel, m.messagesList)
	sp.lastSubmitted = "same"
	sp.results = []messageSearchResult{{Message: discord.Message{ID: 1}}}
	sp.onInputChanged("same")
	if len(sp.results) != 1 {
		t.Fatal("expected unchanged query to keep results")
	}

	sp.onInputChanged("")
	if sp.status != "Type a query and press Enter" {
		t.Fatalf("expected prompt reset for empty query, got %q", sp.status)
	}

	sp.results = []messageSearchResult{{Message: discord.Message{ID: 2}}}
	sp.onInputChanged("new")
	if len(sp.results) != 0 {
		t.Fatal("expected results to clear after new query text")
	}
	if sp.status != "Press Enter to search this channel" {
		t.Fatalf("expected search hint for non-empty query, got %q", sp.status)
	}
}

func TestMessageSearchPopupHelpAndEnqueueUpdateDraw(t *testing.T) {
	m := newMockChatModel()
	channel := discord.Channel{ID: 200, GuildID: 100, Type: discord.GuildText, Name: "general"}
	sp := newMessageSearchPopup(m.cfg, m, m.messagesList)
	sp.Prepare(channel, m.messagesList)

	if len(sp.ShortHelp()) == 0 || len(sp.FullHelp()) == 0 {
		t.Fatal("expected popup help to be populated")
	}

	runs := 0
	sp.queueUpdateDraw = func(f func()) {
		runs++
		f()
	}
	sp.enqueueUpdateDraw(func() { runs++ })
	if runs != 2 {
		t.Fatalf("expected queueUpdateDraw path to execute callback, got %d runs", runs)
	}

	(&messageSearchPopup{}).enqueueUpdateDraw(func() {})
}

func TestMessageSearchPopupFetchSearchResults_UsesDiscordSearchEndpoints(t *testing.T) {
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
			if got := req.URL.Query().Get("content"); got != "needle" {
				t.Fatalf("expected guild search query %q, got %q", "needle", got)
			}

			switch req.URL.Query().Get("offset") {
			case "":
				return jsonResponse(api.SearchResponse{
					Messages: [][]discord.Message{
						{{ID: 1, ChannelID: 200, Content: "needle first", Author: discord.User{Username: "one"}}},
						{
							{ID: 2, ChannelID: 999, Content: "needle wrong channel", Author: discord.User{Username: "skip"}},
							{ID: 3, ChannelID: 200, Content: "fallback", Author: discord.User{Username: "two"}},
						},
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
			if got := req.URL.Query().Get("content"); got != "dm" {
				t.Fatalf("expected DM search query %q, got %q", "dm", got)
			}
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
		if guildResults[i].Message.GuildID != 100 {
			t.Fatalf("expected guild result %d to inherit guild ID 100, got %v", i, guildResults[i].Message.GuildID)
		}
	}

	dmResults, err := sp.fetchSearchResults(discord.Channel{ID: 300, Type: discord.DirectMessage}, "dm")
	if err != nil {
		t.Fatalf("expected DM search to succeed, got %v", err)
	}
	if len(dmResults) != 1 || dmResults[0].Message.ID != 10 {
		t.Fatalf("expected one DM result with ID 10, got %+v", dmResults)
	}
	if dmResults[0].Message.GuildID.IsValid() {
		t.Fatalf("expected DM result to keep invalid guild ID, got %v", dmResults[0].Message.GuildID)
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

func TestMessageSearchPopupSearch_IgnoresStaleResponses(t *testing.T) {
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

func TestMessageSearchPopupHandleEvent_ListSelectionBranches(t *testing.T) {
	m := newMockChatModel()
	channel := discord.Channel{ID: 200, GuildID: 100, Type: discord.GuildText, Name: "general"}
	sp := newMessageSearchPopup(m.cfg, m, m.messagesList)
	m.messageSearch = sp

	sp.Prepare(channel, m.messagesList)
	sp.setResults([]messageSearchResult{
		{Message: discord.Message{ID: 1, ChannelID: channel.ID, Content: "first", Timestamp: discord.NewTimestamp(time.Unix(0, 0)), Author: discord.User{Username: "one"}}},
		{Message: discord.Message{ID: 2, ChannelID: channel.ID, Content: "second", Timestamp: discord.NewTimestamp(time.Unix(0, 0)), Author: discord.User{Username: "two"}}},
	})

	m.AddLayer(sp, layers.WithName(messageSearchLayerName), layers.WithVisible(true))
	m.app.SetFocus(sp.list)

	var selected discord.MessageID
	sp.jumpToMessage = func(_ discord.Channel, messageID discord.MessageID) error {
		selected = messageID
		return nil
	}

	sp.HandleEvent(tcell.NewEventKey(tcell.KeyTab, "", tcell.ModNone))
	if m.app.GetFocus() != sp.input {
		t.Fatalf("expected focus on input after tab, got %T", m.app.GetFocus())
	}

	sp.HandleEvent(tcell.NewEventKey(tcell.KeyTab, "", tcell.ModNone))
	sp.HandleEvent(tcell.NewEventKey(tcell.KeyDown, "", tcell.ModNone))
	sp.HandleEvent(tcell.NewEventKey(tcell.KeyRune, "G", tcell.ModNone))
	if sp.list.Cursor() != 1 {
		t.Fatalf("expected G to move to the last result, got cursor %d", sp.list.Cursor())
	}

	sp.HandleEvent(tcell.NewEventKey(tcell.KeyEnter, "", tcell.ModNone))
	if selected != 2 {
		t.Fatalf("expected enter on the list to select message 2, got %v", selected)
	}
	if m.HasLayer(messageSearchLayerName) {
		t.Fatal("expected enter on the list to close the popup")
	}
}

func TestMessageSearchPopupHandleEvent_NonKeyFallsBackToFlex(t *testing.T) {
	m := newMockChatModel()
	sp := newMessageSearchPopup(m.cfg, m, m.messagesList)
	m.app.SetFocus(sp.input)

	if cmd := sp.HandleEvent(&tview.MouseEvent{}); cmd != nil {
		t.Fatalf("expected non-key event to fall back without issuing a command, got %T", cmd)
	}
}

func TestMessageSearchPopupAdditionalBranches(t *testing.T) {
	t.Run("handle event covers up down and g shortcuts", func(t *testing.T) {
		m := newMockChatModel()
		channel := discord.Channel{ID: 200, GuildID: 100, Type: discord.GuildText, Name: "general"}
		sp := newMessageSearchPopup(m.cfg, m, m.messagesList)
		sp.Prepare(channel, m.messageInput)
		sp.setResults([]messageSearchResult{
			{Message: discord.Message{ID: 1, ChannelID: channel.ID, Content: "first", Timestamp: discord.NewTimestamp(time.Unix(0, 0)), Author: discord.User{Username: "one"}}},
			{Message: discord.Message{ID: 2, ChannelID: channel.ID, Content: "second", Timestamp: discord.NewTimestamp(time.Unix(0, 0)), Author: discord.User{Username: "two"}}},
		})
		m.AddLayer(sp, layers.WithName(messageSearchLayerName), layers.WithVisible(true))
		sp.list.Focus(nil)
		m.app.SetFocus(sp.list)

		for _, event := range []*tcell.EventKey{
			tcell.NewEventKey(tcell.KeyCtrlP, "", tcell.ModNone),
			tcell.NewEventKey(tcell.KeyCtrlN, "", tcell.ModNone),
			tcell.NewEventKey(tcell.KeyRune, "g", tcell.ModNone),
		} {
			sp.HandleEvent(event)
		}
	})

	t.Run("search ignores callback when input text changed", func(t *testing.T) {
		m := newMockChatModel()
		channel := discord.Channel{ID: 200, GuildID: 100, Type: discord.GuildText, Name: "general"}
		sp := newMessageSearchPopup(m.cfg, m, m.messagesList)
		sp.Prepare(channel, m.messagesList)
		release := make(chan struct{})
		sp.queueUpdateDraw = func(f func()) { f() }
		sp.searchMessages = func(discord.Channel, string) ([]messageSearchResult, error) {
			<-release
			return []messageSearchResult{{
				Message: discord.Message{
					ID:        1,
					ChannelID: channel.ID,
					GuildID:   channel.GuildID,
					Content:   "needle",
					Timestamp: discord.NewTimestamp(time.Unix(0, 0)),
					Author:    discord.User{Username: "user"},
				},
			}}, nil
		}

		sp.input.SetText("needle")
		sp.search()
		sp.input.SetText("changed")
		close(release)
		time.Sleep(10 * time.Millisecond)
		if len(sp.results) != 0 {
			t.Fatalf("expected changed input to suppress stale callback results, got %+v", sp.results)
		}
	})

	t.Run("fetchSearchResults handles empty pages invalid groups and errors", func(t *testing.T) {
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
			if req.URL.Path != "/api/v9/channels/300/messages/search" {
				t.Fatalf("unexpected search path %q", req.URL.Path)
			}
			switch req.URL.Query().Get("content") {
			case "bad":
				return &http.Response{
					StatusCode: http.StatusBadRequest,
					Body:       io.NopCloser(strings.NewReader(`{"message":"boom"}`)),
					Header:     make(http.Header),
				}, nil
			case "empty":
				return jsonResponse(api.SearchResponse{})
			default:
				return jsonResponse(api.SearchResponse{
					Messages: [][]discord.Message{
						{{ID: 0, ChannelID: 300, Content: "invalid"}},
						{{ID: 1, ChannelID: 999, Content: "wrong channel"}},
					},
					TotalResults: 2,
				})
			}
		}

		if results, err := sp.fetchSearchResults(discord.Channel{ID: 300, Type: discord.DirectMessage}, "empty"); err != nil || len(results) != 0 {
			t.Fatalf("expected empty search page to return no results, got results=%+v err=%v", results, err)
		}
		if results, err := sp.fetchSearchResults(discord.Channel{ID: 300, Type: discord.DirectMessage}, "filtered"); err != nil || len(results) != 0 {
			t.Fatalf("expected invalid search groups to be skipped, got results=%+v err=%v", results, err)
		}
		if _, err := sp.fetchSearchResults(discord.Channel{ID: 300, Type: discord.DirectMessage}, "bad"); err == nil {
			t.Fatal("expected DM search error to be returned")
		}
	})

	t.Run("selectCurrent uses default jump and enqueueUpdateDraw uses app", func(t *testing.T) {
		transport := &mockTransport{
			messages: []discord.Message{{ID: 500, ChannelID: 200, Content: "target", Author: discord.User{ID: 1, Username: "me"}}},
		}
		m := newTestModelWithTransport(transport)
		channel := discord.Channel{ID: 200, Type: discord.DirectMessage, Name: "dm"}
		m.SetSelectedChannel(&channel)
		sp := newMessageSearchPopup(m.cfg, m, m.messagesList)
		m.messageSearch = sp
		sp.Prepare(channel, m.messageInput)
		sp.results = []messageSearchResult{{
			Message: discord.Message{
				ID:        500,
				ChannelID: channel.ID,
				Content:   "target",
				Timestamp: discord.NewTimestamp(time.Unix(0, 0)),
				Author:    discord.User{Username: "me"},
			},
		}}
		sp.list.SetCursor(0)
		m.AddLayer(sp, layers.WithName(messageSearchLayerName), layers.WithVisible(true))

		done := make(chan struct{}, 1)
		sp.enqueueUpdateDraw(func() { done <- struct{}{} })
		select {
		case <-done:
		case <-time.After(300 * time.Millisecond):
			t.Fatal("expected enqueueUpdateDraw default app path to run callback")
		}

		sp.selectCurrent()
		if m.HasLayer(messageSearchLayerName) {
			t.Fatal("expected default jump path to close the popup")
		}
	})
}
