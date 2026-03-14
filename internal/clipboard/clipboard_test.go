package clipboard

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestFormatToType(t *testing.T) {
	if got := formatToType(FmtText); got != "text/plain;charset=utf-8" {
		t.Fatalf("formatToType(FmtText) = %q", got)
	}
	if got := formatToType(FmtImage); got != "image" {
		t.Fatalf("formatToType(FmtImage) = %q", got)
	}
	if got := formatToType(Format(255)); got != "text/plain;charset=utf-8" {
		t.Fatalf("formatToType(default) = %q", got)
	}
}

func TestWaylandInitRequiresClipboardTools(t *testing.T) {
	oldWayland := wayland
	oldPath := os.Getenv("PATH")
	oldDisplay, hadDisplay := os.LookupEnv("WAYLAND_DISPLAY")
	t.Cleanup(func() {
		wayland = oldWayland
		_ = os.Setenv("PATH", oldPath)
		if hadDisplay {
			_ = os.Setenv("WAYLAND_DISPLAY", oldDisplay)
		} else {
			_ = os.Unsetenv("WAYLAND_DISPLAY")
		}
	})

	wayland = false
	tmpDir := t.TempDir()
	_ = os.Setenv("WAYLAND_DISPLAY", "wayland-0")
	_ = os.Setenv("PATH", tmpDir)

	if err := Init(); err == nil {
		t.Fatal("expected Init to fail when wl-copy and wl-paste are unavailable")
	}
	if wayland {
		t.Fatal("expected failed Init to leave wayland disabled")
	}
}

func TestWaylandReadWriteRoundTrip(t *testing.T) {
	oldWayland := wayland
	oldPath := os.Getenv("PATH")
	oldDisplay, hadDisplay := os.LookupEnv("WAYLAND_DISPLAY")
	t.Cleanup(func() {
		wayland = oldWayland
		_ = os.Setenv("PATH", oldPath)
		if hadDisplay {
			_ = os.Setenv("WAYLAND_DISPLAY", oldDisplay)
		} else {
			_ = os.Unsetenv("WAYLAND_DISPLAY")
		}
	})

	tmpDir := t.TempDir()
	clipFile := filepath.Join(tmpDir, "clipboard.txt")
	writeScript := filepath.Join(tmpDir, "wl-copy")
	readScript := filepath.Join(tmpDir, "wl-paste")

	if err := os.WriteFile(writeScript, []byte("#!/bin/sh\ncat > "+clipFile+"\n"), 0o755); err != nil {
		t.Fatalf("write wl-copy: %v", err)
	}
	if err := os.WriteFile(readScript, []byte("#!/bin/sh\ncat "+clipFile+"\n"), 0o755); err != nil {
		t.Fatalf("write wl-paste: %v", err)
	}

	wayland = false
	_ = os.Setenv("WAYLAND_DISPLAY", "wayland-0")
	_ = os.Setenv("PATH", tmpDir+string(os.PathListSeparator)+oldPath)

	if err := Init(); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if !wayland {
		t.Fatal("expected Init to enable wayland mode")
	}

	want := []byte("hello clipboard")
	if err := Write(FmtText, want); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	got, err := Read(FmtText)
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if string(got) != string(want) {
		t.Fatalf("Read() = %q, want %q", string(got), string(want))
	}
}

func TestNonWaylandClipboardFallbacks(t *testing.T) {
	oldWayland := wayland
	oldInit := clipboardInit
	oldRead := clipboardRead
	oldWrite := clipboardWrite
	oldLookupEnv := osLookupEnv
	t.Cleanup(func() {
		wayland = oldWayland
		clipboardInit = oldInit
		clipboardRead = oldRead
		clipboardWrite = oldWrite
		osLookupEnv = oldLookupEnv
	})

	wayland = false
	osLookupEnv = func(string) (string, bool) { return "", false }

	initCalls := 0
	clipboardInit = func() error {
		initCalls++
		return nil
	}
	if err := Init(); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if initCalls != 1 {
		t.Fatalf("expected non-wayland Init to call clipboard.Init once, got %d", initCalls)
	}

	clipboardRead = func(format Format) ([]byte, error) {
		if format != FmtText {
			t.Fatalf("expected text format, got %v", format)
		}
		return []byte("fallback"), nil
	}
	got, err := Read(FmtText)
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if string(got) != "fallback" {
		t.Fatalf("Read() = %q, want %q", string(got), "fallback")
	}

	writeCalls := 0
	clipboardWrite = func(format Format, buf []byte) error {
		writeCalls++
		if format != FmtText {
			t.Fatalf("expected text format, got %v", format)
		}
		if string(buf) != "payload" {
			t.Fatalf("Write() payload = %q, want %q", string(buf), "payload")
		}
		return nil
	}
	if err := Write(FmtText, []byte("payload")); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if writeCalls != 1 {
		t.Fatalf("expected non-wayland Write to call clipboard.Write once, got %d", writeCalls)
	}
}

func TestWaylandInitMissingPasteTool(t *testing.T) {
	oldWayland := wayland
	oldLookupEnv := osLookupEnv
	oldLookPath := execLookPath
	t.Cleanup(func() {
		wayland = oldWayland
		osLookupEnv = oldLookupEnv
		execLookPath = oldLookPath
	})

	wayland = false
	osLookupEnv = func(string) (string, bool) { return "wayland-0", true }
	execLookPath = func(file string) (string, error) {
		if file == "wl-copy" {
			return "/bin/wl-copy", nil
		}
		return "", errors.New("missing")
	}

	if err := Init(); err == nil {
		t.Fatal("expected Init() to fail when wl-paste is unavailable")
	}
	if wayland {
		t.Fatal("expected failed Init to keep wayland disabled")
	}
}

func TestWaylandCommandErrorsAndMimeTypes(t *testing.T) {
	oldWayland := wayland
	oldExecCommand := execCommand
	t.Cleanup(func() {
		wayland = oldWayland
		execCommand = oldExecCommand
	})

	wayland = true
	execCommand = func(name string, args ...string) *exec.Cmd {
		if name == "wl-paste" {
			return exec.Command("sh", "-c", "printf %s \"$1\"", "ignored", args[len(args)-1])
		}
		return exec.Command("false")
	}

	got, err := Read(FmtImage)
	if err != nil {
		t.Fatalf("Read(FmtImage) error = %v", err)
	}
	if string(bytes.TrimSpace(got)) != "image" {
		t.Fatalf("Read(FmtImage) mime type = %q, want %q", string(bytes.TrimSpace(got)), "image")
	}

	if err := Write(FmtText, []byte("payload")); err == nil {
		t.Fatal("expected Write() to fail when wl-copy fails")
	}

	execCommand = func(string, ...string) *exec.Cmd {
		return exec.Command("false")
	}
	if _, err := Read(FmtText); err == nil {
		t.Fatal("expected Read() to fail when wl-paste fails")
	}
}
