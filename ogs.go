package main

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// @region ogs:auth

// authFileName lives under ~/.config/gogo/.
const authFileName = "auth.json"

// ogsModel holds OGS auth state. Data only — no UI.
type ogsModel struct {
	Username     string `json:"username"`
	UserID       int64  `json:"user_id"`
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

// authenticated reports whether we hold tokens.
func (o ogsModel) authenticated() bool {
	return o.AccessToken != "" && o.RefreshToken != ""
}

// authPath resolves ~/.config/gogo/auth.json.
func authPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "gogo", authFileName), nil
}

// loadOGS reads persisted auth state. Missing file yields an empty (unauthed) model.
func loadOGS() (ogsModel, error) {
	var o ogsModel
	path, err := authPath()
	if err != nil {
		return o, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return o, nil
		}
		return o, err
	}
	if err := json.Unmarshal(data, &o); err != nil {
		return o, err
	}
	return o, nil
}

// save persists auth state to ~/.config/gogo/auth.json with 0600 perms.
func (o ogsModel) save() error {
	path, err := authPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(o, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

// clear removes persisted auth state (logout).
func (o ogsModel) clear() error {
	path, err := authPath()
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
