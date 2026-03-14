package main

import (
	"log/slog"

	"github.com/ayn2op/discordo/cmd"
)

var runCmd = cmd.Run

func main() {
	if err := runCmd(); err != nil {
		slog.Error("failed to run command", "err", err)
	}
}
