package chat

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
	"unsafe"

	"github.com/ayn2op/discordo/internal/config"
	"github.com/ayn2op/tview"
	"github.com/ayn2op/tview/layers"
	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/diamondburned/arikawa/v3/gateway"
	"github.com/diamondburned/arikawa/v3/session"
	"github.com/diamondburned/arikawa/v3/state"
	"github.com/diamondburned/arikawa/v3/state/store/defaultstore"
	"github.com/diamondburned/ningen/v3"
	"github.com/diamondburned/ningen/v3/states/read"
	"github.com/gdamore/tcell/v3"
)

func getAfterDrawFunc(app *tview.Application) func(tcell.Screen) {
	field := reflect.ValueOf(app).Elem().FieldByName("afterDrawFunc")
	return reflect.NewAt(field.Type(), unsafe.Pointer(field.UnsafeAddr())).Elem().Interface().(func(tcell.Screen))
}

func TestModel_Branches(t *testing.T) {
	m := newTestModel()

	t.Run("hasPopupOverlay", func(t *testing.T) {
		if m.hasPopupOverlay() {
			t.Errorf("Expected false initially")
		}
		m.openPicker()
		if !m.hasPopupOverlay() {
			t.Errorf("Expected true after openPicker")
		}
	})

	t.Run("togglePicker", func(t *testing.T) {
		m.togglePicker() // should close it
		if m.HasLayer(channelsPickerLayerName) {
			t.Errorf("Expected picker to be closed")
		}
		m.togglePicker() // should open it
		if !m.HasLayer(channelsPickerLayerName) {
			t.Errorf("Expected picker to be open")
		}
	})

	t.Run("toggleGuildsTree", func(t *testing.T) {
		m.toggleGuildsTree()
		if m.mainFlex.GetItemCount() != 1 {
			t.Errorf("Expected 1 item after toggle off")
		}
		m.toggleGuildsTree()
		if m.mainFlex.GetItemCount() != 2 {
			t.Errorf("Expected 2 items after toggle on")
		}
	})

	t.Run("focusPreviousNext_Branches", func(t *testing.T) {
		m.app.SetFocus(m.guildsTree)
		m.focusPrevious()
		if m.app.GetFocus() != m.messagesList {
			t.Errorf("Expected messagesList, got %T", m.app.GetFocus())
		}

		m.app.SetFocus(m.messageInput)
		m.focusPrevious()
		if m.app.GetFocus() != m.messagesList {
			t.Errorf("Expected messagesList, got %T", m.app.GetFocus())
		}

		m.app.SetFocus(m.messageInput)
		m.focusNext()
		if m.app.GetFocus() != m.guildsTree {
			t.Errorf("Expected guildsTree, got %T", m.app.GetFocus())
		}
	})

	t.Run("showConfirmModal", func(t *testing.T) {
		m.showConfirmModal("test", []string{"OK"}, nil)
		if !m.HasLayer(confirmModalLayerName) {
			t.Errorf("Expected confirm modal")
		}
	})

	t.Run("onReadUpdate_Branches", func(t *testing.T) {
		m.guildsTree.guildNodeByID[1] = tview.NewTreeNode("G")
		m.guildsTree.channelNodeByID[2] = tview.NewTreeNode("C")

		ev := &read.UpdateEvent{
			ReadState: gateway.ReadState{ChannelID: 2},
			GuildID:   1,
		}
		m.onReadUpdate(ev)
		time.Sleep(10 * time.Millisecond)
	})
}

func TestModelHandleEventCloseLayerEvent(t *testing.T) {
	m := newTestModel()
	m.AddLayer(tview.NewBox(), layers.WithName("test"), layers.WithVisible(true))
	if cmd := m.HandleEvent(&closeLayerEvent{name: "test"}); cmd != nil {
		t.Fatalf("expected close layer event to return nil, got %T", cmd)
	}
	if m.GetVisible("test") {
		t.Fatal("expected close layer event to hide the layer")
	}
}

func TestModel_HandleEvent_ExtraKeys(t *testing.T) {
	m := newTestModel()

	// Quit event
	m.HandleEvent(&QuitEvent{})

	// ModalDone event
	m.showConfirmModal("test", []string{"Yes"}, nil)
	m.HandleEvent(&tview.ModalDoneEvent{ButtonLabel: "Yes"})

	// Init event
	m.HandleEvent(&tview.InitEvent{})

	// All main keybinds
	keys := []tcell.Key{
		tcell.KeyCtrlG,
		tcell.KeyCtrlT,
		tcell.KeyCtrlI,
		tcell.KeyCtrlU,
		tcell.KeyCtrlO,
		tcell.KeyCtrlQ,
		tcell.KeyCtrlF,
		tcell.KeyCtrlP,
	}
	for _, k := range keys {
		m.HandleEvent(tcell.NewEventKey(k, "", tcell.ModNone))
	}
}

