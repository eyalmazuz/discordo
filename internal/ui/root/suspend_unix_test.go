//go:build unix

package root

import (
	"os"
	"syscall"
	"testing"

	"github.com/ayn2op/tview"
	"github.com/gdamore/tcell/v3"
)

type rootMockScreen struct {
	tcell.Screen
}

func (m *rootMockScreen) SetContent(int, int, rune, []rune, tcell.Style) {}
func (m *rootMockScreen) Size() (int, int)                                { return 80, 24 }
func (m *rootMockScreen) Put(x, y int, s string, style tcell.Style) (string, int) {
	return s, len(s)
}
func (m *rootMockScreen) PutStrStyled(int, int, string, tcell.Style) {}

func TestRootModelDraw(t *testing.T) {
	m := newTestRootModel(t)
	m.Draw(&rootMockScreen{})
}

func TestModelSuspend(t *testing.T) {
	m := newTestRootModel(t)

	oldSuspendApp := suspendApp
	oldNotifySignal := notifySignal
	oldStopSignal := stopSignal
	oldKillProcess := killProcess
	t.Cleanup(func() {
		suspendApp = oldSuspendApp
		notifySignal = oldNotifySignal
		stopSignal = oldStopSignal
		killProcess = oldKillProcess
	})

	var suspended, stopped bool
	suspendApp = func(_ *tview.Application, fn func()) {
		suspended = true
		fn()
	}
	notifySignal = func(c chan<- os.Signal, _ ...os.Signal) {
		c <- syscall.SIGCONT
	}
	stopSignal = func(chan<- os.Signal) {
		stopped = true
	}
	killProcess = func(pid int, sig syscall.Signal) error {
		if pid != 0 || sig != syscall.SIGTSTP {
			t.Fatalf("unexpected kill args: pid=%d sig=%v", pid, sig)
		}
		return nil
	}

	m.suspend()
	if !suspended {
		t.Fatal("expected suspend to delegate to the app suspend seam")
	}
	if !stopped {
		t.Fatal("expected suspend to stop signal notifications")
	}
}
