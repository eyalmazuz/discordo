package login

import (
	"github.com/eyalmazuz/tview/help"
	"github.com/eyalmazuz/tview/keybind"
)

var _ help.KeyMap = (*Model)(nil)

func (m *Model) ShortHelp() []keybind.Keybind {
	return m.tabs.ShortHelp()
}

func (m *Model) FullHelp() [][]keybind.Keybind {
	return m.tabs.FullHelp()
}
