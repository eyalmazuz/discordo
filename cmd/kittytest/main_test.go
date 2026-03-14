package main

import (
	"bytes"
	"errors"
	"image"
	"io"
	"strings"
	"testing"

	"golang.org/x/sys/unix"
)

func TestRun(t *testing.T) {
	baseDeps := deps{
		isKittySupported: func() bool { return true },
		ioctlGetWinsize: func(int, uint) (*unix.Winsize, error) {
			return &unix.Winsize{Col: 80, Row: 24, Xpixel: 800, Ypixel: 480}, nil
		},
		encodeKitty: func(io.Writer, image.Image, int, int, int, int) error { return nil },
		deleteAllKitty: func(io.Writer) error { return nil },
	}

	t.Run("unsupported terminal", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		d := baseDeps
		d.isKittySupported = func() bool { return false }
		if code := run(&stdout, &stderr, strings.NewReader("\n"), d); code != 1 {
			t.Fatalf("exit code = %d, want 1", code)
		}
		if !strings.Contains(stderr.String(), "Terminal does not appear to support Kitty graphics protocol.") {
			t.Fatalf("stderr = %q", stderr.String())
		}
	})

	t.Run("winsize ioctl failure", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		d := baseDeps
		d.ioctlGetWinsize = func(int, uint) (*unix.Winsize, error) { return nil, errors.New("ioctl failed") }
		if code := run(&stdout, &stderr, strings.NewReader("\n"), d); code != 1 {
			t.Fatalf("exit code = %d, want 1", code)
		}
		if !strings.Contains(stderr.String(), "ioctl TIOCGWINSZ: ioctl failed") {
			t.Fatalf("stderr = %q", stderr.String())
		}
	})

	t.Run("zero terminal size", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		d := baseDeps
		d.ioctlGetWinsize = func(int, uint) (*unix.Winsize, error) {
			return &unix.Winsize{Col: 0, Row: 24, Xpixel: 800, Ypixel: 480}, nil
		}
		if code := run(&stdout, &stderr, strings.NewReader("\n"), d); code != 1 {
			t.Fatalf("exit code = %d, want 1", code)
		}
		if !strings.Contains(stderr.String(), "Could not determine terminal size.") {
			t.Fatalf("stderr = %q", stderr.String())
		}
	})

	t.Run("zero cell dimensions", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		d := baseDeps
		d.ioctlGetWinsize = func(int, uint) (*unix.Winsize, error) {
			return &unix.Winsize{Col: 80, Row: 24, Xpixel: 0, Ypixel: 0}, nil
		}
		if code := run(&stdout, &stderr, strings.NewReader("\n"), d); code != 1 {
			t.Fatalf("exit code = %d, want 1", code)
		}
		if !strings.Contains(stderr.String(), "Cell pixel dimensions are zero") {
			t.Fatalf("stderr = %q", stderr.String())
		}
	})

	t.Run("encode failure", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		d := baseDeps
		d.encodeKitty = func(io.Writer, image.Image, int, int, int, int) error { return errors.New("encode failed") }
		if code := run(&stdout, &stderr, strings.NewReader("\n"), d); code != 1 {
			t.Fatalf("exit code = %d, want 1", code)
		}
		if !strings.Contains(stderr.String(), "EncodeKitty error: encode failed") {
			t.Fatalf("stderr = %q", stderr.String())
		}
	})

	t.Run("success path", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		encodeCalled := false
		deleteCalled := false
		d := baseDeps
		d.encodeKitty = func(w io.Writer, img image.Image, cols, rows, cellW, cellH int) error {
			encodeCalled = true
			if img.Bounds().Dx() == 0 || img.Bounds().Dy() == 0 {
				t.Fatal("expected non-empty generated image")
			}
			return nil
		}
		d.deleteAllKitty = func(io.Writer) error {
			deleteCalled = true
			return nil
		}

		if code := run(&stdout, &stderr, strings.NewReader("\n"), d); code != 0 {
			t.Fatalf("exit code = %d, want 0", code)
		}
		if stderr.Len() != 0 {
			t.Fatalf("stderr = %q, want empty", stderr.String())
		}
		if !encodeCalled || !deleteCalled {
			t.Fatalf("expected encode and cleanup to be called, got encode=%v delete=%v", encodeCalled, deleteCalled)
		}
		if !strings.Contains(stdout.String(), "If you see a gradient above, Kitty graphics protocol is working.") {
			t.Fatalf("stdout = %q", stdout.String())
		}
	})
}
