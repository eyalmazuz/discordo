package chat

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"reflect"
	"strings"
	"sync"
	"unsafe"

	"github.com/ayn2op/discordo/internal/config"
	"github.com/ayn2op/tview"
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
)

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
		return &http.Response{StatusCode: http.StatusNoContent, Body: io.NopCloser(strings.NewReader("")), Header: make(http.Header)}, nil
	}
	if strings.Contains(req.URL.Path, "/messages") {
		data, _ := json.Marshal(t.messages)
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewReader(data)), Header: make(http.Header)}, nil
	}
	return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("[]")), Header: make(http.Header)}, nil
}

func newTestModelWithTransport(transport *mockTransport) *Model {
	cfg, _ := config.Load("")
	app := tview.NewApplication()
	m := NewModel(app, cfg, "token")

	driver := httpdriver.WrapClient(http.Client{Transport: transport})
	apiClient := api.NewCustomClient("token", httputil.NewClientWithDriver(driver))
	s := state.NewFromSession(session.NewCustom(gateway.DefaultIdentifier("token"), apiClient, handler.New()), defaultstore.New())
	m.state = ningen.FromState(s)
	m.state.Cabinet.MeStore.MyselfSet(discord.User{ID: 1}, false)
	return m
}

func newTestModel() *Model {
	return newTestModelWithTransport(&mockTransport{})
}

func execCmdForTest(app *tview.Application, cmd tview.Cmd) {
	if cmd == nil {
		return
	}
	msg := cmd()
	if msg == nil {
		return
	}
	value := reflect.ValueOf(msg)
	if value.Kind() == reflect.Slice && value.Type().Elem().Kind() == reflect.Func {
		for i := 0; i < value.Len(); i++ {
			cmd := reflect.NewAt(value.Type().Elem(), unsafe.Pointer(value.Index(i).UnsafeAddr())).Elem().Interface().(tview.Cmd)
			execCmdForTest(app, cmd)
		}
		return
	}
	setFocusMsgForTest(app, msg)
}

func setFocusMsgForTest(app *tview.Application, msg tview.Msg) {
	value := reflect.ValueOf(msg)
	if value.Kind() != reflect.Struct || value.Type().Name() != "setFocusMsg" {
		return
	}
	copyValue := reflect.New(value.Type()).Elem()
	copyValue.Set(value)
	value = copyValue
	targetField := value.FieldByName("target")
	if !targetField.IsValid() || targetField.IsNil() {
		return
	}
	target := reflect.NewAt(targetField.Type(), unsafe.Pointer(targetField.UnsafeAddr())).Elem().Interface().(tview.Model)

	appValue := reflect.ValueOf(app).Elem()
	focusField := appValue.FieldByName("focus")
	reflect.NewAt(focusField.Type(), unsafe.Pointer(focusField.UnsafeAddr())).Elem().Set(reflect.ValueOf(target))
}
