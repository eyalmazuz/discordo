package chat

import (
	"fmt"

	"github.com/eyalmazuz/tview"
	"github.com/diamondburned/ningen/v3"
	"github.com/gdamore/tcell/v3"
)

// SetState sets the state of the model for testing purposes.
func (m *Model) SetState(s *ningen.State) {
	m.state = s
}

// GetState returns the state of the model for testing purposes.
func (m *Model) GetState() *ningen.State {
	return m.state
}

// GetGuildsTree returns the guilds tree for testing purposes.
func (m *Model) GetGuildsTree() tview.Primitive {
	return m.guildsTree
}

// GetMessagesList returns the messages list for testing purposes.
func (m *Model) GetMessagesList() tview.Primitive {
	return m.messagesList
}

// GetMessageInput returns the message input for testing purposes.
func (m *Model) GetMessageInput() tview.Primitive {
	return m.messageInput
}

type MockScreen struct {
	tcell.Screen
	Content map[string]rune
}

func (m *MockScreen) SetContent(x int, y int, primary rune, combining []rune, style tcell.Style) {
	if m.Content == nil {
		m.Content = make(map[string]rune)
	}
	m.Content[fmt.Sprintf("%d,%d", x, y)] = primary
}

func (m *MockScreen) LockRegion(x, y, width, height int, lock bool) {}

func (m *MockScreen) Tty() (tcell.Tty, bool) {
	return nil, false
}

func (m *MockScreen) Get(x, y int) (string, tcell.Style, int) {
	return " ", tcell.StyleDefault, 1
}
