package main

import (
	"os"
	"path/filepath"
)

// @region board:theme-persist

// Lives beside auth.json/tabs.json under ~/.config/gogo/. Stores the chosen
// theme's display name, one line.
const themeFileName = "theme"

func themePrefPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "gogo", themeFileName), nil
}

// Reads the persisted theme name; empty string when unset.
func loadThemePref() string {
	path, err := themePrefPath()
	if err != nil {
		return ""
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(data)
}

func saveThemePref(name string) error {
	path, err := themePrefPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(name), 0o600)
}
