package main

import (
	"errors"
	"testing"
)

func TestMain(t *testing.T) {
	oldRunCmd := runCmd
	t.Cleanup(func() {
		runCmd = oldRunCmd
	})

	calls := 0
	runCmd = func() error {
		calls++
		return nil
	}
	main()

	runCmd = func() error {
		calls++
		return errors.New("run fail")
	}
	main()

	if calls != 2 {
		t.Fatalf("expected main to call runCmd twice, got %d", calls)
	}
}