func TestModel_Typers_Footer_AllBranches(t *testing.T) {
	m := newTestModel()
	cid := discord.ChannelID(123)
	m.SetSelectedChannel(&discord.Channel{ID: cid})

	// 1 typer
	m.SelectedChannel().DMRecipients = []discord.User{{ID: 2, Username: "user2"}}
	m.addTyper(2)
	time.Sleep(10 * time.Millisecond)

	// 2 typers
	m.SelectedChannel().DMRecipients = append(m.SelectedChannel().DMRecipients, discord.User{ID: 3, Username: "user3"})
	m.addTyper(3)
	time.Sleep(10 * time.Millisecond)

	// 3 typers
	m.SelectedChannel().DMRecipients = append(m.SelectedChannel().DMRecipients, discord.User{ID: 4, Username: "user4"})
	m.addTyper(4)
	time.Sleep(10 * time.Millisecond)

	// 4 typers
	m.SelectedChannel().DMRecipients = append(m.SelectedChannel().DMRecipients, discord.User{ID: 5, Username: "user5"})
	m.addTyper(5)
	time.Sleep(10 * time.Millisecond)

	// guild member case
	m.SelectedChannel().GuildID = 10
	m.state.Cabinet.MemberStore.MemberSet(10, &discord.Member{User: discord.User{ID: 6, Username: "member6"}}, false)
	m.addTyper(6)
	time.Sleep(10 * time.Millisecond)

	m.removeTyper(2)
	m.clearTypers()
}

func TestModelOpenMessageSearchBranches(t *testing.T) {
	m := newTestModel()
	channel := &discord.Channel{ID: 123, Type: discord.DirectMessage}

	t.Run("no selected channel", func(t *testing.T) {
		m.SetSelectedChannel(nil)
		m.openMessageSearch()
		if m.HasLayer(messageSearchLayerName) {
			t.Fatal("expected no message search layer without a selected channel")
		}
	})

	t.Run("opens normally", func(t *testing.T) {
		m.SetSelectedChannel(channel)
		m.openMessageSearch()
		if !m.HasLayer(messageSearchLayerName) {
			t.Fatal("expected message search layer to be opened")
		}
	})

	t.Run("focuses existing popup", func(t *testing.T) {
		m.SetSelectedChannel(channel)
		m.openMessageSearch()
		if !m.GetVisible(messageSearchLayerName) {
			t.Fatal("expected message search layer to remain visible")
		}
	})

	t.Run("blocked by channels picker", func(t *testing.T) {
		m.RemoveLayer(messageSearchLayerName)
		m.SetSelectedChannel(channel)
		m.openPicker()
		m.openMessageSearch()
		if m.HasLayer(messageSearchLayerName) {
			t.Fatal("expected channels picker overlay to block message search")
		}
		m.closePicker()
	})
}

func TestModelHandleEventConfirmModalAndLogout(t *testing.T) {
	m := newTestModel()
	channel := &discord.Channel{ID: 123, Type: discord.DirectMessage}
	m.SetSelectedChannel(channel)

	t.Run("confirm modal restores focus and runs callback", func(t *testing.T) {
		m.app.SetFocus(m.messageInput)
		called := ""
		m.showConfirmModal("confirm", []string{"Yes", "No"}, func(label string) {
			called = label
		})

		m.HandleEvent(&tview.ModalDoneEvent{ButtonLabel: "No"})
		if called != "No" {
			t.Fatalf("expected modal callback label %q, got %q", "No", called)
		}
		if m.confirmModalDone != nil || m.confirmModalPreviousFocus != nil {
			t.Fatal("expected confirm modal state to be reset")
		}
	})

	t.Run("logout key returns batch command", func(t *testing.T) {
		if cmd := m.HandleEvent(tcell.NewEventKey(tcell.KeyCtrlD, "", tcell.ModNone)); cmd == nil {
			t.Fatal("expected logout key to return a command")
		}
	})

	t.Run("message search key opens popup", func(t *testing.T) {
		m.RemoveLayer(messageSearchLayerName)
		m.HandleEvent(tcell.NewEventKey(tcell.KeyCtrlF, "", tcell.ModNone))
		if !m.HasLayer(messageSearchLayerName) {
			t.Fatal("expected message search key to open popup")
		}
	})
}

