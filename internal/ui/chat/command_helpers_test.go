package chat

import (
	"reflect"
	"testing"

	"github.com/eyalmazuz/tview"
	"github.com/gdamore/tcell/v3"
)

func requireCommand(t *testing.T, cmd tview.Command) tview.Command {
	t.Helper()
	if cmd == nil {
		t.Fatal("expected command")
	}
	return cmd
}

func executeCommand(cmd tview.Command) tcell.Event {
	if cmd == nil {
		return nil
	}
	return cmd()
}

func executeModelCommand(m *Model, cmd tview.Command) {
	if cmd == nil {
		return
	}

	event := cmd()
	if event == nil {
		return
	}

	value := reflect.ValueOf(event)
	if value.IsValid() && value.Kind() == reflect.Pointer && !value.IsNil() {
		elem := value.Elem()
		switch elem.Type().Name() {
		case "batchEvent":
			commands := elem.FieldByName("commands")
			for i := 0; i < commands.Len(); i++ {
				executeModelCommand(m, commands.Index(i).Interface().(tview.Command))
			}
			return
		case "setFocusEvent":
			target, _ := elem.FieldByName("target").Interface().(tview.Primitive)
			if target != nil {
				m.app.SetFocus(target)
			}
			return
		}
	}

	executeModelCommand(m, m.HandleEvent(event))
}
