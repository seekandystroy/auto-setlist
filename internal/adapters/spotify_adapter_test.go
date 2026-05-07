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

// newTestAccountsAdapter returns a spotifyAccountsAdapter wired to the given test server URL
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
	original := &domain.SpotifyToken{
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
	validToken := &domain.SpotifyToken{
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
	if token.AccessToken != "cached-token" {
		t.Errorf("expected cached token, got: %s", token.AccessToken)
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
	expiredToken := &domain.SpotifyToken{
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
	if token.AccessToken != "refreshed-access" {
		t.Errorf("expected refreshed access token, got: %s", token.AccessToken)
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
	if token.AccessToken != "oauth-access" {
		t.Errorf("expected oauth-access, got: %s", token.AccessToken)
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