func TestModelFocusFallbackBranches(t *testing.T) {
	m := newTestModel()

	t.Run("focusPrevious with hidden tree and disabled input", func(t *testing.T) {
		m.toggleGuildsTree()
		m.messageInput.SetDisabled(true)
		m.app.SetFocus(m.messagesList)
		m.focusPrevious()
		if m.app.GetFocus() != m.messagesList {
			t.Fatalf("expected focus to remain on messages list, got %T", m.app.GetFocus())
		}
	})

	t.Run("focusNext with hidden tree falls back to messages list", func(t *testing.T) {
		m.messageInput.SetDisabled(false)
		m.app.SetFocus(m.messageInput)
		m.focusNext()
		if m.app.GetFocus() != m.messagesList {
			t.Fatalf("expected focus to fall back to messages list, got %T", m.app.GetFocus())
		}
	})
}

func TestModelAdditionalFocusAndFooterBranches(t *testing.T) {
	t.Run("toggle guilds tree moves focus to main flex when hidden", func(t *testing.T) {
		m := newTestModel()
		m.app.SetFocus(m.guildsTree)
		m.toggleGuildsTree()
		if m.mainFlex.GetItemCount() != 1 {
			t.Fatalf("expected guild tree to be removed, got %d items", m.mainFlex.GetItemCount())
		}
		if m.app.GetFocus() != m.mainFlex {
			t.Fatalf("expected focus to move to main flex, got %T", m.app.GetFocus())
		}
	})

	t.Run("focusPrevious moves from messages list to visible tree", func(t *testing.T) {
		m := newTestModel()
		m.app.SetFocus(m.messagesList)
		m.focusPrevious()
		if m.app.GetFocus() != m.guildsTree {
			t.Fatalf("expected focus on guilds tree, got %T", m.app.GetFocus())
		}
	})

	t.Run("focusNext falls through to guild tree when input disabled", func(t *testing.T) {
		m := newTestModel()
		m.messageInput.SetDisabled(true)
		m.app.SetFocus(m.messagesList)
		m.focusNext()
		if m.app.GetFocus() != m.guildsTree {
			t.Fatalf("expected focus on guilds tree, got %T", m.app.GetFocus())
		}
	})

	t.Run("addTyper refreshes existing timer and updateFooter ignores unknown dm users", func(t *testing.T) {
		m := newTestModel()
		channel := &discord.Channel{ID: 900, Type: discord.DirectMessage, DMRecipients: []discord.User{{ID: 2, Username: "known"}}}
		m.SetSelectedChannel(channel)
		m.addTyper(2)
		firstTimer := m.typers[2]
		m.addTyper(2)
		if m.typers[2] != firstTimer {
			t.Fatal("expected repeated typer event to reuse existing timer")
		}

		m.addTyper(999)
		time.Sleep(20 * time.Millisecond)
		if footer := m.messagesList.GetFooter(); footer == "" || !strings.Contains(footer, "known") || (!strings.Contains(footer, "is typing") && !strings.Contains(footer, "are typing")) {
			t.Fatalf("expected footer to mention known typer, got %q", footer)
		}
		m.clearTypers()
	})

	t.Run("updateFooter with no selected channel is a no-op", func(t *testing.T) {
		m := newTestModel()
		m.SetSelectedChannel(nil)
		m.updateFooter()
	})
}

func TestModelHandleEventUnmatchedKeyFallsThrough(t *testing.T) {
	m := newTestModel()
	if cmd := m.HandleEvent(tcell.NewEventKey(tcell.KeyRune, "z", tcell.ModNone)); cmd != nil {
		t.Fatalf("expected unmatched model key to fall through, got %T", cmd)
	}
}

