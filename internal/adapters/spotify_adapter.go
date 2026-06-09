package adapters

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	applog "github.com/seekandystroy/auto-setlist/internal"
	"github.com/seekandystroy/auto-setlist/internal/core/domain"
	"github.com/seekandystroy/auto-setlist/internal/ports"
)

type spotifyAdapter struct {
	clientID         string
	clientSecret     string
	redirectURI      string
	httpClient       *http.Client
	accountsBaseURL  string
	apiBaseURL       string
	tokenFilePath    string
	openBrowserFn    func(string) error
	callbackReceiver ports.SpotifyCallbackReceiver
	sleepFn          func(time.Duration)
}

type spotifySearchArtist struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type spotifySearchTrack struct {
	Name    string                `json:"name"`
	URI     string                `json:"uri"`
	Artists []spotifySearchArtist `json:"artists"`
}

type spotifySearchResponse struct {
	Tracks struct {
		Items []spotifySearchTrack `json:"items"`
	} `json:"tracks"`
}

type spotifyCreatePlaylistRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type spotifyAddItemsRequest struct {
	URIs     []string `json:"uris"`
	Position int      `json:"position"`
}

type spotifyPlaylist struct {
	ID string `json:"id"`
}

type spotifyToken struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	TokenType    string    `json:"token_type"`
	ExpiresAt    time.Time `json:"expires_at"`
	Scope        string    `json:"scope"`
}

const errNotRegisteredMsg = "spotify: users outside of allowlist not supported. Contact the maintainer to use auto-setlist."

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

type spotifyErrorResponse struct {
	Error struct {
		Message string `json:"message"`
	} `json:"error"`
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
		apiBaseURL:       "https://api.spotify.com/v1",
		tokenFilePath:    filepath.Join(dir, "spotify_token.json"),
		callbackReceiver: callbackReceiver,
	}
	a.openBrowserFn = defaultOpenBrowser
	a.sleepFn = time.Sleep
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
		slog.Info("Invalid token found, refreshing authentication with Spotify")
		refreshed, err := a.refreshAccessToken(token.RefreshToken)
		if err == nil {
			if saveErr := a.saveToken(refreshed); saveErr != nil {
				return "", saveErr
			}
			return refreshed.AccessToken, nil
		}
	}

	slog.Info("No token found, authenticating with Spotify")
	tok, err := a.runOAuthFlow()
	if err != nil {
		return "", err
	}
	return tok.AccessToken, nil
}

func (a *spotifyAdapter) GetSetlistTracks(ctx context.Context, token string, setlist domain.Setlist, includeCovers bool) ([]string, error) {
	applog.LoggerFromCtx(ctx).Info("Searching for tracks on Spotify", "include_covers", includeCovers, "count", len(setlist.Tracks))

	type result struct {
		uri   string
		found bool
		err   error
	}

	results := make([]result, len(setlist.Tracks))
	var wg sync.WaitGroup

	for i, track := range setlist.Tracks {
		wg.Add(1)
		go func(idx int, t domain.Track) {
			defer wg.Done()
			uri, found, err := a.searchTrack(ctx, token, t, setlist.Artist, includeCovers)
			results[idx] = result{uri: uri, found: found, err: err}
		}(i, track)
	}

	wg.Wait()

	var uris []string
	for _, r := range results {
		if r.err != nil {
			return nil, r.err
		}
		if r.found {
			uris = append(uris, r.uri)
		}
	}
	return uris, nil
}

func (a *spotifyAdapter) CreatePlaylist(ctx context.Context, token string, setlist domain.Setlist, uris []string, tourPlaylist bool) (string, error) {
	applog.LoggerFromCtx(ctx).Info("Creating playlist on Spotify", "artist", setlist.Artist.Name)

	var playlistName string
	if tourPlaylist && setlist.Tour != "" {
		playlistName = fmt.Sprintf("%s - %s setlist by auto-setlist", setlist.Artist.Name, setlist.Tour)
	} else {
		playlistName = fmt.Sprintf("%s setlist by auto-setlist", setlist.Artist.Name)
	}

	body := spotifyCreatePlaylistRequest{
		Name:        playlistName,
		Description: "auto-generated",
	}
	data, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("spotify: marshalling playlist request: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, a.apiBaseURL+"/me/playlists", bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("spotify: building create playlist request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Close = true

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("spotify: executing create playlist request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		if resp.StatusCode == http.StatusForbidden && strings.Contains(string(body), "The user is not registered for this application. Please check your settings on https://developer.spotify.com/dashboard.") {
			return "", errors.New(errNotRegisteredMsg)
		}
		return "", fmt.Errorf("spotify: unexpected status %d from create playlist", resp.StatusCode)
	}

	var playlist spotifyPlaylist
	if err := json.NewDecoder(resp.Body).Decode(&playlist); err != nil {
		return "", fmt.Errorf("spotify: decoding create playlist response: %w", err)
	}

	if len(uris) > 0 {
		if err := a.addTracksToPlaylist(token, playlist.ID, uris); err != nil {
			return "", err
		}
	}

	return playlist.ID, nil
}

