//go:build !darwin

package notifications

import (
	"errors"
	"testing"
)

func TestSendDesktopNotificationImpl(t *testing.T) {
	oldBeeepNotify := beeepNotify
	oldBeeepBeep := beeepBeep
	t.Cleanup(func() {
		beeepNotify = oldBeeepNotify
		beeepBeep = oldBeeepBeep
	})

	t.Run("notify error", func(t *testing.T) {
		beeepNotify = func(string, string, any) error { return errors.New("notify") }
		beeepBeep = func(float64, int) error {
			t.Fatal("beep should not run after notify error")
			return nil
		}
		if err := sendDesktopNotificationImpl("t", "m", "", true, 1); err == nil {
			t.Fatal("expected notify error")
		}
	})

	t.Run("silent notification", func(t *testing.T) {
		beeepNotify = func(title, message string, icon any) error { return nil }
		beeepBeep = func(float64, int) error {
			t.Fatal("beep should not run for silent notification")
			return nil
		}
		if err := sendDesktopNotificationImpl("t", "m", "", false, 1); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("beep error", func(t *testing.T) {
		beeepNotify = func(string, string, any) error { return nil }
		beeepBeep = func(float64, int) error { return errors.New("beep") }
		if err := sendDesktopNotificationImpl("t", "m", "", true, 1); err == nil {
			t.Fatal("expected beep error")
		}
	})
}
