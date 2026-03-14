package chat

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	clipkg "github.com/ayn2op/discordo/internal/clipboard"
	"github.com/ayn2op/discordo/internal/config"
	"github.com/diamondburned/arikawa/v3/api"
	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/diamondburned/arikawa/v3/gateway"
	"github.com/diamondburned/arikawa/v3/session"
	"github.com/diamondburned/arikawa/v3/state"
	"github.com/diamondburned/arikawa/v3/state/store/defaultstore"
	"github.com/diamondburned/arikawa/v3/utils/handler"
	"github.com/diamondburned/arikawa/v3/utils/httputil"
	"github.com/diamondburned/arikawa/v3/utils/httputil/httpdriver"
	"github.com/diamondburned/ningen/v3"
	"github.com/ayn2op/tview"
	"github.com/gdamore/tcell/v3"
	"github.com/gdamore/tcell/v3/color"
)

type completeMockScreen struct {
	MockScreen
}

func (m *completeMockScreen) Init() error { return nil }
func (m *completeMockScreen) Fini() {}
func (m *completeMockScreen) Clear() {}
func (m *completeMockScreen) Fill(rune, tcell.Style) {}
func (m *completeMockScreen) Show() {}
func (m *completeMockScreen) CharacterSet() string { return "UTF-8" }
func (m *completeMockScreen) Size() (int, int) { return 80, 24 }
func (m *completeMockScreen) HasMouse() bool { return false }
func (m *completeMockScreen) HasKey(tcell.Key) bool { return true }
func (m *completeMockScreen) Colors() int { return 256 }
func (m *completeMockScreen) SetCursorStyle(tcell.CursorStyle, ...color.Color) {}
func (m *completeMockScreen) ShowCursor(x, y int) {}
func (m *completeMockScreen) HideCursor() {}
func (m *completeMockScreen) SetStyle(tcell.Style) {}
func (m *completeMockScreen) GetContent(x, y int) (rune, []rune, tcell.Style, int) { return ' ', nil, tcell.StyleDefault, 1 }
func (m *completeMockScreen) SetSize(int, int) {}
func (m *completeMockScreen) Channel() chan tcell.Event { return make(chan tcell.Event) }
func (m *completeMockScreen) EventQ() chan tcell.Event { return make(chan tcell.Event) }
func (m *completeMockScreen) PostEvent(tcell.Event) error { return nil }
func (m *completeMockScreen) PostEventWait(tcell.Event) {}
func (m *completeMockScreen) Sync() {}
func (m *completeMockScreen) Register() {}
func (m *completeMockScreen) Unregister() {}
func (m *completeMockScreen) EnableMouse(...tcell.MouseFlags) {}
func (m *completeMockScreen) DisableMouse() {}
func (m *completeMockScreen) EnablePaste() {}
func (m *completeMockScreen) DisablePaste() {}
func (m *completeMockScreen) Reload() {}
func (m *completeMockScreen) SetClip(x, y, w, h int) {}
func (m *completeMockScreen) GetClip() (int, int, int, int) { return 0, 0, 80, 24 }
func (m *completeMockScreen) SetAttributes(tcell.AttrMask) {}
func (m *completeMockScreen) Beep() error { return nil }
func (m *completeMockScreen) SetTitle(string) {}
func (m *completeMockScreen) Stop() {}
func (m *completeMockScreen) Suspend() error { return nil }
func (m *completeMockScreen) Pause() {}
func (m *completeMockScreen) Resume() error { return nil }
func (m *completeMockScreen) IsPaused() bool { return false }
func (m *completeMockScreen) Put(x, y int, s string, style tcell.Style) (string, int) { return s, len(s) }

func init() {
	openStart = func(string) error { return nil }
}

type mockTransport struct {
	messages  []discord.Message
	mu        sync.Mutex
	method    string
	path      string
	body      string
	roundTrip func(*http.Request) (*http.Response, error)
}

