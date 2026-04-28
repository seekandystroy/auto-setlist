package adapters

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/seekandystroy/auto-setlist/internal/core/domain"
)

func newTestAdapter(serverURL string) *setlistFMAdapter {
	return &setlistFMAdapter{
		apiKey:     "test-key",
		httpClient: &http.Client{},
		baseURL:    serverURL,
	}
}

func TestSearchArtists_HappyPath(t *testing.T) {
	expected := domain.ArtistSearchResult{
		Type:         "artist",
		ItemsPerPage: 20,
		Page:         1,
		Total:        1,
		Artists: []domain.Artist{
			{MBID: "abc123", Name: "Sprout", SortName: "Sprout", URL: "https://example.com"},
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(expected)
	}))
	defer srv.Close()

	adapter := newTestAdapter(srv.URL)
	result, err := adapter.SearchArtists("Sprout")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Artists) != 1 {
		t.Fatalf("expected 1 artist, got %d", len(result.Artists))
	}
	if result.Artists[0].MBID != "abc123" {
		t.Errorf("expected MBID %q, got %q", "abc123", result.Artists[0].MBID)
	}
}

func TestSearchArtists_NonOKStatus(t *testing.T) {
	for _, status := range []int{http.StatusUnauthorized, http.StatusNotFound, http.StatusInternalServerError} {
		status := status
		t.Run(http.StatusText(status), func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(status)
			}))
			defer srv.Close()

			adapter := newTestAdapter(srv.URL)
			_, err := adapter.SearchArtists("Sprout")
			if err == nil {
				t.Fatalf("expected error for status %d, got nil", status)
			}
			if !strings.Contains(err.Error(), "unexpected status") {
				t.Errorf("expected 'unexpected status' in error, got %q", err.Error())
			}
		})
	}
}

func TestSearchArtists_NetworkError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	srv.Close() // close immediately so the request fails

	adapter := newTestAdapter(srv.URL)
	_, err := adapter.SearchArtists("Sprout")
	if err == nil {
		t.Fatal("expected network error, got nil")
	}
	if !strings.Contains(err.Error(), "executing request") {
		t.Errorf("expected 'executing request' in error, got %q", err.Error())
	}
}

func TestSearchArtists_MalformedJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("not json {{{"))
	}))
	defer srv.Close()

	adapter := newTestAdapter(srv.URL)
	_, err := adapter.SearchArtists("Sprout")
	if err == nil {
		t.Fatal("expected decode error, got nil")
	}
	if !strings.Contains(err.Error(), "decoding response") {
		t.Errorf("expected 'decoding response' in error, got %q", err.Error())
	}
}

func TestSearchArtists_SendsCorrectHeaders(t *testing.T) {
	var gotAPIKey, gotAccept string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAPIKey = r.Header.Get("x-api-key")
		gotAccept = r.Header.Get("Accept")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(domain.ArtistSearchResult{})
	}))
	defer srv.Close()

	adapter := newTestAdapter(srv.URL)
	adapter.apiKey = "my-secret-key"
	adapter.SearchArtists("Sprout")

	if gotAPIKey != "my-secret-key" {
		t.Errorf("expected x-api-key %q, got %q", "my-secret-key", gotAPIKey)
	}
	if gotAccept != "application/json" {
		t.Errorf("expected Accept %q, got %q", "application/json", gotAccept)
	}
}
