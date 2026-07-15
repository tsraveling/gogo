package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Appends a line to ~/.config/gogo/debug.log when GOGO_DEBUG is set. No-op
// otherwise. The TUI owns the screen, so diagnostics can't go to stdout.
func debugf(format string, args ...any) {
	if os.Getenv("GOGO_DEBUG") == "" {
		return
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}
	path := filepath.Join(home, ".config", "gogo", "debug.log")
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "%s "+format+"\n", append([]any{time.Now().Format("15:04:05.000")}, args...)...)
}
