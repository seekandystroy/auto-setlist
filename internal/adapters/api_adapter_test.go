package adapters

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/seekandystroy/auto-setlist/internal/core/domain"
)

type mockSetlistService struct {
	playlistID                     string
	err                            error
	receivedIncludeCovers          bool
	receivedAllSongsFromLatestTour bool
	receivedArtist                 string
	receivedURL                    string
}

func (m *mockSetlistService) SetlistToPlaylist(_ context.Context, artist string, includeCovers, tourPlaylist bool) (string, error) {
	m.receivedIncludeCovers = includeCovers
	m.receivedAllSongsFromLatestTour = tourPlaylist
	return m.playlistID, m.err
}

func (m *mockSetlistService) SetlistToPlaylistAuthed(_ context.Context, artist, token string, includeCovers, tourPlaylist bool) (string, error) {
	m.receivedArtist = artist
	m.receivedIncludeCovers = includeCovers
	m.receivedAllSongsFromLatestTour = tourPlaylist
	return m.playlistID, m.err
}

func (m *mockSetlistService) SetlistURLToPlaylist(_ context.Context, setlistURL string, includeCovers bool) (string, error) {
	m.receivedURL = setlistURL
	m.receivedIncludeCovers = includeCovers
	return m.playlistID, m.err
}

func (m *mockSetlistService) SetlistURLToPlaylistAuthed(_ context.Context, setlistURL, token string, includeCovers bool) (string, error) {
	m.receivedURL = setlistURL
	m.receivedIncludeCovers = includeCovers
	return m.playlistID, m.err
}

func errorOf(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()
	var body map[string]string
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("could not decode response: %v", err)
	}
	return body["error"]
}

