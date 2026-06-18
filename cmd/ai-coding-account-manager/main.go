package main

import (
	"log/slog"
	"os"

	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/app"
)

func main() {
	if err := app.Run(os.Args[1:]); err != nil {
		slog.Error("application stopped", "error", err)
		os.Exit(1)
	}
}
