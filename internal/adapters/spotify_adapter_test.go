package adapters

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/seekandystroy/auto-setlist/internal/core/domain"
)

type mockCallbackReceiver struct {
	code string
	err  error
}

func (m *mockCallbackReceiver) WaitForCode(_ context.Context, _ string) (string, error) {
	return m.code, m.err
}

// newTestAccountsAdapter returns a spotifyAdapter wired to the given test server URL
// and a temp directory for token file storage.
func newTestAccountsAdapter(t *testing.T, serverURL string) *spotifyAdapter {
	t.Helper()
	return &spotifyAdapter{
		clientID:         "test-client-id",
		clientSecret:     "test-client-secret",
		redirectURI:      "http://127.0.0.1:8080/spotify_callback",
		httpClient:       &http.Client{Timeout: 5 * time.Second},
		accountsBaseURL:  serverURL,
		tokenFilePath:    filepath.Join(t.TempDir(), "spotify_token.json"),
		openBrowserFn:    func(string) error { return nil },
		callbackReceiver: &mockCallbackReceiver{err: errors.New("unexpected callback call")},
		sleepFn:          func(time.Duration) {},
	}
}

func writeTokenResponseJSON(w http.ResponseWriter, accessToken, refreshToken string, expiresIn int) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"access_token":  accessToken,
		"token_type":    "Bearer",
		"scope":         "playlist-modify-private",
		"expires_in":    expiresIn,
		"refresh_token": refreshToken,
	})
}

func TestExchangeCode_HappyPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/token" {
			http.NotFound(w, r)
			return
		}
		writeTokenResponseJSON(w, "access-token-1", "refresh-token-1", 3600)
	}))
	defer srv.Close()

	a := newTestAccountsAdapter(t, srv.URL)
	before := time.Now()
	token, err := a.exchangeCode("auth-code")
	after := time.Now()

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token.AccessToken != "access-token-1" {
		t.Errorf("unexpected access token: %s", token.AccessToken)
	}
	if token.RefreshToken != "refresh-token-1" {
		t.Errorf("unexpected refresh token: %s", token.RefreshToken)
	}
	minExpiry := before.Add(3599 * time.Second)
	maxExpiry := after.Add(3601 * time.Second)
	if token.ExpiresAt.Before(minExpiry) || token.ExpiresAt.After(maxExpiry) {
		t.Errorf("ExpiresAt %v not within expected range", token.ExpiresAt)
	}
}

func TestExchangeCode_NonOKStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad request", http.StatusBadRequest)
	}))
	defer srv.Close()

	a := newTestAccountsAdapter(t, srv.URL)
	_, err := a.exchangeCode("auth-code")
	if err == nil {
		t.Fatal("expected error for non-200 status")
	}
	if !strings.Contains(err.Error(), "400") {
		t.Errorf("expected error to mention status 400, got: %v", err)
	}
}

