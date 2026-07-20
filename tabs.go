package main

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// @region tabs:persist

// Lives beside auth.json under ~/.config/gogo/.
const tabsFileName = "tabs.json"

// A locally-cached open tab. Source is "ogs" (later "local"); GameID references
// the game it tracks. The game data itself is fetched, not stored.
type tabRef struct {
	Source string `json:"source"`
	GameID int64  `json:"game_id"`
}

func tabsPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "gogo", tabsFileName), nil
}

// Reads persisted tabs. Missing file yields an empty list.
func loadTabs() ([]tabRef, error) {
	path, err := tabsPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var tabs []tabRef
	if err := json.Unmarshal(data, &tabs); err != nil {
		return nil, err
	}
	return tabs, nil
}

func saveTabs(tabs []tabRef) error {
	path, err := tabsPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(tabs, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}
