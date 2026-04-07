package chat

import (
	"reflect"
	"testing"
	"unsafe"

	"github.com/eyalmazuz/tview"
	"github.com/gdamore/tcell/v3"
)

func requireCommand(t *testing.T, cmd tview.Cmd) tview.Cmd {
	t.Helper()
	if cmd == nil {
		t.Fatal("expected command")
	}
	return cmd
}

func executeCommand(cmd tview.Cmd) tcell.Event {
	if cmd == nil {
		return nil
	}
	return cmd()
}

func executeModelCommand(m *Model, cmd tview.Cmd) {
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
		case "batchMsg":
			commands := elem.FieldByName("cmds")
			for i := 0; i < commands.Len(); i++ {
				cmdValue := commands.Index(i)
				cmd := reflect.NewAt(cmdValue.Type(), unsafe.Pointer(cmdValue.UnsafeAddr())).Elem().Interface().(tview.Cmd)
				executeModelCommand(m, cmd)
			}
			return
		case "setFocusMsg":
			targetField := elem.FieldByName("target")
			target, _ := reflect.NewAt(targetField.Type(), unsafe.Pointer(targetField.UnsafeAddr())).Elem().Interface().(tview.Model)
			if target != nil {
				setFocusForTest(m.app, target)
			}
			return
		}
	}

	executeModelCommand(m, m.Update(event))
}