func TestExchangeCode_MalformedJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{not valid json`))
	}))
	defer srv.Close()

	a := newTestAccountsAdapter(t, srv.URL)
	_, err := a.exchangeCode("auth-code")
	if err == nil {
		t.Fatal("expected error for malformed JSON")
	}
	if !strings.Contains(err.Error(), "decoding") {
		t.Errorf("expected decoding error, got: %v", err)
	}
}

func TestExchangeCode_SendsBasicAuth(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		writeTokenResponseJSON(w, "tok", "rtok", 3600)
	}))
	defer srv.Close()

	a := newTestAccountsAdapter(t, srv.URL)
	a.exchangeCode("code")

	if !strings.HasPrefix(gotAuth, "Basic ") {
		t.Errorf("expected Basic auth header, got: %s", gotAuth)
	}
}

func TestRefreshAccessToken_HappyPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Response omits refresh_token — existing one should be preserved.
		json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token": "new-access-token",
			"token_type":   "Bearer",
			"scope":        "playlist-modify-private",
			"expires_in":   3600,
		})
	}))
	defer srv.Close()

	a := newTestAccountsAdapter(t, srv.URL)
	token, err := a.refreshAccessToken("old-refresh-token")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token.AccessToken != "new-access-token" {
		t.Errorf("unexpected access token: %s", token.AccessToken)
	}
	if token.RefreshToken != "old-refresh-token" {
		t.Errorf("expected old refresh token to be preserved, got: %s", token.RefreshToken)
	}
}

func TestRefreshAccessToken_RotatesRefreshToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeTokenResponseJSON(w, "new-access", "new-refresh", 3600)
	}))
	defer srv.Close()

	a := newTestAccountsAdapter(t, srv.URL)
	token, err := a.refreshAccessToken("old-refresh-token")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token.RefreshToken != "new-refresh" {
		t.Errorf("expected new refresh token, got: %s", token.RefreshToken)
	}
}

func TestSaveAndLoadToken_RoundTrip(t *testing.T) {
	a := newTestAccountsAdapter(t, "")
	original := &spotifyToken{
		AccessToken:  "access",
		RefreshToken: "refresh",
		TokenType:    "Bearer",
		Scope:        "playlist-modify-private",
		ExpiresAt:    time.Now().Add(time.Hour).Truncate(time.Second),
	}

	if err := a.saveToken(original); err != nil {
		t.Fatalf("saveToken: %v", err)
	}

	info, err := os.Stat(a.tokenFilePath)
	if err != nil {
		t.Fatalf("stat token file: %v", err)
	}
	if info.Mode().Perm() != 0600 {
		t.Errorf("expected file mode 0600, got %v", info.Mode().Perm())
	}

	loaded, err := a.loadToken()
	if err != nil {
		t.Fatalf("loadToken: %v", err)
	}
	if loaded.AccessToken != original.AccessToken {
		t.Errorf("AccessToken mismatch: %s", loaded.AccessToken)
	}
	if loaded.RefreshToken != original.RefreshToken {
		t.Errorf("RefreshToken mismatch: %s", loaded.RefreshToken)
	}
	if !loaded.ExpiresAt.Equal(original.ExpiresAt) {
		t.Errorf("ExpiresAt mismatch: %v vs %v", loaded.ExpiresAt, original.ExpiresAt)
	}
}

func TestLoadToken_FileNotExist(t *testing.T) {
	a := newTestAccountsAdapter(t, "")
	_, err := a.loadToken()
	if err == nil {
		t.Fatal("expected error for missing file")
	}
	if !os.IsNotExist(err) {
		t.Errorf("expected os.IsNotExist error, got: %v", err)
	}
}

func TestGetValidToken_UsesValidCachedToken(t *testing.T) {
	// Server should never be called.
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		writeTokenResponseJSON(w, "should-not-be-called", "r", 3600)
	}))
	defer srv.Close()

	a := newTestAccountsAdapter(t, srv.URL)
	validToken := &spotifyToken{
		AccessToken:  "cached-token",
		RefreshToken: "cached-refresh",
		TokenType:    "Bearer",
		ExpiresAt:    time.Now().Add(time.Hour),
	}
	if err := a.saveToken(validToken); err != nil {
		t.Fatalf("saveToken: %v", err)
	}

	token, err := a.GetValidToken()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token != "cached-token" {
		t.Errorf("expected cached token, got: %s", token)
	}
	if called {
		t.Error("expected no HTTP call for valid cached token")
	}
}

func TestGetValidToken_RefreshesExpiredToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/token" {
			http.NotFound(w, r)
			return
		}
		writeTokenResponseJSON(w, "refreshed-access", "refreshed-refresh", 3600)
	}))
	defer srv.Close()

	a := newTestAccountsAdapter(t, srv.URL)
	expiredToken := &spotifyToken{
		AccessToken:  "old-access",
		RefreshToken: "old-refresh",
		TokenType:    "Bearer",
		ExpiresAt:    time.Now().Add(-time.Hour), // expired
	}
	if err := a.saveToken(expiredToken); err != nil {
		t.Fatalf("saveToken: %v", err)
	}

	token, err := a.GetValidToken()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token != "refreshed-access" {
		t.Errorf("expected refreshed access token, got: %s", token)
	}

	// Verify the new token was persisted.
	saved, err := a.loadToken()
	if err != nil {
		t.Fatalf("loadToken after refresh: %v", err)
	}
	if saved.AccessToken != "refreshed-access" {
		t.Errorf("expected saved refreshed token, got: %s", saved.AccessToken)
	}
}

func TestRunOAuthFlow_HappyPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/token" {
			http.NotFound(w, r)
			return
		}
		writeTokenResponseJSON(w, "oauth-access", "oauth-refresh", 3600)
	}))
	defer srv.Close()

	a := newTestAccountsAdapter(t, srv.URL)

	a.callbackReceiver = &mockCallbackReceiver{code: "auth-code-from-browser"}
	token, err := a.GetValidToken()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token != "oauth-access" {
		t.Errorf("expected oauth-access, got: %s", token)
	}

	saved, err := a.loadToken()
	if err != nil {
		t.Fatalf("loadToken after oauth flow: %v", err)
	}
	if saved.AccessToken != "oauth-access" {
		t.Errorf("expected saved oauth-access, got: %s", saved.AccessToken)
	}
}

func TestRunOAuthFlow_CallbackError(t *testing.T) {
	a := newTestAccountsAdapter(t, "")

	a.callbackReceiver = &mockCallbackReceiver{err: errors.New("user closed browser")}
	_, err := a.GetValidToken()
	if err == nil {
		t.Fatal("expected error when callback fails")
	}
	if !strings.Contains(err.Error(), "user closed browser") {
		t.Errorf("expected callback error message, got: %v", err)
	}
}

func TestBuildAuthURL_ContainsRequiredParams(t *testing.T) {
	a := newTestAccountsAdapter(t, "https://accounts.spotify.com")
	authURL := a.buildAuthURL("my-state")

	checks := []string{
		"client_id=test-client-id",
		"response_type=code",
		"scope=",
		"state=my-state",
		"redirect_uri=",
	}
	for _, check := range checks {
		if !strings.Contains(authURL, check) {
			t.Errorf("auth URL missing %q: %s", check, authURL)
		}
	}
}

func makeSearchResponse(tracks [][3]string) spotifySearchResponse {
	items := make([]spotifySearchTrack, len(tracks))
	for i, t := range tracks {
		items[i] = spotifySearchTrack{Name: t[0], URI: t[1], Artists: []spotifySearchArtist{{Name: t[2]}}}
	}
	return spotifySearchResponse{Tracks: struct {
		Items []spotifySearchTrack `json:"items"`
	}{Items: items}}
}

func TestGetSetlistTracks_ReturnsURIs(t *testing.T) {
	setlist := domain.Setlist{
		Artist: domain.Artist{MBID: "m1", Name: "Pitbull"},
		Tracks: []domain.Track{{Name: "Give Me Everything"}, {Name: "Timber"}},
	}

	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("q")
		var resp spotifySearchResponse
		if strings.Contains(q, "Give+Me+Everything") || strings.Contains(q, "Give Me Everything") {
			resp = makeSearchResponse([][3]string{{"Give Me Everything", "spotify:track:uri1", "Pitbull"}})
		} else {
			resp = makeSearchResponse([][3]string{{"Timber", "spotify:track:uri2", "Pitbull"}})
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer apiSrv.Close()

	a := newTestAccountsAdapter(t, "")
	a.apiBaseURL = apiSrv.URL

	uris, err := a.GetSetlistTracks(context.Background(), "test-token", setlist, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(uris) != 2 {
		t.Fatalf("expected 2 URIs, got %d", len(uris))
	}
	if uris[0] != "spotify:track:uri1" {
		t.Errorf("unexpected first URI: %s", uris[0])
	}
}

func TestGetSetlistTracks_SkipsTrackNotFound(t *testing.T) {
	setlist := domain.Setlist{
		Artist: domain.Artist{Name: "Artist"},
		Tracks: []domain.Track{{Name: "Known Song"}, {Name: "Unknown Song"}},
	}

	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("q")
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(q, "Known") {
			json.NewEncoder(w).Encode(makeSearchResponse([][3]string{{"Known Song", "spotify:track:uri1", "Artist"}}))
		} else {
			json.NewEncoder(w).Encode(spotifySearchResponse{})
		}
	}))
	defer apiSrv.Close()

	a := newTestAccountsAdapter(t, "")
	a.apiBaseURL = apiSrv.URL

	uris, err := a.GetSetlistTracks(context.Background(), "test-token", setlist, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(uris) != 1 {
		t.Errorf("expected 1 URI (unknown track skipped), got %d", len(uris))
	}
}

func TestGetSetlistTracks_SkipsWrongArtist(t *testing.T) {
	setlist := domain.Setlist{
		Artist: domain.Artist{Name: "Artist"},
		Tracks: []domain.Track{{Name: "Song"}},
	}

	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Returns a track with the right name but wrong artist ID.
		json.NewEncoder(w).Encode(makeSearchResponse([][3]string{{"Song", "spotify:track:uri1", "wrong-artist"}}))
	}))
	defer apiSrv.Close()

	a := newTestAccountsAdapter(t, "")
	a.apiBaseURL = apiSrv.URL

	uris, err := a.GetSetlistTracks(context.Background(), "test-token", setlist, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(uris) != 0 {
		t.Errorf("expected 0 URIs (wrong artist skipped), got %d", len(uris))
	}
}

func TestGetSetlistTracks_NonOKStatusReturnsError(t *testing.T) {
	setlist := domain.Setlist{
		Artist: domain.Artist{Name: "Artist"},
		Tracks: []domain.Track{{Name: "Song"}},
	}

	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer apiSrv.Close()

	a := newTestAccountsAdapter(t, "")
	a.apiBaseURL = apiSrv.URL

	_, err := a.GetSetlistTracks(context.Background(), "test-token", setlist, false)
	if err == nil {
		t.Fatal("expected error for non-OK status, got nil")
	}
	if !strings.Contains(err.Error(), "unexpected status") {
		t.Errorf("expected 'unexpected status' in error, got %q", err.Error())
	}
}

func TestGetSetlistTracks_403NotRegistered(t *testing.T) {
	setlist := domain.Setlist{
		Artist: domain.Artist{Name: "Artist"},
		Tracks: []domain.Track{{Name: "Song"}},
	}

	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte("The user is not registered for this application. Please check your settings on https://developer.spotify.com/dashboard."))
	}))
	defer apiSrv.Close()

	a := newTestAccountsAdapter(t, "")
	a.apiBaseURL = apiSrv.URL

	_, err := a.GetSetlistTracks(context.Background(), "test-token", setlist, false)
	if err == nil {
		t.Fatal("expected error for 403 not registered, got nil")
	}
	want := "spotify: users outside of allowlist not supported. Contact the maintainer to use auto-setlist."
	if err.Error() != want {
		t.Errorf("expected %q, got %q", want, err.Error())
	}
}

func TestGetSetlistTracks_SendsBearerToken(t *testing.T) {
	var gotAuth string

	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(spotifySearchResponse{})
	}))
	defer apiSrv.Close()

	a := newTestAccountsAdapter(t, "")
	a.apiBaseURL = apiSrv.URL

	a.GetSetlistTracks(context.Background(), "test-access-token", domain.Setlist{Tracks: []domain.Track{{Name: "Song"}}}, false)

	if !strings.HasPrefix(gotAuth, "Bearer ") {
		t.Errorf("expected Bearer token, got: %s", gotAuth)
	}
	if !strings.Contains(gotAuth, "test-access-token") {
		t.Errorf("expected access token in header, got: %s", gotAuth)
	}
}

func TestCreatePlaylist_403NotRegistered(t *testing.T) {
	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte("The user is not registered for this application. Please check your settings on https://developer.spotify.com/dashboard."))
	}))
	defer apiSrv.Close()

	a := newTestAccountsAdapter(t, "")
	a.apiBaseURL = apiSrv.URL

	_, err := a.CreatePlaylist(context.Background(), "test-token", domain.Setlist{}, nil, false)
	if err == nil {
		t.Fatal("expected error for 403 not registered, got nil")
	}
	want := "spotify: users outside of allowlist not supported. Contact the maintainer to use auto-setlist."
	if err.Error() != want {
		t.Errorf("expected %q, got %q", want, err.Error())
	}
}

func TestCreatePlaylist_HappyPath(t *testing.T) {
	setlist := domain.Setlist{Artist: domain.Artist{Name: "Pitbull"}}

	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(spotifyPlaylist{ID: "playlist-xyz"})
	}))
	defer apiSrv.Close()

	a := newTestAccountsAdapter(t, "")
	a.apiBaseURL = apiSrv.URL

	id, err := a.CreatePlaylist(context.Background(), "test-token", setlist, nil, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != "playlist-xyz" {
		t.Errorf("expected playlist ID %q, got %q", "playlist-xyz", id)
	}
}

func TestCreatePlaylist_AddsTracksToPlaylist(t *testing.T) {
	setlist := domain.Setlist{Artist: domain.Artist{Name: "Pitbull"}}
	uris := []string{"spotify:track:a", "spotify:track:b"}
	var gotAddBody spotifyAddItemsRequest

	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/me/playlists" {
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(spotifyPlaylist{ID: "playlist-xyz"})
		} else {
			json.NewDecoder(r.Body).Decode(&gotAddBody)
			w.WriteHeader(http.StatusCreated)
		}
	}))
	defer apiSrv.Close()

	a := newTestAccountsAdapter(t, "")
	a.apiBaseURL = apiSrv.URL

	_, err := a.CreatePlaylist(context.Background(), "test-token", setlist, uris, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(gotAddBody.URIs) != 2 {
		t.Fatalf("expected 2 URIs in add-items body, got %d", len(gotAddBody.URIs))
	}
	if gotAddBody.URIs[0] != "spotify:track:a" {
		t.Errorf("unexpected first URI: %s", gotAddBody.URIs[0])
	}
	if gotAddBody.Position != 0 {
		t.Errorf("expected position 0, got %d", gotAddBody.Position)
	}
}

func TestCreatePlaylist_AddTracksNonOKStatus(t *testing.T) {
	setlist := domain.Setlist{Artist: domain.Artist{Name: "Pitbull"}}

	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/me/playlists" {
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(spotifyPlaylist{ID: "playlist-xyz"})
		} else {
			w.WriteHeader(http.StatusForbidden)
		}
	}))
	defer apiSrv.Close()

	a := newTestAccountsAdapter(t, "")
	a.apiBaseURL = apiSrv.URL

	_, err := a.CreatePlaylist(context.Background(), "test-token", setlist, []string{"spotify:track:a"}, false)
	if err == nil {
		t.Fatal("expected error from add items, got nil")
	}
	if !strings.Contains(err.Error(), "unexpected status") {
		t.Errorf("expected 'unexpected status' in error, got %q", err.Error())
	}
}

func TestCreatePlaylist_PlaylistNameWithoutTour(t *testing.T) {
	var gotBody spotifyCreatePlaylistRequest
	setlist := domain.Setlist{Artist: domain.Artist{Name: "Pitbull"}}

	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(spotifyPlaylist{ID: "x"})
	}))
	defer apiSrv.Close()

	a := newTestAccountsAdapter(t, "")
	a.apiBaseURL = apiSrv.URL

	a.CreatePlaylist(context.Background(), "test-token", setlist, nil, false)

	wantName := "Pitbull setlist by auto-setlist"
	if gotBody.Name != wantName {
		t.Errorf("expected name %q, got %q", wantName, gotBody.Name)
	}
	if gotBody.Description != "auto-generated" {
		t.Errorf("expected description %q, got %q", "auto-generated", gotBody.Description)
	}
}

func TestCreatePlaylist_PlaylistNameWithTour(t *testing.T) {
	var gotBody spotifyCreatePlaylistRequest
	setlist := domain.Setlist{Artist: domain.Artist{Name: "Pitbull"}, Tour: "Worldwide"}

	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(spotifyPlaylist{ID: "x"})
	}))
	defer apiSrv.Close()

	a := newTestAccountsAdapter(t, "")
	a.apiBaseURL = apiSrv.URL

	a.CreatePlaylist(context.Background(), "test-token", setlist, nil, true)

	wantName := "Pitbull - Worldwide setlist by auto-setlist"
	if gotBody.Name != wantName {
		t.Errorf("expected name %q, got %q", wantName, gotBody.Name)
	}
	if gotBody.Description != "auto-generated" {
		t.Errorf("expected description %q, got %q", "auto-generated", gotBody.Description)
	}
}

func TestCreatePlaylist_PlaylistNameTourIgnoredWhenTourPlaylistFalse(t *testing.T) {
	var gotBody spotifyCreatePlaylistRequest
	setlist := domain.Setlist{Artist: domain.Artist{Name: "Pitbull"}, Tour: "Worldwide"}

	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(spotifyPlaylist{ID: "x"})
	}))
	defer apiSrv.Close()

	a := newTestAccountsAdapter(t, "")
	a.apiBaseURL = apiSrv.URL

	a.CreatePlaylist(context.Background(), "test-token", setlist, nil, false)

	wantName := "Pitbull setlist by auto-setlist"
	if gotBody.Name != wantName {
		t.Errorf("expected name %q, got %q", wantName, gotBody.Name)
	}
}

func TestCreatePlaylist_SendsCorrectHeaders(t *testing.T) {
	var gotAuth, gotContentType string

	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotContentType = r.Header.Get("Content-Type")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(spotifyPlaylist{ID: "x"})
	}))
	defer apiSrv.Close()

	a := newTestAccountsAdapter(t, "")
	a.apiBaseURL = apiSrv.URL

	a.CreatePlaylist(context.Background(), "test-access-token", domain.Setlist{}, nil, false)

	if !strings.HasPrefix(gotAuth, "Bearer ") {
		t.Errorf("expected Bearer auth, got: %s", gotAuth)
	}
	if !strings.Contains(gotAuth, "test-access-token") {
		t.Errorf("expected access token in header, got: %s", gotAuth)
	}
	if gotContentType != "application/json" {
		t.Errorf("expected Content-Type application/json, got: %s", gotContentType)
	}
}

func TestCreatePlaylist_NonOKStatus(t *testing.T) {
	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer apiSrv.Close()

	a := newTestAccountsAdapter(t, "")
	a.apiBaseURL = apiSrv.URL

	_, err := a.CreatePlaylist(context.Background(), "test-token", domain.Setlist{}, nil, false)
	if err == nil {
		t.Fatal("expected error for non-201 status, got nil")
	}
	if !strings.Contains(err.Error(), "unexpected status") {
		t.Errorf("expected 'unexpected status' in error, got %q", err.Error())
	}
}

func TestCreatePlaylist_MalformedJSON(t *testing.T) {
	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte("not json {{{"))
	}))
	defer apiSrv.Close()

	a := newTestAccountsAdapter(t, "")
	a.apiBaseURL = apiSrv.URL

	_, err := a.CreatePlaylist(context.Background(), "test-token", domain.Setlist{}, nil, false)
	if err == nil {
		t.Fatal("expected decode error, got nil")
	}
	if !strings.Contains(err.Error(), "decoding create playlist response") {
		t.Errorf("expected 'decoding create playlist response' in error, got %q", err.Error())
	}
}

func TestGetSetlistTracks_401InvalidToken(t *testing.T) {
	setlist := domain.Setlist{
		Artist: domain.Artist{Name: "Artist"},
		Tracks: []domain.Track{{Name: "Song"}},
	}

	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer apiSrv.Close()

	a := newTestAccountsAdapter(t, "")
	a.apiBaseURL = apiSrv.URL

	_, err := a.GetSetlistTracks(context.Background(), "expired-token", setlist, false)
	if err == nil {
		t.Fatal("expected error for 401, got nil")
	}
	if !strings.Contains(err.Error(), "401") {
		t.Errorf("expected error to mention 401, got %q", err.Error())
	}
}

func TestCreatePlaylist_401InvalidToken(t *testing.T) {
	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer apiSrv.Close()

	a := newTestAccountsAdapter(t, "")
	a.apiBaseURL = apiSrv.URL

	_, err := a.CreatePlaylist(context.Background(), "expired-token", domain.Setlist{}, nil, false)
	if err == nil {
		t.Fatal("expected error for 401, got nil")
	}
	if !strings.Contains(err.Error(), "401") {
		t.Errorf("expected error to mention 401, got %q", err.Error())
	}
}

func TestSearchTrack_429RetriesAfterDelay(t *testing.T) {
	var callCount atomic.Int32
	var sleptFor atomic.Int64

	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := callCount.Add(1)
		if n == 1 {
			w.Header().Set("Retry-After", "3")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(makeSearchResponse([][3]string{{"Song", "spotify:track:uri1", "Artist"}}))
	}))
	defer apiSrv.Close()

	a := newTestAccountsAdapter(t, "")
	a.apiBaseURL = apiSrv.URL
	a.sleepFn = func(d time.Duration) { sleptFor.Add(int64(d)) }

	artist := domain.Artist{Name: "Artist"}
	uri, found, err := a.searchTrack(context.Background(), "token", domain.Track{Name: "Song"}, artist, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !found {
		t.Fatal("expected track to be found after retry")
	}
	if uri != "spotify:track:uri1" {
		t.Errorf("unexpected URI: %s", uri)
	}
	if callCount.Load() != 2 {
		t.Errorf("expected 2 requests (1 retry), got %d", callCount.Load())
	}
	if sleptFor.Load() != int64(3*time.Second) {
		t.Errorf("expected sleep of 3s, got %v", time.Duration(sleptFor.Load()))
	}
}

func TestSearchTrack_429MissingRetryAfterDefaultsToOne(t *testing.T) {
	var callCount atomic.Int32
	var sleptFor atomic.Int64

	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := callCount.Add(1)
		if n == 1 {
			// No Retry-After header.
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(makeSearchResponse([][3]string{{"Song", "spotify:track:uri1", "Artist"}}))
	}))
	defer apiSrv.Close()

	a := newTestAccountsAdapter(t, "")
	a.apiBaseURL = apiSrv.URL
	a.sleepFn = func(d time.Duration) { sleptFor.Add(int64(d)) }

	uri, found, err := a.searchTrack(context.Background(), "token", domain.Track{Name: "Song"}, domain.Artist{Name: "Artist"}, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !found {
		t.Fatal("expected track to be found")
	}
	_ = uri
	if sleptFor.Load() != int64(time.Second) {
		t.Errorf("expected default sleep of 1s, got %v", time.Duration(sleptFor.Load()))
	}
}

func TestGetSetlistTracks_PreservesOrder(t *testing.T) {
	setlist := domain.Setlist{
		Artist: domain.Artist{Name: "Artist"},
		Tracks: []domain.Track{{Name: "Alpha"}, {Name: "Beta"}, {Name: "Gamma"}},
	}

	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("q")
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(q, "Alpha"):
			json.NewEncoder(w).Encode(makeSearchResponse([][3]string{{"Alpha", "spotify:track:alpha", "Artist"}}))
		case strings.Contains(q, "Beta"):
			// Simulate Beta being slower to verify ordering is index-based, not arrival-based.
			time.Sleep(10 * time.Millisecond)
			json.NewEncoder(w).Encode(makeSearchResponse([][3]string{{"Beta", "spotify:track:beta", "Artist"}}))
		case strings.Contains(q, "Gamma"):
			json.NewEncoder(w).Encode(makeSearchResponse([][3]string{{"Gamma", "spotify:track:gamma", "Artist"}}))
		}
	}))
	defer apiSrv.Close()

	a := newTestAccountsAdapter(t, "")
	a.apiBaseURL = apiSrv.URL

	uris, err := a.GetSetlistTracks(context.Background(), "token", setlist, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"spotify:track:alpha", "spotify:track:beta", "spotify:track:gamma"}
	if len(uris) != len(want) {
		t.Fatalf("expected %d URIs, got %d", len(want), len(uris))
	}
	for i, w := range want {
		if uris[i] != w {
			t.Errorf("position %d: expected %s, got %s", i, w, uris[i])
		}
	}
}

func TestSearchTrack_MatchesDirectMatchOverVerboseNames(t *testing.T) {
	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(makeSearchResponse([][3]string{
			{"Wasted Years - 2015 Remaster", "spotify:track:uri1", "Iron Maiden"},
			{"Wasted Years", "spotify:track:correct-uri", "Iron Maiden"}}))
	}))
	defer apiSrv.Close()

	a := newTestAccountsAdapter(t, "")
	a.apiBaseURL = apiSrv.URL

	track := domain.Track{Name: "Wasted Years"}
	artist := domain.Artist{Name: "Iron Maiden"}
	uri, found, err := a.searchTrack(context.Background(), "token", track, artist, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !found {
		t.Fatal("expected track to be found")
	}
	if uri != "spotify:track:correct-uri" {
		t.Errorf("expected uri spotify:track:correct-uri, got %s", uri)
	}
}

func TestSearchTrack_MatchesVerboseRemasterIfNoDirectMatch(t *testing.T) {
	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(makeSearchResponse([][3]string{
			{"Wasted Years - Harris Remix", "spotify:track:uri2", "Iron Maiden"},
			{"Wasted Years - Live in Lisbon", "spotify:track:uri3", "Iron Maiden"},
			{"Wasted Years - 2015 Remaster", "spotify:track:correct-uri", "Iron Maiden"},
			{"Wasted Years - Demo 1678", "spotify:track:uri4", "Iron Maiden"}}))
	}))
	defer apiSrv.Close()

	a := newTestAccountsAdapter(t, "")
	a.apiBaseURL = apiSrv.URL

	track := domain.Track{Name: "Wasted Years"}
	artist := domain.Artist{Name: "Iron Maiden"}
	uri, found, err := a.searchTrack(context.Background(), "token", track, artist, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !found {
		t.Fatal("expected track to be found")
	}
	if uri != "spotify:track:correct-uri" {
		t.Errorf("expected uri spotify:track:correct-uri, got %s", uri)
	}
}

func TestSearchTrack_MatchesVerboseLiveIfNoDirectMatchOrRemaster(t *testing.T) {
	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(makeSearchResponse([][3]string{
			{"Wasted Years - Live in Lisbon", "spotify:track:correct-uri", "Iron Maiden"},
			{"Wasted Years - Harris Remix", "spotify:track:uri3", "Iron Maiden"},
			{"Wasted Years - Demo 1678", "spotify:track:uri4", "Iron Maiden"}}))
	}))
	defer apiSrv.Close()

	a := newTestAccountsAdapter(t, "")
	a.apiBaseURL = apiSrv.URL

	track := domain.Track{Name: "Wasted Years"}
	artist := domain.Artist{Name: "Iron Maiden"}
	uri, found, err := a.searchTrack(context.Background(), "token", track, artist, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !found {
		t.Fatal("expected track to be found")
	}
	if uri != "spotify:track:correct-uri" {
		t.Errorf("expected uri spotify:track:correct-uri, got %s", uri)
	}
}

func TestSearchTrack_MatchesVerboseRemixIfNoDirectMatchRemasterOrLive(t *testing.T) {
	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(makeSearchResponse([][3]string{
			{"Wasted Years - Demo 1678", "spotify:track:uri4", "Iron Maiden"},
			{"Wasted Years - Harris Remix", "spotify:track:correct-uri", "Iron Maiden"}}))
	}))
	defer apiSrv.Close()

	a := newTestAccountsAdapter(t, "")
	a.apiBaseURL = apiSrv.URL

	track := domain.Track{Name: "Wasted Years"}
	artist := domain.Artist{Name: "Iron Maiden"}
	uri, found, err := a.searchTrack(context.Background(), "token", track, artist, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !found {
		t.Fatal("expected track to be found")
	}
	if uri != "spotify:track:correct-uri" {
		t.Errorf("expected uri spotify:track:correct-uri, got %s", uri)
	}
}

func TestSearchTrack_MatchesVerboseSomethingElseIfNoDirectMatchRemasterRemixOrLive(t *testing.T) {
	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(makeSearchResponse([][3]string{{"Wasted Years - Demo 1678", "spotify:track:correct-uri", "Iron Maiden"}}))
	}))
	defer apiSrv.Close()

	a := newTestAccountsAdapter(t, "")
	a.apiBaseURL = apiSrv.URL

	track := domain.Track{Name: "Wasted Years"}
	artist := domain.Artist{Name: "Iron Maiden"}
	uri, found, err := a.searchTrack(context.Background(), "token", track, artist, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !found {
		t.Fatal("expected track to be found")
	}
	if uri != "spotify:track:correct-uri" {
		t.Errorf("expected uri spotify:track:correct-uri, got %s", uri)
	}
}

func TestSearchTrack_CoverFallback_FindsOriginal(t *testing.T) {
	var callCount atomic.Int32

	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("q")
		w.Header().Set("Content-Type", "application/json")
		callCount.Add(1)
		if strings.Contains(q, "Metallica") {
			json.NewEncoder(w).Encode(makeSearchResponse([][3]string{{"Whiplash", "spotify:track:whiplash", "metallica-id"}}))
		} else {
			json.NewEncoder(w).Encode(spotifySearchResponse{})
		}
	}))
	defer apiSrv.Close()

	a := newTestAccountsAdapter(t, "")
	a.apiBaseURL = apiSrv.URL

	track := domain.Track{Name: "Whiplash", CoveredArtistName: "Metallica"}
	artist := domain.Artist{Name: "Hellripper"}
	uri, found, err := a.searchTrack(context.Background(), "token", track, artist, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !found {
		t.Fatal("expected track to be found via cover fallback")
	}
	if uri != "spotify:track:whiplash" {
		t.Errorf("unexpected URI: %s", uri)
	}
	if callCount.Load() != 2 {
		t.Errorf("expected 2 requests (original + cover fallback), got %d", callCount.Load())
	}
}

func TestSearchTrack_CoverFallback_DisabledDoesNotRetry(t *testing.T) {
	var callCount atomic.Int32

	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount.Add(1)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(spotifySearchResponse{})
	}))
	defer apiSrv.Close()

	a := newTestAccountsAdapter(t, "")
	a.apiBaseURL = apiSrv.URL

	track := domain.Track{Name: "Whiplash", CoveredArtistName: "Metallica"}
	artist := domain.Artist{Name: "Hellripper"}
	_, found, err := a.searchTrack(context.Background(), "token", track, artist, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if found {
		t.Fatal("expected track not to be found when cover fallback disabled")
	}
	if callCount.Load() != 1 {
		t.Errorf("expected 1 request (no cover retry), got %d", callCount.Load())
	}
}

func TestSearchTrack_CoverFallback_EmptyCoveredArtistDoesNotRetry(t *testing.T) {
	var callCount atomic.Int32

	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount.Add(1)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(spotifySearchResponse{})
	}))
	defer apiSrv.Close()

	a := newTestAccountsAdapter(t, "")
	a.apiBaseURL = apiSrv.URL

	track := domain.Track{Name: "Some Song", CoveredArtistName: ""}
	artist := domain.Artist{Name: "Artist"}
	_, found, err := a.searchTrack(context.Background(), "token", track, artist, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if found {
		t.Fatal("expected track not to be found when CoveredArtistName is empty")
	}
	if callCount.Load() != 1 {
		t.Errorf("expected 1 request (no cover retry when CoveredArtistName empty), got %d", callCount.Load())
	}
}

func makeCoverServer(t *testing.T, coverArtist string, coverItems [][3]string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("q")
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(q, coverArtist) {
			json.NewEncoder(w).Encode(makeSearchResponse(coverItems))
		} else {
			json.NewEncoder(w).Encode(spotifySearchResponse{})
		}
	}))
}

func TestSearchTrack_CoverFallback_MatchesDirectMatchOverVerboseNames(t *testing.T) {
	apiSrv := makeCoverServer(t, "Metallica", [][3]string{
		{"Whiplash - Remastered", "spotify:track:uri1", "metallica-id"},
		{"Whiplash", "spotify:track:correct-uri", "metallica-id"},
	})
	defer apiSrv.Close()

	a := newTestAccountsAdapter(t, "")
	a.apiBaseURL = apiSrv.URL

	track := domain.Track{Name: "Whiplash", CoveredArtistName: "Metallica"}
	artist := domain.Artist{Name: "Hellripper"}
	uri, found, err := a.searchTrack(context.Background(), "token", track, artist, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !found {
		t.Fatal("expected track to be found")
	}
	if uri != "spotify:track:correct-uri" {
		t.Errorf("expected spotify:track:correct-uri, got %s", uri)
	}
}

func TestSearchTrack_CoverFallback_MatchesVerboseRemasterIfNoDirectMatch(t *testing.T) {
	apiSrv := makeCoverServer(t, "Metallica", [][3]string{
		{"Whiplash - Live Shit", "spotify:track:uri2", "metallica-id"},
		{"Whiplash - 2021 Remaster", "spotify:track:correct-uri", "metallica-id"},
		{"Whiplash - Demo", "spotify:track:uri4", "metallica-id"},
	})
	defer apiSrv.Close()

	a := newTestAccountsAdapter(t, "")
	a.apiBaseURL = apiSrv.URL

	track := domain.Track{Name: "Whiplash", CoveredArtistName: "Metallica"}
	artist := domain.Artist{Name: "Hellripper"}
	uri, found, err := a.searchTrack(context.Background(), "token", track, artist, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !found {
		t.Fatal("expected track to be found")
	}
	if uri != "spotify:track:correct-uri" {
		t.Errorf("expected spotify:track:correct-uri, got %s", uri)
	}
}

func TestSearchTrack_CoverFallback_MatchesVerboseLiveIfNoDirectMatchOrRemaster(t *testing.T) {
	apiSrv := makeCoverServer(t, "Metallica", [][3]string{
		{"Whiplash - Live Shit", "spotify:track:correct-uri", "metallica-id"},
		{"Whiplash - Kirk Remix", "spotify:track:uri3", "metallica-id"},
		{"Whiplash - Demo", "spotify:track:uri4", "metallica-id"},
	})
	defer apiSrv.Close()

	a := newTestAccountsAdapter(t, "")
	a.apiBaseURL = apiSrv.URL

	track := domain.Track{Name: "Whiplash", CoveredArtistName: "Metallica"}
	artist := domain.Artist{Name: "Hellripper"}
	uri, found, err := a.searchTrack(context.Background(), "token", track, artist, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !found {
		t.Fatal("expected track to be found")
	}
	if uri != "spotify:track:correct-uri" {
		t.Errorf("expected spotify:track:correct-uri, got %s", uri)
	}
}

func TestSearchTrack_CoverFallback_MatchesVerboseRemixIfNoDirectMatchRemasterOrLive(t *testing.T) {
	apiSrv := makeCoverServer(t, "Metallica", [][3]string{
		{"Whiplash - Demo", "spotify:track:uri4", "metallica-id"},
		{"Whiplash - Kirk Remix", "spotify:track:correct-uri", "metallica-id"},
	})
	defer apiSrv.Close()

	a := newTestAccountsAdapter(t, "")
	a.apiBaseURL = apiSrv.URL

	track := domain.Track{Name: "Whiplash", CoveredArtistName: "Metallica"}
	artist := domain.Artist{Name: "Hellripper"}
	uri, found, err := a.searchTrack(context.Background(), "token", track, artist, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !found {
		t.Fatal("expected track to be found")
	}
	if uri != "spotify:track:correct-uri" {
		t.Errorf("expected spotify:track:correct-uri, got %s", uri)
	}
}

func TestSearchTrack_CoverFallback_MatchesVerboseSomethingElseIfNoDirectMatchRemasterRemixOrLive(t *testing.T) {
	apiSrv := makeCoverServer(t, "Metallica", [][3]string{
		{"Whiplash - Demo", "spotify:track:correct-uri", "metallica-id"},
	})
	defer apiSrv.Close()

	a := newTestAccountsAdapter(t, "")
	a.apiBaseURL = apiSrv.URL

	track := domain.Track{Name: "Whiplash", CoveredArtistName: "Metallica"}
	artist := domain.Artist{Name: "Hellripper"}
	uri, found, err := a.searchTrack(context.Background(), "token", track, artist, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !found {
		t.Fatal("expected track to be found")
	}
	if uri != "spotify:track:correct-uri" {
		t.Errorf("expected spotify:track:correct-uri, got %s", uri)
	}
}

func TestSearchTrack_CoverFallback_429RetryPolicy(t *testing.T) {
	var callCount atomic.Int32
	var sleptFor atomic.Int64

	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := callCount.Add(1)
		q := r.URL.Query().Get("q")
		if !strings.Contains(q, "Metallica") {
			// First search (original artist): no results.
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(spotifySearchResponse{})
			return
		}
		// Cover fallback: first attempt is 429, second succeeds.
		if n == 2 {
			w.Header().Set("Retry-After", "2")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(makeSearchResponse([][3]string{{"Whiplash", "spotify:track:whiplash", "metallica-id"}}))
	}))
	defer apiSrv.Close()

	a := newTestAccountsAdapter(t, "")
	a.apiBaseURL = apiSrv.URL
	a.sleepFn = func(d time.Duration) { sleptFor.Add(int64(d)) }

	track := domain.Track{Name: "Whiplash", CoveredArtistName: "Metallica"}
	artist := domain.Artist{Name: "Hellripper"}
	uri, found, err := a.searchTrack(context.Background(), "token", track, artist, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !found {
		t.Fatal("expected track to be found after cover fallback retry")
	}
	if uri != "spotify:track:whiplash" {
		t.Errorf("unexpected URI: %s", uri)
	}
	if sleptFor.Load() != int64(2*time.Second) {
		t.Errorf("expected sleep of 2s for cover fallback 429, got %v", time.Duration(sleptFor.Load()))
	}
}
