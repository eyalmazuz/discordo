//go:build !darwin

package notifications

import "github.com/gen2brain/beeep"

var (
	beeepNotify = beeep.Notify
	beeepBeep   = beeep.Beep
)

func sendDesktopNotification(title string, message string, image string, playSound bool, duration int) error {
	if err := beeepNotify(title, message, image); err != nil {
		return err
	}

	if playSound {
		return beeepBeep(beeep.DefaultFreq, duration)
	}

	return nil
}
