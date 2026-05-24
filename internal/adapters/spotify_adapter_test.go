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

func makeSearchResponse(name, uri, artistID string) spotifySearchResponse {
	return spotifySearchResponse{Tracks: struct {
		Items []spotifySearchTrack `json:"items"`
	}{Items: []spotifySearchTrack{
		{Name: name, URI: uri, Artists: []spotifySearchArtist{{ID: artistID}}},
	}}}
}

func TestGetSetlistTracks_ReturnsURIs(t *testing.T) {
	setlist := domain.Setlist{
		Artist: domain.Artist{MBID: "m1", Name: "Pitbull", SpotifyID: "artist-id"},
		Tracks: []string{"Give Me Everything", "Timber"},
	}

	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("q")
		var resp spotifySearchResponse
		if strings.Contains(q, "Give+Me+Everything") || strings.Contains(q, "Give Me Everything") {
			resp = makeSearchResponse("Give Me Everything", "spotify:track:uri1", "artist-id")
		} else {
			resp = makeSearchResponse("Timber", "spotify:track:uri2", "artist-id")
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer apiSrv.Close()

	a := newTestAccountsAdapter(t, "")
	a.apiBaseURL = apiSrv.URL

	uris, err := a.GetSetlistTracks(context.Background(), "test-token", setlist)
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
		Artist: domain.Artist{SpotifyID: "artist-id"},
		Tracks: []string{"Known Song", "Unknown Song"},
	}

	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("q")
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(q, "Known") {
			json.NewEncoder(w).Encode(makeSearchResponse("Known Song", "spotify:track:uri1", "artist-id"))
		} else {
			json.NewEncoder(w).Encode(spotifySearchResponse{})
		}
	}))
	defer apiSrv.Close()

	a := newTestAccountsAdapter(t, "")
	a.apiBaseURL = apiSrv.URL

	uris, err := a.GetSetlistTracks(context.Background(), "test-token", setlist)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(uris) != 1 {
		t.Errorf("expected 1 URI (unknown track skipped), got %d", len(uris))
	}
}

func TestGetSetlistTracks_SkipsWrongArtist(t *testing.T) {
	setlist := domain.Setlist{
		Artist: domain.Artist{SpotifyID: "correct-artist"},
		Tracks: []string{"Song"},
	}

	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Returns a track with the right name but wrong artist ID.
		json.NewEncoder(w).Encode(makeSearchResponse("Song", "spotify:track:uri1", "wrong-artist"))
	}))
	defer apiSrv.Close()

	a := newTestAccountsAdapter(t, "")
	a.apiBaseURL = apiSrv.URL

	uris, err := a.GetSetlistTracks(context.Background(), "test-token", setlist)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(uris) != 0 {
		t.Errorf("expected 0 URIs (wrong artist skipped), got %d", len(uris))
	}
}

func TestGetSetlistTracks_NonOKStatusReturnsError(t *testing.T) {
	setlist := domain.Setlist{
		Artist: domain.Artist{SpotifyID: "artist-id"},
		Tracks: []string{"Song"},
	}

	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer apiSrv.Close()

	a := newTestAccountsAdapter(t, "")
	a.apiBaseURL = apiSrv.URL

	_, err := a.GetSetlistTracks(context.Background(), "test-token", setlist)
	if err == nil {
		t.Fatal("expected error for non-OK status, got nil")
	}
	if !strings.Contains(err.Error(), "unexpected status") {
		t.Errorf("expected 'unexpected status' in error, got %q", err.Error())
	}
}

func TestGetSetlistTracks_403NotRegistered(t *testing.T) {
	setlist := domain.Setlist{
		Artist: domain.Artist{SpotifyID: "artist-id"},
		Tracks: []string{"Song"},
	}

	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte("The user is not registered for this application. Please check your settings on https://developer.spotify.com/dashboard."))
	}))
	defer apiSrv.Close()

	a := newTestAccountsAdapter(t, "")
	a.apiBaseURL = apiSrv.URL

	_, err := a.GetSetlistTracks(context.Background(), "test-token", setlist)
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

	a.GetSetlistTracks(context.Background(), "test-access-token", domain.Setlist{Tracks: []string{"Song"}})

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

	_, err := a.CreatePlaylist(context.Background(), "test-token", domain.Setlist{}, nil)
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

	id, err := a.CreatePlaylist(context.Background(), "test-token", setlist, nil)
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

	_, err := a.CreatePlaylist(context.Background(), "test-token", setlist, uris)
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

	_, err := a.CreatePlaylist(context.Background(), "test-token", setlist, []string{"spotify:track:a"})
	if err == nil {
		t.Fatal("expected error from add items, got nil")
	}
	if !strings.Contains(err.Error(), "unexpected status") {
		t.Errorf("expected 'unexpected status' in error, got %q", err.Error())
	}
}

func TestCreatePlaylist_PlaylistName(t *testing.T) {
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

	a.CreatePlaylist(context.Background(), "test-token", setlist, nil)

	wantName := "Pitbull setlist by auto-setlist"
	if gotBody.Name != wantName {
		t.Errorf("expected name %q, got %q", wantName, gotBody.Name)
	}
	if gotBody.Description != "auto-generated" {
		t.Errorf("expected description %q, got %q", "auto-generated", gotBody.Description)
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

	a.CreatePlaylist(context.Background(), "test-access-token", domain.Setlist{}, nil)

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

	_, err := a.CreatePlaylist(context.Background(), "test-token", domain.Setlist{}, nil)
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

	_, err := a.CreatePlaylist(context.Background(), "test-token", domain.Setlist{}, nil)
	if err == nil {
		t.Fatal("expected decode error, got nil")
	}
	if !strings.Contains(err.Error(), "decoding create playlist response") {
		t.Errorf("expected 'decoding create playlist response' in error, got %q", err.Error())
	}
}

func TestGetSetlistTracks_401InvalidToken(t *testing.T) {
	setlist := domain.Setlist{
		Artist: domain.Artist{SpotifyID: "artist-id"},
		Tracks: []string{"Song"},
	}

	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer apiSrv.Close()

	a := newTestAccountsAdapter(t, "")
	a.apiBaseURL = apiSrv.URL

	_, err := a.GetSetlistTracks(context.Background(), "expired-token", setlist)
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

	_, err := a.CreatePlaylist(context.Background(), "expired-token", domain.Setlist{}, nil)
	if err == nil {
		t.Fatal("expected error for 401, got nil")
	}
	if !strings.Contains(err.Error(), "401") {
		t.Errorf("expected error to mention 401, got %q", err.Error())
	}
}
