package main

import (
	"os"

	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/app"
)

func main() {
	if err := app.Run(os.Args[1:]); err != nil {
		os.Exit(1)
	}
}