func TestModelAdditionalBranchCoverage(t *testing.T) {
	t.Run("focusPrevious from guild tree prefers enabled input", func(t *testing.T) {
		m := newTestModel()
		m.messageInput.SetDisabled(false)
		m.app.SetFocus(m.guildsTree)
		m.focusPrevious()
		if m.app.GetFocus() != m.messageInput {
			t.Fatalf("expected focus to move to message input, got %T", m.app.GetFocus())
		}
	})

	t.Run("init event error becomes EventError", func(t *testing.T) {
		oldNewOpenState := newOpenState
		oldOpenNingenState := openNingenState
		t.Cleanup(func() {
			newOpenState = oldNewOpenState
			openNingenState = oldOpenNingenState
		})

		sentinel := errors.New("open failed")
		built := ningen.FromState(state.NewFromSession(session.New(""), defaultstore.New()))
		newOpenState = func(string, gateway.Identifier) *ningen.State { return built }
		openNingenState = func(*ningen.State) error { return sentinel }

		m := newTestModel()
		event := executeCommand(requireCommand(t, m.HandleEvent(&tview.InitEvent{})))
		errEvent, ok := event.(*tcell.EventError)
		if !ok {
			t.Fatalf("expected EventError, got %T", event)
		}
		if errEvent.Error() != sentinel.Error() {
			t.Fatalf("expected wrapped error %q, got %q", sentinel.Error(), errEvent.Error())
		}
	})

	t.Run("init event success returns nil event", func(t *testing.T) {
		oldNewOpenState := newOpenState
		oldOpenNingenState := openNingenState
		t.Cleanup(func() {
			newOpenState = oldNewOpenState
			openNingenState = oldOpenNingenState
		})

		built := ningen.FromState(state.NewFromSession(session.New(""), defaultstore.New()))
		newOpenState = func(string, gateway.Identifier) *ningen.State { return built }
		openNingenState = func(*ningen.State) error { return nil }

		m := newTestModel()
		if event := executeCommand(requireCommand(t, m.HandleEvent(&tview.InitEvent{}))); event != nil {
			t.Fatalf("expected successful init command to emit nil event, got %T", event)
		}
	})

	t.Run("updateFooter prefers guild nicknames", func(t *testing.T) {
		m := newTestModel()
		channel := &discord.Channel{ID: 42, GuildID: 77, Type: discord.GuildText}
		m.SetSelectedChannel(channel)
		m.state.Cabinet.MemberStore.MemberSet(channel.GuildID, &discord.Member{
			User: discord.User{ID: 9, Username: "username"},
			Nick: "nickname",
		}, false)

		m.addTyper(9)
		time.Sleep(20 * time.Millisecond)
		if footer := m.messagesList.GetFooter(); !strings.Contains(footer, "nickname") {
			t.Fatalf("expected guild nickname in footer, got %q", footer)
		}
		m.clearTypers()
	})

	t.Run("addTyper callback removes typer", func(t *testing.T) {
		oldTypingAfterFunc := typingAfterFunc
		t.Cleanup(func() { typingAfterFunc = oldTypingAfterFunc })

		var callback func()
		typingAfterFunc = func(_ time.Duration, fn func()) *time.Timer {
			callback = fn
			return time.NewTimer(time.Hour)
		}

		m := newTestModel()
		channel := &discord.Channel{ID: 100, Type: discord.DirectMessage, DMRecipients: []discord.User{{ID: 3, Username: "user3"}}}
		m.SetSelectedChannel(channel)
		m.addTyper(3)
		if callback == nil {
			t.Fatal("expected addTyper to install an expiry callback")
		}
		callback()
		time.Sleep(20 * time.Millisecond)
		if _, ok := m.typers[3]; ok {
			t.Fatal("expected expiry callback to remove typer")
		}
	})
}

func TestModelHandleEventInitAndKeyRouting(t *testing.T) {
	t.Run("init event returns event command", func(t *testing.T) {
		m := newTestModel()
		if cmd := m.HandleEvent(&tview.InitEvent{}); cmd == nil {
			t.Fatal("expected init event to return a command")
		}
	})

	t.Run("focus and picker keys redraw through handle event", func(t *testing.T) {
		m := newTestModel()
		m.messageInput.SetDisabled(false)
		keys := []*tcell.EventKey{
			tcell.NewEventKey(tcell.KeyCtrlG, "", tcell.ModNone),
			tcell.NewEventKey(tcell.KeyCtrlT, "", tcell.ModNone),
			tcell.NewEventKey(tcell.KeyCtrlI, "", tcell.ModNone),
			tcell.NewEventKey(tcell.KeyCtrlH, "", tcell.ModNone),
			tcell.NewEventKey(tcell.KeyCtrlL, "", tcell.ModNone),
			tcell.NewEventKey(tcell.KeyCtrlB, "", tcell.ModNone),
			tcell.NewEventKey(tcell.KeyCtrlK, "", tcell.ModNone),
		}

		for _, key := range keys {
			m.HandleEvent(key)
		}
	})

	t.Run("unknown key falls through to layer handling", func(t *testing.T) {
		m := newTestModel()
		if cmd := m.HandleEvent(tcell.NewEventKey(tcell.KeyRune, "x", tcell.ModNone)); cmd != nil {
			t.Fatalf("expected unmatched model key to return nil, got %T", cmd)
		}
	})
}

