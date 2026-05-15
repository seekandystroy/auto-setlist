package adapters

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/seekandystroy/auto-setlist/internal/core/domain"
)

func newTestAdapter(serverURL string) *setlistfmAdapter {
	return &setlistfmAdapter{
		apiKey:     "test-key",
		httpClient: &http.Client{},
		baseURL:    serverURL,
		sleepFn:    func(time.Duration) {},
	}
}

func TestSearchArtists_HappyPath(t *testing.T) {
	expected := setlistfmSearchResult{
		Type:         "artist",
		ItemsPerPage: 20,
		Page:         1,
		Total:        1,
		Artists: []setlistfmArtist{
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
	if len(result) != 1 {
		t.Fatalf("expected 1 artist, got %d", len(result))
	}
	if result[0].MBID != "abc123" {
		t.Errorf("expected MBID %q, got %q", "abc123", result[0].MBID)
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

func TestGetSetlists_HappyPath(t *testing.T) {
	response := setlistfmSetlistsResponse{
		Setlists: []setlistfmSetlist{
			{Sets: setlistfmSets{Set: []setlistfmSet{
				{Songs: []setlistfmSong{{Name: "Song A"}, {Name: "Song B"}}},
				{Songs: []setlistfmSong{{Name: "Encore Song"}}},
			}}},
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer srv.Close()

	adapter := newTestAdapter(srv.URL)
	result, err := adapter.GetSetlists(domain.Artist{MBID: "abc123", Name: "Sprout"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 setlist, got %d", len(result))
	}
	if len(result[0].Tracks) != 3 {
		t.Errorf("expected 3 tracks, got %d", len(result[0].Tracks))
	}
	if result[0].Tracks[0] != "Song A" {
		t.Errorf("expected first track %q, got %q", "Song A", result[0].Tracks[0])
	}
	if result[0].Artist.MBID != "abc123" {
		t.Errorf("expected artist MBID %q, got %q", "abc123", result[0].Artist.MBID)
	}
}

func TestGetSetlists_SkipsUnnamedSongs(t *testing.T) {
	response := setlistfmSetlistsResponse{
		Setlists: []setlistfmSetlist{
			{Sets: setlistfmSets{Set: []setlistfmSet{
				{Songs: []setlistfmSong{{Name: "Real Song"}, {Name: ""}}},
			}}},
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer srv.Close()

	adapter := newTestAdapter(srv.URL)
	result, err := adapter.GetSetlists(domain.Artist{MBID: "abc123"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result[0].Tracks) != 1 {
		t.Errorf("expected 1 track (unnamed skipped), got %d", len(result[0].Tracks))
	}
}

func TestGetSetlists_NonOKStatus(t *testing.T) {
	for _, status := range []int{http.StatusUnauthorized, http.StatusNotFound, http.StatusInternalServerError} {
		status := status
		t.Run(http.StatusText(status), func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(status)
			}))
			defer srv.Close()

			adapter := newTestAdapter(srv.URL)
			_, err := adapter.GetSetlists(domain.Artist{MBID: "abc123"})
			if err == nil {
				t.Fatalf("expected error for status %d, got nil", status)
			}
			if !strings.Contains(err.Error(), "unexpected status") {
				t.Errorf("expected 'unexpected status' in error, got %q", err.Error())
			}
		})
	}
}

func TestGetSetlists_NetworkError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	srv.Close()

	adapter := newTestAdapter(srv.URL)
	_, err := adapter.GetSetlists(domain.Artist{MBID: "abc123"})
	if err == nil {
		t.Fatal("expected network error, got nil")
	}
	if !strings.Contains(err.Error(), "executing request") {
		t.Errorf("expected 'executing request' in error, got %q", err.Error())
	}
}

func TestGetSetlists_MalformedJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("not json {{{"))
	}))
	defer srv.Close()

	adapter := newTestAdapter(srv.URL)
	_, err := adapter.GetSetlists(domain.Artist{MBID: "abc123"})
	if err == nil {
		t.Fatal("expected decode error, got nil")
	}
	if !strings.Contains(err.Error(), "decoding response") {
		t.Errorf("expected 'decoding response' in error, got %q", err.Error())
	}
}

func TestGetSetlists_RetriesOnFailureThenSucceeds(t *testing.T) {
	response := setlistfmSetlistsResponse{
		Setlists: []setlistfmSetlist{
			{Sets: setlistfmSets{Set: []setlistfmSet{
				{Songs: []setlistfmSong{{Name: "Song A"}}},
			}}},
		},
	}
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts < 3 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer srv.Close()

	adapter := newTestAdapter(srv.URL)
	result, err := adapter.GetSetlists(domain.Artist{MBID: "abc123"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if attempts != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts)
	}
	if len(result[0].Tracks) != 1 || result[0].Tracks[0] != "Song A" {
		t.Errorf("unexpected result: %+v", result)
	}
}

func TestGetSetlists_ExhaustsAllRetries(t *testing.T) {
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	adapter := newTestAdapter(srv.URL)
	_, err := adapter.GetSetlists(domain.Artist{MBID: "abc123"})
	if err == nil {
		t.Fatal("expected error after exhausting retries, got nil")
	}
	if attempts != 4 {
		t.Errorf("expected 4 total attempts (1 + 3 retries), got %d", attempts)
	}
	if !strings.Contains(err.Error(), "unexpected status") {
		t.Errorf("expected 'unexpected status' in error, got %q", err.Error())
	}
}

func TestGetSetlists_SleepDurationsAreExponential(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	var slept []time.Duration
	adapter := newTestAdapter(srv.URL)
	adapter.sleepFn = func(d time.Duration) { slept = append(slept, d) }

	adapter.GetSetlists(domain.Artist{MBID: "abc123"})

	expected := []time.Duration{time.Second, 2 * time.Second, 4 * time.Second}
	if len(slept) != len(expected) {
		t.Fatalf("expected %d sleeps, got %d", len(expected), len(slept))
	}
	for i, d := range expected {
		if slept[i] != d {
			t.Errorf("sleep[%d]: expected %v, got %v", i, d, slept[i])
		}
	}
}

func TestSearchArtists_SendsCorrectHeaders(t *testing.T) {
	var gotAPIKey, gotAccept string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAPIKey = r.Header.Get("x-api-key")
		gotAccept = r.Header.Get("Accept")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(setlistfmSearchResult{})
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
