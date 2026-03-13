package chat

import (
	"testing"

	"github.com/ayn2op/discordo/internal/config"
	"github.com/ayn2op/tview"
	"github.com/diamondburned/arikawa/v3/session"
	"github.com/diamondburned/arikawa/v3/state"
	"github.com/diamondburned/arikawa/v3/state/store/defaultstore"
	"github.com/diamondburned/ningen/v3"
	"github.com/gdamore/tcell/v3"
)

func TestComplexScenario_SelectGuildAndChannel(t *testing.T) {
	app := tview.NewApplication()
	cfg, _ := config.Load("")

	// Create the view
	view := NewView(app, cfg, "test-token")

	// Initialize mock state
	s := state.NewFromSession(session.New(""), defaultstore.New())
	view.SetState(ningen.FromState(s))
	
	// Simulate starting the app and getting focus on guilds tree
	app.SetRoot(view)
	
	// Initially, focus should be on the guilds tree
	// Simulate pressing 'j' (Down) in guilds tree
	view.HandleEvent(tcell.NewEventKey(tcell.KeyRune, "j", tcell.ModNone))
	
	// Simulate pressing 'Enter' to select a guild/channel
	view.HandleEvent(tcell.NewEventKey(tcell.KeyEnter, "", tcell.ModNone))
	
	// Simulate switching focus to message input
	view.HandleEvent(tcell.NewEventKey(tcell.KeyCtrlI, "", tcell.ModNone))
	
	// Verify if we can type in message input
	view.HandleEvent(tcell.NewEventKey(tcell.KeyRune, "h", tcell.ModNone))
	view.HandleEvent(tcell.NewEventKey(tcell.KeyRune, "i", tcell.ModNone))
	
	// Simulate opening the channels picker
	view.HandleEvent(tcell.NewEventKey(tcell.KeyCtrlK, "", tcell.ModNone))
}
