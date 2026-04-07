package qr

import (
	"testing"

	"github.com/eyalmazuz/tview"
	"github.com/gdamore/tcell/v3"
)

func requireCommand(t *testing.T, cmd tview.Cmd) tview.Cmd {
	t.Helper()
	if cmd == nil {
		t.Fatal("expected command, got nil")
	}
	return cmd
}

func runCommand(t *testing.T, cmd tview.Cmd) tcell.Event {
	t.Helper()
	return requireCommand(t, cmd)()
}
