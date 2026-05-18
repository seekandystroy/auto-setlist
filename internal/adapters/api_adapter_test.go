package adapters

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type mockSetlistService struct {
	playlistID string
	err        error
}

func (m *mockSetlistService) SetlistToPlaylist(artist string) (string, error) {
	return m.playlistID, m.err
}

func post(handler http.Handler, body string) *httptest.ResponseRecorder {
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
