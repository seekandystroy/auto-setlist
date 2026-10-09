package adapters

import (
	"context"
	"encoding/json"
	"errors"
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
	result, err := adapter.SearchArtists(context.Background(), "Sprout")
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

// Setlist.fm answers a search with no matches with 404.
func TestSearchArtists_NotFoundMeansNoArtists(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	adapter := newTestAdapter(srv.URL)
	result, err := adapter.SearchArtists(context.Background(), "Nonexistent Band")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected no artists, got %+v", result)
	}
}

func TestSearchArtists_NonOKStatus(t *testing.T) {
	for _, status := range []int{http.StatusUnauthorized, http.StatusInternalServerError} {
		status := status
		t.Run(http.StatusText(status), func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(status)
			}))
			defer srv.Close()

			adapter := newTestAdapter(srv.URL)
			_, err := adapter.SearchArtists(context.Background(), "Sprout")
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
	_, err := adapter.SearchArtists(context.Background(), "Sprout")
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
	_, err := adapter.SearchArtists(context.Background(), "Sprout")
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
	result, err := adapter.GetSetlists(context.Background(), domain.Artist{MBID: "abc123", Name: "Sprout"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 setlist, got %d", len(result))
	}
	if len(result[0].Tracks) != 3 {
		t.Errorf("expected 3 tracks, got %d", len(result[0].Tracks))
	}
	if result[0].Tracks[0].Name != "Song A" {
		t.Errorf("expected first track %q, got %q", "Song A", result[0].Tracks[0].Name)
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
	result, err := adapter.GetSetlists(context.Background(), domain.Artist{MBID: "abc123"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result[0].Tracks) != 1 {
		t.Errorf("expected 1 track (unnamed skipped), got %d", len(result[0].Tracks))
	}
}

// Setlist.fm answers with 404 when there are no setlists. That's an answer, not a failure: no retries.
func TestGetSetlists_NotFoundMeansNoSetlists(t *testing.T) {
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	adapter := newTestAdapter(srv.URL)
	slept := false
	adapter.sleepFn = func(time.Duration) { slept = true }

	result, err := adapter.GetSetlists(context.Background(), domain.Artist{MBID: "abc123"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected no setlists, got %+v", result)
	}
	if attempts != 1 || slept {
		t.Errorf("expected a single attempt without waiting, got %d attempts (slept: %v)", attempts, slept)
	}
}

func TestGetSetlists_NonOKStatus(t *testing.T) {
	for _, status := range []int{http.StatusUnauthorized, http.StatusInternalServerError} {
		status := status
		t.Run(http.StatusText(status), func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(status)
			}))
			defer srv.Close()

			adapter := newTestAdapter(srv.URL)
			_, err := adapter.GetSetlists(context.Background(), domain.Artist{MBID: "abc123"})
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
	_, err := adapter.GetSetlists(context.Background(), domain.Artist{MBID: "abc123"})
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
	_, err := adapter.GetSetlists(context.Background(), domain.Artist{MBID: "abc123"})
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
	result, err := adapter.GetSetlists(context.Background(), domain.Artist{MBID: "abc123"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if attempts != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts)
	}
	if len(result[0].Tracks) != 1 || result[0].Tracks[0].Name != "Song A" {
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
	_, err := adapter.GetSetlists(context.Background(), domain.Artist{MBID: "abc123"})
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

	adapter.GetSetlists(context.Background(), domain.Artist{MBID: "abc123"})

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

func TestGetSetlists_ParsesCoverSong(t *testing.T) {
	response := setlistfmSetlistsResponse{
		Setlists: []setlistfmSetlist{
			{Sets: setlistfmSets{Set: []setlistfmSet{
				{Songs: []setlistfmSong{
					{Name: "Whiplash", Cover: &setlistfmCoverArtist{Name: "Metallica"}},
					{Name: "Own Song"},
				}},
			}}},
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer srv.Close()

	adapter := newTestAdapter(srv.URL)
	result, err := adapter.GetSetlists(context.Background(), domain.Artist{MBID: "abc123"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result[0].Tracks) != 2 {
		t.Fatalf("expected 2 tracks, got %d", len(result[0].Tracks))
	}
	if result[0].Tracks[0].CoveredArtistName != "Metallica" {
		t.Errorf("expected CoveredArtistName %q, got %q", "Metallica", result[0].Tracks[0].CoveredArtistName)
	}
	if result[0].Tracks[1].CoveredArtistName != "" {
		t.Errorf("expected empty CoveredArtistName for own song, got %q", result[0].Tracks[1].CoveredArtistName)
	}
}

func TestGetSetlists_ParsesTourName(t *testing.T) {
	response := setlistfmSetlistsResponse{
		Setlists: []setlistfmSetlist{
			{
				Sets: setlistfmSets{Set: []setlistfmSet{
					{Songs: []setlistfmSong{{Name: "Song A"}}},
				}},
				Tour: &setlistfmTour{Name: "World Tour 2025"},
			},
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer srv.Close()

	adapter := newTestAdapter(srv.URL)
	result, err := adapter.GetSetlists(context.Background(), domain.Artist{MBID: "abc123"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result[0].Tour != "World Tour 2025" {
		t.Errorf("expected Tour %q, got %q", "World Tour 2025", result[0].Tour)
	}
}

func TestGetSetlists_NoTourLeavesEmpty(t *testing.T) {
	response := setlistfmSetlistsResponse{
		Setlists: []setlistfmSetlist{
			{Sets: setlistfmSets{Set: []setlistfmSet{
				{Songs: []setlistfmSong{{Name: "Song A"}}},
			}}},
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer srv.Close()

	adapter := newTestAdapter(srv.URL)
	result, err := adapter.GetSetlists(context.Background(), domain.Artist{MBID: "abc123"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result[0].Tour != "" {
		t.Errorf("expected empty Tour, got %q", result[0].Tour)
	}
}

func TestGetSetlists_CoverAbsentLeavesEmpty(t *testing.T) {
	response := setlistfmSetlistsResponse{
		Setlists: []setlistfmSetlist{
			{Sets: setlistfmSets{Set: []setlistfmSet{
				{Songs: []setlistfmSong{{Name: "Regular Song"}}},
			}}},
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer srv.Close()

	adapter := newTestAdapter(srv.URL)
	result, err := adapter.GetSetlists(context.Background(), domain.Artist{MBID: "abc123"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result[0].Tracks[0].CoveredArtistName != "" {
		t.Errorf("expected empty CoveredArtistName, got %q", result[0].Tracks[0].CoveredArtistName)
	}
}

func TestGetSetlistsForTour_HappyPath(t *testing.T) {
	var gotPath string
	response := setlistfmSetlistsResponse{
		Setlists: []setlistfmSetlist{
			{
				Sets: setlistfmSets{Set: []setlistfmSet{
					{Songs: []setlistfmSong{{Name: "Song A"}, {Name: "Song B"}}},
				}},
				Tour: &setlistfmTour{Name: "Big Tour"},
			},
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer srv.Close()

	adapter := newTestAdapter(srv.URL)
	result, err := adapter.GetSetlistsForTour(context.Background(), domain.Artist{MBID: "abc123", Name: "Sprout"}, "Big Tour")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 setlist, got %d", len(result))
	}
	if len(result[0].Tracks) != 2 {
		t.Errorf("expected 2 tracks, got %d", len(result[0].Tracks))
	}
	if result[0].Tour != "Big Tour" {
		t.Errorf("expected Tour %q, got %q", "Big Tour", result[0].Tour)
	}
	if !strings.Contains(gotPath, "artistMbid=abc123") {
		t.Errorf("expected artistMbid in query, got %q", gotPath)
	}
	if !strings.Contains(gotPath, "tourName=Big+Tour") && !strings.Contains(gotPath, "tourName=Big%20Tour") {
		t.Errorf("expected tourName in query, got %q", gotPath)
	}
}

// Setlist.fm answers with 404 when there are no setlists. That's an answer, not a failure: no retries.
func TestGetSetlistsForTour_NotFoundMeansNoSetlists(t *testing.T) {
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	adapter := newTestAdapter(srv.URL)
	slept := false
	adapter.sleepFn = func(time.Duration) { slept = true }

	result, err := adapter.GetSetlistsForTour(context.Background(), domain.Artist{MBID: "abc123"}, "Some Tour")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected no setlists, got %+v", result)
	}
	if attempts != 1 || slept {
		t.Errorf("expected a single attempt without waiting, got %d attempts (slept: %v)", attempts, slept)
	}
}

func TestGetSetlistsForTour_NonOKStatus(t *testing.T) {
	for _, status := range []int{http.StatusUnauthorized, http.StatusInternalServerError} {
		status := status
		t.Run(http.StatusText(status), func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(status)
			}))
			defer srv.Close()

			adapter := newTestAdapter(srv.URL)
			_, err := adapter.GetSetlistsForTour(context.Background(), domain.Artist{MBID: "abc123"}, "Some Tour")
			if err == nil {
				t.Fatalf("expected error for status %d, got nil", status)
			}
			if !strings.Contains(err.Error(), "unexpected status") {
				t.Errorf("expected 'unexpected status' in error, got %q", err.Error())
			}
		})
	}
}

func TestGetSetlistsForTour_NetworkError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	srv.Close()

	adapter := newTestAdapter(srv.URL)
	_, err := adapter.GetSetlistsForTour(context.Background(), domain.Artist{MBID: "abc123"}, "Some Tour")
	if err == nil {
		t.Fatal("expected network error, got nil")
	}
	if !strings.Contains(err.Error(), "executing request") {
		t.Errorf("expected 'executing request' in error, got %q", err.Error())
	}
}

func TestGetSetlistsForTour_MalformedJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("not json {{{"))
	}))
	defer srv.Close()

	adapter := newTestAdapter(srv.URL)
	_, err := adapter.GetSetlistsForTour(context.Background(), domain.Artist{MBID: "abc123"}, "Some Tour")
	if err == nil {
		t.Fatal("expected decode error, got nil")
	}
	if !strings.Contains(err.Error(), "decoding response") {
		t.Errorf("expected 'decoding response' in error, got %q", err.Error())
	}
}

func TestGetSetlistsForTour_RetriesOnFailureThenSucceeds(t *testing.T) {
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
	result, err := adapter.GetSetlistsForTour(context.Background(), domain.Artist{MBID: "abc123"}, "Some Tour")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if attempts != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts)
	}
	if len(result[0].Tracks) != 1 || result[0].Tracks[0].Name != "Song A" {
		t.Errorf("unexpected result: %+v", result)
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
	adapter.SearchArtists(context.Background(), "Sprout")

	if gotAPIKey != "my-secret-key" {
		t.Errorf("expected x-api-key %q, got %q", "my-secret-key", gotAPIKey)
	}
	if gotAccept != "application/json" {
		t.Errorf("expected Accept %q, got %q", "application/json", gotAccept)
	}
}

func TestNewSetlistfmAdapter_WithBaseURL(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(setlistfmSearchResult{})
	}))
	defer srv.Close()

	adapter := NewSetlistfmAdapter("test-key", WithSetlistfmBaseURL(srv.URL+"/rest/1.0"))
	if _, err := adapter.SearchArtists(context.Background(), "Sprout"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotPath != "/rest/1.0/search/artists" {
		t.Errorf("expected request to /rest/1.0/search/artists, got %q", gotPath)
	}
}

func TestSetlistIDFromURL_SetlistLinks(t *testing.T) {
	tests := map[string]string{
		"https://www.setlist.fm/setlist/the-beatles/1964/hollywood-bowl-hollywood-ca-63de4613.html":                    "63de4613",
		"http://www.setlist.fm/setlist/the-beatles/1964/hollywood-bowl-hollywood-ca-63de4613.html":                     "63de4613",
		"https://setlist.fm/setlist/the-beatles/1964/hollywood-bowl-hollywood-ca-63de4613.html":                        "63de4613",
		"https://de.setlist.fm/setlist/the-beatles/1964/hollywood-bowl-hollywood-ca-63de4613.html":                     "63de4613",
		"https://WWW.Setlist.FM/setlist/the-beatles/1964/hollywood-bowl-hollywood-ca-63de4613.html":                    "63de4613",
		"https://www.setlist.fm/setlist/the-beatles/1964/hollywood-bowl-hollywood-ca-63de4613.html?utm_source=x#songs": "63de4613",
	}
	adapter := newTestAdapter("")
	for link, want := range tests {
		got, err := adapter.SetlistIDFromURL(link)
		if err != nil {
			t.Errorf("%s: unexpected error: %v", link, err)
			continue
		}
		if got != want {
			t.Errorf("%s: expected ID %q, got %q", link, want, got)
		}
	}
}

func TestSetlistIDFromURL_RejectsOtherLinks(t *testing.T) {
	tests := []string{
		"",
		"Hellripper",
		"not a url %%%",
		"ftp://www.setlist.fm/setlist/the-beatles/1964/hollywood-bowl-hollywood-ca-63de4613.html",
		"https://example.com/setlist/the-beatles/1964/hollywood-bowl-hollywood-ca-63de4613.html",
		"https://notsetlist.fm/setlist/the-beatles/1964/hollywood-bowl-hollywood-ca-63de4613.html",
		"https://www.setlist.fm.example.com/setlist/the-beatles/1964/hollywood-bowl-hollywood-ca-63de4613.html",
		"https://www.setlist.fm/setlists/the-beatles-23d6a88b.html",
		"https://www.setlist.fm/venue/compaq-center-san-jose-ca-usa-6bd6ca6e.html",
		"https://www.setlist.fm/setlist/the-beatles/1964/hollywood-bowl-hollywood-ca.html",
		"https://www.setlist.fm/setlist/the-beatles/hollywood-bowl-hollywood-ca-63de4613.html",
		"https://www.setlist.fm/",
	}
	adapter := newTestAdapter("")
	for _, link := range tests {
		id, err := adapter.SetlistIDFromURL(link)
		if !errors.Is(err, domain.ErrInvalidSetlistURL) {
			t.Errorf("%q: expected ErrInvalidSetlistURL, got ID %q and error %v", link, id, err)
		}
	}
}

func TestGetSetlist_HappyPath(t *testing.T) {
	response := setlistfmSetlist{
		Artist: setlistfmArtist{MBID: "b10bbbfc", Name: "The Beatles"},
		Sets: setlistfmSets{Set: []setlistfmSet{
			{Songs: []setlistfmSong{{Name: "Twist and Shout", Cover: &setlistfmCoverArtist{Name: "The Top Notes"}}, {Name: ""}, {Name: "She Loves You"}}},
			{Songs: []setlistfmSong{{Name: "Long Tall Sally"}}},
		}},
		Tour: &setlistfmTour{Name: "North American Tour 1964"},
	}
	var gotPath, gotAPIKey, gotAccept string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAPIKey = r.Header.Get("x-api-key")
		gotAccept = r.Header.Get("Accept")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer srv.Close()

	adapter := newTestAdapter(srv.URL)
	result, err := adapter.GetSetlist(context.Background(), "63de4613")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotPath != "/setlist/63de4613" {
		t.Errorf("expected request to /setlist/63de4613, got %q", gotPath)
	}
	if gotAPIKey != "test-key" || gotAccept != "application/json" {
		t.Errorf("unexpected headers: x-api-key %q, Accept %q", gotAPIKey, gotAccept)
	}
	expected := domain.Setlist{
		Artist: domain.Artist{MBID: "b10bbbfc", Name: "The Beatles"},
		Tracks: []domain.Track{
			{Name: "Twist and Shout", CoveredArtistName: "The Top Notes"},
			{Name: "She Loves You"},
			{Name: "Long Tall Sally"},
		},
		Tour: "North American Tour 1964",
	}
	if result == nil {
		t.Fatal("expected a setlist, got nil")
	}
	if result.Artist != expected.Artist || result.Tour != expected.Tour || len(result.Tracks) != len(expected.Tracks) {
		t.Fatalf("expected %+v, got %+v", expected, *result)
	}
	for i := range expected.Tracks {
		if result.Tracks[i] != expected.Tracks[i] {
			t.Errorf("track %d: expected %+v, got %+v", i, expected.Tracks[i], result.Tracks[i])
		}
	}
}

// Setlist.fm answers with 404 for an unknown setlist ID. That's an answer, not a failure: no retries.
func TestGetSetlist_NotFoundMeansNoSetlist(t *testing.T) {
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	adapter := newTestAdapter(srv.URL)
	result, err := adapter.GetSetlist(context.Background(), "deadbeef")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Errorf("expected no setlist, got %+v", *result)
	}
	if attempts != 1 {
		t.Errorf("expected a single attempt, got %d", attempts)
	}
}

func TestGetSetlist_RetriesOnFailureThenSucceeds(t *testing.T) {
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts < 3 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(setlistfmSetlist{
			Sets: setlistfmSets{Set: []setlistfmSet{{Songs: []setlistfmSong{{Name: "Song A"}}}}},
		})
	}))
	defer srv.Close()

	adapter := newTestAdapter(srv.URL)
	result, err := adapter.GetSetlist(context.Background(), "63de4613")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if attempts != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts)
	}
	if result == nil || len(result.Tracks) != 1 || result.Tracks[0].Name != "Song A" {
		t.Errorf("unexpected result: %+v", result)
	}
}

func TestGetSetlist_ExhaustsAllRetries(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	adapter := newTestAdapter(srv.URL)
	_, err := adapter.GetSetlist(context.Background(), "63de4613")
	if err == nil || !strings.Contains(err.Error(), "unexpected status 500") {
		t.Errorf("expected 'unexpected status 500' error, got %v", err)
	}
}

func TestGetSetlist_MalformedJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("not json {{{"))
	}))
	defer srv.Close()

	adapter := newTestAdapter(srv.URL)
	_, err := adapter.GetSetlist(context.Background(), "63de4613")
	if err == nil || !strings.Contains(err.Error(), "decoding response") {
		t.Errorf("expected 'decoding response' error, got %v", err)
	}
}