func TestModelTyperResetBranch(t *testing.T) {
	m := newTestModel()
	channel := &discord.Channel{ID: 123, Type: discord.DirectMessage, DMRecipients: []discord.User{{ID: 2, Username: "user2"}}}
	m.SetSelectedChannel(channel)

	m.addTyper(2)
	firstTimer := m.typers[2]
	m.addTyper(2)

	if len(m.typers) != 1 {
		t.Fatalf("expected duplicate typer to reuse existing entry, got %d", len(m.typers))
	}
	if m.typers[2] != firstTimer {
		t.Fatal("expected duplicate typer to reset the existing timer")
	}

	m.removeTyper(2)
}

func TestModelNewViewAfterDrawCallback(t *testing.T) {
	cfg, _ := config.Load("")
	app := tview.NewApplication()
	m := NewView(app, cfg, "token")

	afterDraw := getAfterDrawFunc(app)
	if afterDraw == nil {
		t.Fatal("expected NewView to install an after-draw callback")
	}

	m.messagesList.cfg.InlineImages.Enabled = true
	m.messagesList.useKitty = true
	m.messagesList.pendingFullClear = true
	m.messagesList.pendingDeletes = []uint32{7}
	screen := &screenWithTty{tty: &mockTty{}}

	afterDraw(screen)
	if m.messagesList.pendingFullClear || len(m.messagesList.pendingDeletes) != 0 {
		t.Fatal("expected after-draw callback to flush pending messages-list kitty work")
	}

	m.AddLayer(tview.NewBox(), layers.WithName(reactionPickerLayerName), layers.WithVisible(true), layers.WithOverlay())
	afterDraw(screen)
}

func TestModelHandleEventFocusAndOverlayKeys(t *testing.T) {
	m := newTestModel()
	channel := &discord.Channel{ID: 123, Type: discord.DirectMessage}
	m.SetSelectedChannel(channel)
	m.messageInput.SetDisabled(false)

	t.Run("focus guilds tree key", func(t *testing.T) {
		m.openPicker()
		m.closePicker()
		m.messageInput.showMentionList()
		m.HandleEvent(tcell.NewEventKey(tcell.KeyCtrlG, "", tcell.ModNone))
		if m.app.GetFocus() != m.guildsTree {
			t.Fatalf("expected guild tree focus, got %T", m.app.GetFocus())
		}
	})

	t.Run("focus messages list key", func(t *testing.T) {
		m.HandleEvent(tcell.NewEventKey(tcell.KeyCtrlT, "", tcell.ModNone))
		if m.app.GetFocus() != m.messagesList {
			t.Fatalf("expected messages list focus, got %T", m.app.GetFocus())
		}
	})

	t.Run("focus message input key", func(t *testing.T) {
		m.app.SetFocus(m.messagesList)
		m.messageInput.SetDisabled(false)
		if !m.focusMessageInput() {
			t.Fatal("expected direct focusMessageInput to succeed")
		}
		if m.app.GetFocus() != m.messageInput {
			t.Fatalf("expected message input focus, got %T", m.app.GetFocus())
		}
	})

	t.Run("focus previous and next keys", func(t *testing.T) {
		m.app.SetFocus(m.messagesList)
		m.HandleEvent(tcell.NewEventKey(tcell.KeyCtrlH, "", tcell.ModNone))
		m.HandleEvent(tcell.NewEventKey(tcell.KeyCtrlL, "", tcell.ModNone))
	})

	t.Run("toggle guilds tree key", func(t *testing.T) {
		m.HandleEvent(tcell.NewEventKey(tcell.KeyCtrlB, "", tcell.ModNone))
		if m.mainFlex.GetItemCount() != 1 {
			t.Fatalf("expected guild tree hidden, item count=%d", m.mainFlex.GetItemCount())
		}
		m.HandleEvent(tcell.NewEventKey(tcell.KeyCtrlB, "", tcell.ModNone))
		if m.mainFlex.GetItemCount() != 2 {
			t.Fatalf("expected guild tree shown, item count=%d", m.mainFlex.GetItemCount())
		}
	})

	t.Run("toggle channels picker key", func(t *testing.T) {
		m.HandleEvent(tcell.NewEventKey(tcell.KeyCtrlK, "", tcell.ModNone))
		if !m.HasLayer(channelsPickerLayerName) {
			t.Fatal("expected channels picker layer to open")
		}
		m.HandleEvent(tcell.NewEventKey(tcell.KeyCtrlK, "", tcell.ModNone))
		if m.HasLayer(channelsPickerLayerName) {
			t.Fatal("expected channels picker layer to close")
		}
	})
}
