//go:build !darwin

package notifications

import (
	"errors"
	"testing"

	"github.com/gen2brain/beeep"
)

func TestSendDesktopNotification(t *testing.T) {
	oldBeeepNotify := beeepNotify
	oldBeeepBeep := beeepBeep
	t.Cleanup(func() {
		beeepNotify = oldBeeepNotify
		beeepBeep = oldBeeepBeep
	})

	t.Run("notify error", func(t *testing.T) {
		beeepNotify = func(string, string, any) error { return errors.New("notify") }
		if err := sendDesktopNotification("title", "message", "image", false, 0); err == nil {
			t.Fatal("expected notify error")
		}
	})

	t.Run("silent notification", func(t *testing.T) {
		var beepCalled bool
		beeepNotify = func(title, message string, icon any) error {
			if title != "title" || message != "message" || icon != "image" {
				t.Fatalf("unexpected notify args: %q %q %v", title, message, icon)
			}
			return nil
		}
		beeepBeep = func(freq float64, duration int) error {
			beepCalled = true
			return nil
		}
		if err := sendDesktopNotification("title", "message", "image", false, 12); err != nil {
			t.Fatalf("sendDesktopNotification returned error: %v", err)
		}
		if beepCalled {
			t.Fatal("expected silent notification to skip beep")
		}
	})

	t.Run("beep success and error", func(t *testing.T) {
		beeepNotify = func(string, string, any) error { return nil }
		var gotFreq float64
		var gotDuration int
		beeepBeep = func(freq float64, duration int) error {
			gotFreq, gotDuration = freq, duration
			return nil
		}
		if err := sendDesktopNotification("title", "message", "image", true, 34); err != nil {
			t.Fatalf("sendDesktopNotification returned error: %v", err)
		}
		if gotFreq != beeep.DefaultFreq || gotDuration != 34 {
			t.Fatalf("unexpected beep args: freq=%v duration=%d", gotFreq, gotDuration)
		}

		beeepBeep = func(float64, int) error { return errors.New("beep") }
		if err := sendDesktopNotification("title", "message", "image", true, 34); err == nil {
			t.Fatal("expected beep error")
		}
	})
}