func (t *mockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	var body []byte
	if req.Body != nil {
		body, _ = io.ReadAll(req.Body)
		req.Body = io.NopCloser(bytes.NewReader(body))
	}

	t.mu.Lock()
	t.method = req.Method
	t.path = req.URL.Path
	t.body = string(body)
	t.mu.Unlock()

	if t.roundTrip != nil {
		return t.roundTrip(req)
	}

	if (req.Method == http.MethodPut || req.Method == http.MethodDelete) && strings.Contains(req.URL.Path, "/reactions/") {
		if strings.Contains(req.URL.Path, "999") {
			return &http.Response{
				StatusCode: 400,
				Body:       io.NopCloser(strings.NewReader(`{"message": "reaction fail"}`)),
				Header:     make(http.Header),
			}, nil
		}
		return &http.Response{
			StatusCode: http.StatusNoContent,
			Body:       io.NopCloser(strings.NewReader("")),
			Header:     make(http.Header),
		}, nil
	}
	if req.Method == "GET" && strings.Contains(req.URL.Path, "/users/@me/channels") {
		if req.Header.Get("Authorization") == "error-token" {
			return &http.Response{
				StatusCode: 401,
				Body:       io.NopCloser(strings.NewReader(`{"message": "Unauthorized"}`)),
				Header:     make(http.Header),
			}, nil
		}
	}
	if req.Method == "DELETE" && strings.Contains(req.URL.Path, "/messages/") {
		if strings.Contains(req.URL.Path, "999") {
			return &http.Response{
				StatusCode: 400,
				Body:       io.NopCloser(strings.NewReader(`{"message": "error"}`)),
				Header:     make(http.Header),
			}, nil
		}
		return &http.Response{
			StatusCode: 204,
			Body:       io.NopCloser(strings.NewReader("")),
			Header:     make(http.Header),
		}, nil
	}
	if (req.Method == http.MethodPost && strings.HasSuffix(req.URL.Path, "/messages")) || (req.Method == http.MethodPatch && strings.Contains(req.URL.Path, "/messages/")) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{}`)),
			Header:     make(http.Header),
		}, nil
	}
	if strings.Contains(req.URL.Path, "/messages") {
		data, _ := json.Marshal(t.messages)
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(bytes.NewReader(data)),
			Header:     make(http.Header),
		}, nil
	}
	return &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(strings.NewReader("[]")),
		Header:     make(http.Header),
	}, nil
}

func newTestModelWithTransport(transport *mockTransport) *Model {
	return newTestModelWithTokenAndTransport("token", transport)
}

func newTestModelWithTokenAndTransport(token string, transport *mockTransport) *Model {
	cfg, _ := config.Load("")
	app := tview.NewApplication()
	m := NewView(app, cfg, token)

	driver := httpdriver.WrapClient(http.Client{Transport: transport})
	apiClient := api.NewCustomClient(token, httputil.NewClientWithDriver(driver))
	s := state.NewFromSession(session.NewCustom(gateway.DefaultIdentifier(token), apiClient, handler.New()), defaultstore.New())
	m.state = ningen.FromState(s)
	m.state.Cabinet.MeStore.MyselfSet(discord.User{ID: 1}, false)

	app.SetScreen(&completeMockScreen{})
	go app.Run()

	return m
}

func newTestModelWithMessages(msgs []discord.Message) *Model {
	return newTestModelWithTransport(&mockTransport{messages: msgs})
}

func newTestModel() *Model {
	return newTestModelWithMessages(nil)
}

func newMockChatModel() *Model {
	return newTestModel()
}

type mockEmoteScreen struct {
	MockScreen
	cells map[string]string // "x,y" -> url
}

func (m *mockEmoteScreen) Get(x, y int) (string, tcell.Style, int) {
	style := tcell.StyleDefault
	if url, ok := m.cells[fmt.Sprintf("%d,%d", x, y)]; ok {
		style = style.Url(url)
	}
	return " ", style, 1
}


type mockTty struct {
	strings.Builder
}

func (m *mockTty) Close() error                          { return nil }
func (m *mockTty) Read(p []byte) (n int, err error)      { return 0, nil }
func (m *mockTty) Size() (int, int, error)               { return 80, 24, nil }
func (m *mockTty) Drain() error                          { return nil }
func (m *mockTty) NotifyResize(chan<- bool)              {}
func (m *mockTty) Stop() error                           { return nil }
func (m *mockTty) Start() error                          { return nil }
func (m *mockTty) WindowSize() (tcell.WindowSize, error) { return tcell.WindowSize{}, nil }

type screenWithTty struct {
	completeMockScreen
	tty *mockTty
}

func (s *screenWithTty) Tty() (tcell.Tty, bool) { return s.tty, true }

func stubClipboardWrite(t *testing.T) <-chan string {
	t.Helper()

	oldClipboardWrite := clipboardWrite
	copied := make(chan string, 8)
	clipboardWrite = func(_ clipkg.Format, data []byte) error {
		copied <- string(data)
		return nil
	}
	t.Cleanup(func() {
		clipboardWrite = oldClipboardWrite
	})

	return copied
}

func waitForCopiedText(t *testing.T, copied <-chan string) string {
	t.Helper()

	select {
	case text := <-copied:
		return text
	case <-time.After(200 * time.Millisecond):
		t.Fatal("timed out waiting for clipboard write")
		return ""
	}
}

func setPermissionsForUser(m *Model, guildID discord.GuildID, channel *discord.Channel, user discord.User, perms discord.Permissions) {
	roleID := discord.RoleID(user.ID)
	m.state.Cabinet.GuildStore.GuildSet(&discord.Guild{ID: guildID}, false)
	m.state.Cabinet.ChannelStore.ChannelSet(channel, false)
	m.state.Cabinet.MemberStore.MemberSet(guildID, &discord.Member{
		User:    user,
		RoleIDs: []discord.RoleID{roleID},
	}, false)
	m.state.Cabinet.RoleStore.RoleSet(guildID, &discord.Role{
		ID:          roleID,
		Permissions: perms,
	}, false)
}