func post(handler http.Handler, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(http.MethodPost, "/setlistjob", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Autosetlist-Spotify-Token", "test-token")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	return w
}

func postNoToken(handler http.Handler, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(http.MethodPost, "/setlistjob", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	return w
}

func TestSetlistJob_HappyPath(t *testing.T) {
	handler := NewAPIAdapter(&mockSetlistService{playlistID: "abc123"})
	w := post(handler, `{"artist":"Radiohead"}`)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp setlistJobResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("could not decode response: %v", err)
	}
	if resp.PlaylistURL != "https://open.spotify.com/playlist/abc123" {
		t.Errorf("unexpected playlist_url: %q", resp.PlaylistURL)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("unexpected Content-Type: %q", ct)
	}
}

func TestSetlistJob_MissingArtist(t *testing.T) {
	handler := NewAPIAdapter(&mockSetlistService{playlistID: "abc123"})
	w := post(handler, `{}`)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestSetlistJob_EmptyArtist(t *testing.T) {
	handler := NewAPIAdapter(&mockSetlistService{playlistID: "abc123"})
	w := post(handler, `{"artist":""}`)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestSetlistJob_WhitespaceOnlyArtist(t *testing.T) {
	handler := NewAPIAdapter(&mockSetlistService{playlistID: "abc123"})
	w := post(handler, `{"artist":"   "}`)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestSetlistJob_ArtistTooLong(t *testing.T) {
	handler := NewAPIAdapter(&mockSetlistService{playlistID: "abc123"})
	w := post(handler, `{"artist":"`+strings.Repeat("a", 101)+`"}`)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestSetlistJob_ArtistAtMaxLength(t *testing.T) {
	handler := NewAPIAdapter(&mockSetlistService{playlistID: "abc123"})
	w := post(handler, `{"artist":"`+strings.Repeat("a", 100)+`"}`)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for 100-char artist, got %d", w.Code)
	}
}

func TestSetlistJob_InvalidJSON(t *testing.T) {
	handler := NewAPIAdapter(&mockSetlistService{playlistID: "abc123"})
	w := post(handler, `not json`)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestSetlistJob_ServiceError(t *testing.T) {
	handler := NewAPIAdapter(&mockSetlistService{err: errors.New("artist not found")})
	w := post(handler, `{"artist":"Ghost"}`)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

func TestSetlistJob_WrongMethod(t *testing.T) {
	handler := NewAPIAdapter(&mockSetlistService{playlistID: "abc123"})
	r := httptest.NewRequest(http.MethodGet, "/setlistjob", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

func TestSetlistJob_UnknownPath(t *testing.T) {
	handler := NewAPIAdapter(&mockSetlistService{playlistID: "abc123"})
	r := httptest.NewRequest(http.MethodPost, "/unknown", strings.NewReader(`{"artist":"x"}`))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestSetlistJob_MissingSpotifyToken(t *testing.T) {
	handler := NewAPIAdapter(&mockSetlistService{playlistID: "abc123"})
	w := postNoToken(handler, `{"artist":"Radiohead"}`)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
	var body map[string]string
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("could not decode response: %v", err)
	}
	if !strings.Contains(body["error"], "Autosetlist-Spotify-Token") {
		t.Errorf("expected error to mention header name, got: %q", body["error"])
	}
}

func TestSetlistJob_IncludeCoversDefaultsFalse(t *testing.T) {
	svc := &mockSetlistService{playlistID: "abc123"}
	handler := NewAPIAdapter(svc)
	post(handler, `{"artist":"Hellripper"}`)

	if svc.receivedIncludeCovers {
		t.Error("expected includeCovers to default to false when omitted, got true")
	}
}

func TestSetlistJob_IncludeCoversTrue(t *testing.T) {
	svc := &mockSetlistService{playlistID: "abc123"}
	handler := NewAPIAdapter(svc)
	post(handler, `{"artist":"Hellripper","include_covers":true}`)

	if !svc.receivedIncludeCovers {
		t.Error("expected includeCovers=true to be forwarded to service, got false")
	}
}

func TestSetlistJob_AllSongsFromLatestTourDefaultsFalse(t *testing.T) {
	svc := &mockSetlistService{playlistID: "abc123"}
	handler := NewAPIAdapter(svc)
	post(handler, `{"artist":"Hellripper"}`)

	if svc.receivedAllSongsFromLatestTour {
		t.Error("expected tourPlaylist to default to false when omitted, got true")
	}
}

func TestSetlistJob_AllSongsFromLatestTourTrue(t *testing.T) {
	svc := &mockSetlistService{playlistID: "abc123"}
	handler := NewAPIAdapter(svc)
	post(handler, `{"artist":"Hellripper","tour_playlist":true}`)

	if !svc.receivedAllSongsFromLatestTour {
		t.Error("expected tourPlaylist=true to be forwarded to service, got false")
	}
}

const setlistLink = "https://www.setlist.fm/setlist/the-beatles/1964/hollywood-bowl-hollywood-ca-63de4613.html"

func TestSetlistJob_URLHappyPath(t *testing.T) {
	svc := &mockSetlistService{playlistID: "abc123"}
	handler := NewAPIAdapter(svc)
	w := post(handler, `{"url":"  `+setlistLink+`  ","include_covers":true}`)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp setlistJobResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("could not decode response: %v", err)
	}
	if resp.PlaylistURL != "https://open.spotify.com/playlist/abc123" {
		t.Errorf("unexpected playlist_url: %q", resp.PlaylistURL)
	}
	if svc.receivedURL != setlistLink {
		t.Errorf("expected trimmed url %q forwarded to service, got %q", setlistLink, svc.receivedURL)
	}
	if svc.receivedArtist != "" {
		t.Errorf("expected the artist flow not to run, got artist %q", svc.receivedArtist)
	}
	if !svc.receivedIncludeCovers {
		t.Error("expected includeCovers=true to be forwarded to service, got false")
	}
}

func TestSetlistJob_ArtistAndURL(t *testing.T) {
	svc := &mockSetlistService{playlistID: "abc123"}
	w := post(NewAPIAdapter(svc), `{"artist":"The Beatles","url":"`+setlistLink+`"}`)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
	if got := errorOf(t, w); got != "send either artist or url, not both" {
		t.Errorf("unexpected error: %q", got)
	}
	if svc.receivedURL != "" || svc.receivedArtist != "" {
		t.Error("expected the service not to be called")
	}
}

func TestSetlistJob_NeitherArtistNorURL(t *testing.T) {
	w := post(NewAPIAdapter(&mockSetlistService{}), `{"artist":" ","url":" "}`)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
	if got := errorOf(t, w); got != "artist or url is required" {
		t.Errorf("unexpected error: %q", got)
	}
}

func TestSetlistJob_URLTooLong(t *testing.T) {
	svc := &mockSetlistService{}
	w := post(NewAPIAdapter(svc), `{"url":"https://www.setlist.fm/`+strings.Repeat("a", 500)+`"}`)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
	if got := errorOf(t, w); got != "url must be 500 characters or fewer" {
		t.Errorf("unexpected error: %q", got)
	}
	if svc.receivedURL != "" {
		t.Error("expected the service not to be called")
	}
}

func TestSetlistJob_InvalidSetlistURL(t *testing.T) {
	handler := NewAPIAdapter(&mockSetlistService{err: domain.ErrInvalidSetlistURL})
	w := post(handler, `{"url":"https://example.com/setlist"}`)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
	if got := errorOf(t, w); got != "only setlist.fm setlist links are supported" {
		t.Errorf("unexpected error: %q", got)
	}
}

func TestSetlistJob_URLServiceError(t *testing.T) {
	handler := NewAPIAdapter(&mockSetlistService{err: errors.New("Setlist not found")})
	w := post(handler, `{"url":"`+setlistLink+`"}`)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
	if got := errorOf(t, w); got != "Setlist not found" {
		t.Errorf("unexpected error: %q", got)
	}
}
