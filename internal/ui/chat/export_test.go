package chat

import (
	"github.com/ayn2op/tview"
	"github.com/diamondburned/ningen/v3"
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
