package adapters

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/seekandystroy/auto-setlist/internal/ports"
)

type spotifyAdapter struct {
	clientID         string
	clientSecret     string
	redirectURI      string
	httpClient       *http.Client
	accountsBaseURL  string
	tokenFilePath    string
	openBrowserFn    func(string) error
	callbackReceiver ports.SpotifyCallbackReceiver
}

type spotifyToken struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	TokenType    string    `json:"token_type"`
	ExpiresAt    time.Time `json:"expires_at"`
	Scope        string    `json:"scope"`
}

func (t *spotifyToken) isValid() bool {
	if t == nil || t.AccessToken == "" {
		return false
	}
	return time.Now().Add(30 * time.Second).Before(t.ExpiresAt)
}

// spotifyTokenResponse mirrors the JSON returned by the Spotify token endpoint.
type spotifyTokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	Scope        string `json:"scope"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
}

func NewSpotifyAdapter(clientID, clientSecret string, callbackReceiver ports.SpotifyCallbackReceiver) (*spotifyAdapter, error) {
	dir, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("spotify: resolving working dir: %w", err)
	}
	a := &spotifyAdapter{
		clientID:         clientID,
		clientSecret:     clientSecret,
		redirectURI:      "http://127.0.0.1:8080/spotify_callback",
		httpClient:       &http.Client{Timeout: 10 * time.Second},
		accountsBaseURL:  "https://accounts.spotify.com",
		tokenFilePath:    filepath.Join(dir, "spotify_token.json"),
		callbackReceiver: callbackReceiver,
	}
	a.openBrowserFn = defaultOpenBrowser
	return a, nil
}

// GetValidToken returns a valid Spotify access token, refreshing or re-authorizing as needed.
func (a *spotifyAdapter) GetValidToken() (string, error) {
	token, err := a.loadToken()
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", err
	}

	if token != nil && token.isValid() {
		return token.AccessToken, nil
	}

	if token != nil && token.RefreshToken != "" {
		refreshed, err := a.refreshAccessToken(token.RefreshToken)
		if err == nil {
			if saveErr := a.saveToken(refreshed); saveErr != nil {
				return "", saveErr
			}
			return refreshed.AccessToken, nil
		}
	}

	tok, err := a.runOAuthFlow()
	if err != nil {
		return "", err
	}
	return tok.AccessToken, nil
}

func (a *spotifyAdapter) buildAuthURL(state string) string {
	params := url.Values{
		"client_id":     {a.clientID},
		"response_type": {"code"},
		"redirect_uri":  {a.redirectURI},
		"scope":         {"playlist-modify-private"},
		"state":         {state},
	}
	return a.accountsBaseURL + "/authorize?" + params.Encode()
}

func (a *spotifyAdapter) exchangeCode(code string) (*spotifyToken, error) {
	body := url.Values{
		"grant_type":   {"authorization_code"},
		"code":         {code},
		"redirect_uri": {a.redirectURI},
	}
	return a.postToken(body, "")
}

func (a *spotifyAdapter) refreshAccessToken(refreshToken string) (*spotifyToken, error) {
	body := url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken},
	}
	return a.postToken(body, refreshToken)
}

// postToken sends a POST to /api/token and decodes the response.
// existingRefreshToken is used as a fallback when Spotify doesn't rotate the refresh token.
func (a *spotifyAdapter) postToken(body url.Values, existingRefreshToken string) (*spotifyToken, error) {
	req, err := http.NewRequest(http.MethodPost, a.accountsBaseURL+"/api/token", strings.NewReader(body.Encode()))
	if err != nil {
		return nil, fmt.Errorf("spotify: building token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString(
		[]byte(a.clientID+":"+a.clientSecret),
	))

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("spotify: executing token request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("spotify: unexpected status %d from token endpoint", resp.StatusCode)
	}

	var decoded spotifyTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return nil, fmt.Errorf("spotify: decoding token response: %w", err)
	}

	refreshToken := decoded.RefreshToken
	if refreshToken == "" {
		refreshToken = existingRefreshToken
	}

	return &spotifyToken{
		AccessToken:  decoded.AccessToken,
		RefreshToken: refreshToken,
		TokenType:    decoded.TokenType,
		Scope:        decoded.Scope,
		ExpiresAt:    time.Now().Add(time.Duration(decoded.ExpiresIn) * time.Second),
	}, nil
}

func (a *spotifyAdapter) saveToken(token *spotifyToken) error {
	dir := filepath.Dir(a.tokenFilePath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("spotify: creating config dir: %w", err)
	}
	data, err := json.Marshal(token)
	if err != nil {
		return fmt.Errorf("spotify: marshalling token: %w", err)
	}
	tmp := a.tokenFilePath + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return fmt.Errorf("spotify: writing token file: %w", err)
	}
	if err := os.Rename(tmp, a.tokenFilePath); err != nil {
		return fmt.Errorf("spotify: renaming token file: %w", err)
	}
	return nil
}

func (a *spotifyAdapter) loadToken() (*spotifyToken, error) {
	data, err := os.ReadFile(a.tokenFilePath)
	if err != nil {
		return nil, err
	}
	var token spotifyToken
	if err := json.Unmarshal(data, &token); err != nil {
		return nil, fmt.Errorf("spotify: parsing token file: %w", err)
	}
	return &token, nil
}

func (a *spotifyAdapter) runOAuthFlow() (*spotifyToken, error) {
	state, err := generateState()
	if err != nil {
		return nil, err
	}

	authURL := a.buildAuthURL(state)

	fmt.Fprintln(os.Stderr, "Opening browser for Spotify authorization...")
	if err := a.openBrowserFn(authURL); err != nil {
		fmt.Fprintf(os.Stderr, "Could not open browser automatically. Please visit:\n%s\n", authURL)
	}

	code, err := a.callbackReceiver.WaitForCode(context.Background(), state)
	if err != nil {
		return nil, err
	}

	token, err := a.exchangeCode(code)
	if err != nil {
		return nil, err
	}

	if err := a.saveToken(token); err != nil {
		return nil, err
	}

	return token, nil
}

func generateState() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("spotify: generating state: %w", err)
	}
	return hex.EncodeToString(b), nil
}

func defaultOpenBrowser(rawURL string) error {
	var cmd string
	switch runtime.GOOS {
	case "darwin":
		cmd = "open"
	case "linux":
		cmd = "xdg-open"
	default:
		fmt.Printf("Open this URL in your browser:\n%s\n", rawURL)
		return nil
	}
	return exec.Command(cmd, rawURL).Start()
}
