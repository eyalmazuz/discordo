package login

import (
	"testing"

	"github.com/eyalmazuz/tview"
	"github.com/gdamore/tcell/v3"
)

func runCommand(t *testing.T, cmd tview.Command) tcell.Event {
	t.Helper()
	if cmd == nil {
		return nil
	}
	return cmd()
}
