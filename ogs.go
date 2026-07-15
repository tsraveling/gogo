package main

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// @region ogs:auth

// Lives under ~/.config/gogo/.
const authFileName = "auth.json"

// OGS host, token endpoint (trailing slash required), and profile endpoint.
const ogsBaseURL = "https://online-go.com"
const oauthTokenURL = ogsBaseURL + "/oauth2/token/"
const meURL = ogsBaseURL + "/api/v1/me"

// Our registered OGS OAuth application id. It is a public
// client (aka not a secret), so it ships in source.
const oauthClientID = "JsbA91sZqZ5ytnZhTDRwiCa2T8AK3zIw8bS9fjsj"

var httpClient = &http.Client{}

// Rejected refresh token (stale login).
var errInvalidRefresh = errors.New("invalid refresh token")

// OGS /oauth2/token/ payload.
type oauthResponse struct {
	AccessToken      string `json:"access_token"`
	RefreshToken     string `json:"refresh_token"`
	ExpiresIn        int    `json:"expires_in"`
	TokenType        string `json:"token_type"`
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
}

// Subset of /api/v1/me we keep.
type ogsPlayer struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
}

// OGS auth state. Data only — no UI.
type ogsModel struct {
	Username     string `json:"username"`
	UserID       int64  `json:"user_id"`
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

// Reports whether we hold tokens.
func (o ogsModel) authenticated() bool {
	return o.AccessToken != "" && o.RefreshToken != ""
}

// Resolves ~/.config/gogo/auth.json.
func authPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "gogo", authFileName), nil
}

// Reads persisted auth state. Missing file yields an empty (unauthed) model.
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

// Persists auth state to ~/.config/gogo/auth.json with 0600 perms.
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

// Removes persisted auth state (logout).
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

// @region ogs:auth-net

// Exchanges credentials for tokens via the OAuth password grant, then fetches the profile.
func authenticatePassword(username, password string) (ogsModel, error) {
	if username == "" || password == "" {
		return ogsModel{}, errors.New("username and password required")
	}
	v := url.Values{}
	v.Set("client_id", oauthClientID)
	v.Set("grant_type", "password")
	v.Set("username", username)
	v.Set("password", password)
	return tokenExchange(v)
}

// Trades a refresh token for a fresh access token.
func authenticateRefresh(refreshToken string) (ogsModel, error) {
	v := url.Values{}
	v.Set("client_id", oauthClientID)
	v.Set("grant_type", "refresh_token")
	v.Set("refresh_token", refreshToken)
	o, err := tokenExchange(v)
	if err != nil {
		return o, errInvalidRefresh
	}
	return o, nil
}

// POSTs the form to the token endpoint and resolves the player.
func tokenExchange(v url.Values) (ogsModel, error) {
	var res oauthResponse
	if err := postForm(oauthTokenURL, v, &res); err != nil {
		if res.Error != "" {
			return ogsModel{}, errors.New(oauthErr(res))
		}
		return ogsModel{}, err
	}
	if res.Error != "" {
		return ogsModel{}, errors.New(oauthErr(res))
	}

	player, err := fetchPlayer(res.AccessToken)
	if err != nil {
		return ogsModel{}, err
	}
	return ogsModel{
		Username:     player.Username,
		UserID:       player.ID,
		AccessToken:  res.AccessToken,
		RefreshToken: res.RefreshToken,
	}, nil
}

// Prefers the human-readable description.
func oauthErr(r oauthResponse) string {
	if r.ErrorDescription != "" {
		return r.ErrorDescription
	}
	return r.Error
}

// Reads /api/v1/me with the given access token.
func fetchPlayer(accessToken string) (ogsPlayer, error) {
	var p ogsPlayer
	err := authGet(meURL, accessToken, &p)
	return p, err
}

// Performs a Bearer-authenticated GET and unmarshals the JSON response.
func authGet(rawURL, accessToken string, out any) error {
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return errors.New("ogs request failed: " + resp.Status)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	return json.Unmarshal(body, out)
}

// Submits url-encoded values and unmarshals the JSON response.
// A non-200 status still unmarshals the body so callers can read oauth errors.
func postForm(rawURL string, v url.Values, out any) error {
	req, err := http.NewRequest(http.MethodPost, rawURL, strings.NewReader(v.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	_ = json.Unmarshal(body, out)
	if resp.StatusCode != http.StatusOK {
		return errors.New("oauth request failed: " + resp.Status)
	}
	return nil
}
