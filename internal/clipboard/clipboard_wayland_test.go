//go:build linux || freebsd

package clipboard

import (
	"errors"
	"testing"
)

func TestInit_Wayland(t *testing.T) {
	oldWayland := wayland
	oldLookupEnv := osLookupEnv
	oldLookPath := execLookPath
	oldClipboardInit := clipboardInit
	defer func() {
		wayland = oldWayland
		osLookupEnv = oldLookupEnv
		execLookPath = oldLookPath
		clipboardInit = oldClipboardInit
	}()

	t.Run("NoWayland", func(t *testing.T) {
		osLookupEnv = func(key string) (string, bool) { return "", false }
		clipboardInit = func() error { return nil }
		wayland = false
		err := Init()
		if err != nil {
			t.Errorf("Init failed: %v", err)
		}
		if wayland {
			t.Error("expected wayland to be false")
		}
	})

	t.Run("WaylandFound", func(t *testing.T) {
		osLookupEnv = func(key string) (string, bool) { return "wayland-0", true }
		execLookPath = func(file string) (string, error) { return "/usr/bin/" + file, nil }
		wayland = false
		err := Init()
		if err != nil {
			t.Errorf("Init failed: %v", err)
		}
		if !wayland {
			t.Error("expected wayland to be true")
		}
	})

	t.Run("Wayland_WlCopyMissing", func(t *testing.T) {
		osLookupEnv = func(key string) (string, bool) { return "wayland-0", true }
		execLookPath = func(file string) (string, error) {
			if file == "wl-copy" {
				return "", errors.New("missing")
			}
			return "/usr/bin/" + file, nil
		}
		wayland = false
		err := Init()
		if err == nil {
			t.Error("expected error when wl-copy is missing")
		}
	})
}

func TestReadWrite_Wayland(t *testing.T) {
	oldWayland := wayland
	oldExecCommand := execCommand
	oldClipboardRead := clipboardRead
	oldClipboardWrite := clipboardWrite
	defer func() {
		wayland = oldWayland
		execCommand = oldExecCommand
		clipboardRead = oldClipboardRead
		clipboardWrite = oldClipboardWrite
	}()

	t.Run("Read_NoWayland", func(t *testing.T) {
		wayland = false
		clipboardRead = func(t Format) ([]byte, error) { return []byte("data"), nil }
		data, _ := Read(FmtText)
		if string(data) != "data" {
			t.Errorf("got %q", string(data))
		}
	})

	t.Run("Write_NoWayland", func(t *testing.T) {
		wayland = false
		clipboardWrite = func(t Format, b []byte) error { return nil }
		err := Write(FmtText, []byte("data"))
		if err != nil {
			t.Errorf("Write failed: %v", err)
		}
	})

	t.Run("FormatToType", func(t *testing.T) {
		if got := formatToType(FmtImage); got != "image" {
			t.Errorf("got %q", got)
		}
		if got := formatToType(FmtText); got != "text/plain;charset=utf-8" {
			t.Errorf("got %q", got)
		}
	})
}

