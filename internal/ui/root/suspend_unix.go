//go:build unix

package root

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/ayn2op/tview"
)

var (
	suspendApp   = func(app *tview.Application, fn func()) { app.Suspend(fn) }
	notifySignal = signal.Notify
	stopSignal   = signal.Stop
	killProcess  = syscall.Kill
)

func (m *Model) suspend() {
	suspendApp(m.app, func() {
		c := make(chan os.Signal, 1)
		notifySignal(c, syscall.SIGCONT)
		defer stopSignal(c)

		_ = killProcess(0, syscall.SIGTSTP)
		<-c
	})
}
