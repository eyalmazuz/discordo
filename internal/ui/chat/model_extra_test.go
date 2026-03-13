package chat

import (
	"testing"
	"time"

	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/diamondburned/arikawa/v3/gateway"
	"github.com/diamondburned/ningen/v3/states/read"
	"github.com/ayn2op/tview"
	"github.com/gdamore/tcell/v3"
	"github.com/ayn2op/tview/layers"
)

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

func TestModel_ConsumeLayerCommands(t *testing.T) {
	m := newTestModel()

	t.Run("OpenLayer", func(t *testing.T) {
		m.AddLayer(tview.NewBox(), layers.WithName("test"), layers.WithVisible(false))
		m.consumeLayerCommands(layers.OpenLayerCommand{Name: "test"})
		if !m.GetVisible("test") {
			t.Errorf("Layer should be visible")
		}
	})

	t.Run("CloseLayer", func(t *testing.T) {
		m.AddLayer(tview.NewBox(), layers.WithName("test2"), layers.WithVisible(true))
		m.consumeLayerCommands(layers.CloseLayerCommand{Name: "test2"})
		if m.GetVisible("test2") {
			t.Errorf("Layer should be hidden")
		}
	})

	t.Run("ToggleLayer", func(t *testing.T) {
		m.AddLayer(tview.NewBox(), layers.WithName("test3"), layers.WithVisible(false))
		m.consumeLayerCommands(layers.ToggleLayerCommand{Name: "test3"})
		if !m.GetVisible("test3") {
			t.Errorf("Layer should be visible after toggle")
		}
		m.consumeLayerCommands(layers.ToggleLayerCommand{Name: "test3"})
		if m.GetVisible("test3") {
			t.Errorf("Layer should be hidden after second toggle")
		}
	})
	
	t.Run("BatchCommand", func(t *testing.T) {
		m.consumeLayerCommands(tview.BatchCommand{layers.OpenLayerCommand{Name: "test"}, layers.CloseLayerCommand{Name: "test2"}})
	})
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