func (a *spotifyAdapter) addTracksToPlaylist(token, playlistID string, uris []string) error {
	body := spotifyAddItemsRequest{URIs: uris, Position: 0}
	data, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("spotify: marshalling add items request: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, a.apiBaseURL+"/playlists/"+playlistID+"/items", bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("spotify: building add items request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Close = true

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("spotify: executing add items request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("spotify: unexpected status %d from add items", resp.StatusCode)
	}
	return nil
}

func (a *spotifyAdapter) doSearchLoop(ctx context.Context, token string, params url.Values) (spotifySearchResponse, error) {
	for {
		req, err := http.NewRequest(http.MethodGet, a.apiBaseURL+"/search?"+params.Encode(), nil)
		if err != nil {
			return spotifySearchResponse{}, fmt.Errorf("spotify: building search request: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Close = true

		resp, err := a.httpClient.Do(req)
		if err != nil {
			return spotifySearchResponse{}, fmt.Errorf("spotify: executing search request: %w", err)
		}

		if resp.StatusCode == http.StatusTooManyRequests {
			secs, _ := strconv.Atoi(resp.Header.Get("Retry-After"))
			resp.Body.Close()
			if secs <= 0 {
				secs = 1
			}
			applog.LoggerFromCtx(ctx).Warn("Search tracks rate limited, waiting and retrying", "wait", secs)
			a.sleepFn(time.Duration(secs) * time.Second)
			continue
		}

		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			if resp.StatusCode == http.StatusForbidden && strings.Contains(string(body), "The user is not registered for this application. Please check your settings on https://developer.spotify.com/dashboard.") {
				return spotifySearchResponse{}, errors.New(errNotRegisteredMsg)
			}
			var errResp spotifyErrorResponse
			if jsonErr := json.Unmarshal(body, &errResp); jsonErr == nil && errResp.Error.Message != "" {
				return spotifySearchResponse{}, fmt.Errorf("spotify: unexpected status %d from search: %s", resp.StatusCode, errResp.Error.Message)
			}
			return spotifySearchResponse{}, fmt.Errorf("spotify: unexpected status %d from search", resp.StatusCode)
		}

		var result spotifySearchResponse
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return spotifySearchResponse{}, fmt.Errorf("spotify: decoding search response: %w", err)
		}
		return result, nil
	}
}

func (a *spotifyAdapter) searchTrack(ctx context.Context, token string, track domain.Track, artist domain.Artist, fetchOriginalIfCover bool) (string, bool, error) {
	params := url.Values{
		"q":     {fmt.Sprintf("track:%s artist:%s", track.Name, artist.Name)},
		"type":  {"track"},
		"limit": {"5"},
	}

	result, err := a.doSearchLoop(ctx, token, params)
	if err != nil {
		return "", false, err
	}

	uri, found, err := a.trackURIFromResponse(track.Name, &result, true, artist.Name)
	if found {
		return uri, found, err
	}

	if !fetchOriginalIfCover || track.CoveredArtistName == "" {
		return "", false, nil
	}

	coverParams := url.Values{
		"q":     {fmt.Sprintf("track:%s artist:%s", track.Name, track.CoveredArtistName)},
		"type":  {"track"},
		"limit": {"5"},
	}
	coverResult, err := a.doSearchLoop(ctx, token, coverParams)
	if err != nil {
		return "", false, err
	}

	return a.trackURIFromResponse(track.Name, &coverResult, false, "")
}

func (a *spotifyAdapter) trackURIFromResponse(trackName string, resp *spotifySearchResponse, checkArtistName bool, artistName string) (string, bool, error) {
	remasterIdx := -1
	remixIdx := -1
	liveIdx := -1
	demoIdx := -1
	lcArtistName := strings.ToLower(artistName)
	lcTrackName := strings.ToLower(trackName)

	for i, item := range resp.Tracks.Items {
		after, found := strings.CutPrefix(strings.ToLower(item.Name), lcTrackName)
		if !found {
			continue
		}
		if !checkArtistName || (checkArtistName && slices.ContainsFunc(item.Artists, func(artist spotifySearchArtist) bool { return strings.ToLower(artist.Name) == lcArtistName })) {
			if after == "" {
				return item.URI, true, nil
			} else {
				lcAfter := strings.ToLower(after)
				if remasterIdx == -1 && strings.Contains(lcAfter, "remaster") {
					remasterIdx = i
				} else if liveIdx == -1 && strings.Contains(lcAfter, "live") {
					liveIdx = i
				} else if remixIdx == -1 && strings.Contains(lcAfter, "remix") {
					remixIdx = i
				} else if demoIdx == -1 {
					demoIdx = i
				}
			}
		}
	}

	if remasterIdx > -1 {
		return resp.Tracks.Items[remasterIdx].URI, true, nil
	} else if liveIdx > -1 {
		return resp.Tracks.Items[liveIdx].URI, true, nil
	} else if remixIdx > -1 {
		return resp.Tracks.Items[remixIdx].URI, true, nil
	} else if demoIdx > -1 {
		return resp.Tracks.Items[demoIdx].URI, true, nil
	}
	return "", false, nil
}

func (a *spotifyAdapter) buildAuthURL(state string) string {
	params := url.Values{
		"client_id":     {a.clientID},
		"response_type": {"code"},
		"redirect_uri":  {a.redirectURI},
		"scope":         {"playlist-modify-public playlist-modify-private"},
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
	req.Close = true

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
