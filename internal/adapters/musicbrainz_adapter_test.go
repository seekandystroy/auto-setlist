package adapters

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newTestMusicbrainzAdapter(serverURL string) *musicbrainzAdapter {
	return &musicbrainzAdapter{
		httpClient: &http.Client{},
		baseURL:    serverURL,
		userAgent:  "TestApp/1.0 ( test@example.com )",
	}
}

func makeRelation(resourceURL string) musicbrainzURLRelation {
	var rel musicbrainzURLRelation
	rel.URL.Resource = resourceURL
	return rel
}

func TestGetArtist_HappyPath(t *testing.T) {
	response := musicbrainzArtistResponse{
		ID:   "d262ea27-3ffe-40f7-b922-85c42d625e67",
		Name: "Pitbull",
		Relations: []musicbrainzURLRelation{
			makeRelation("https://open.spotify.com/artist/0TnOYISbd1XYRBk9myaseg"),
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer srv.Close()

	adapter := newTestMusicbrainzAdapter(srv.URL)
	artist, err := adapter.GetArtist("d262ea27-3ffe-40f7-b922-85c42d625e67")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if artist.MBID != "d262ea27-3ffe-40f7-b922-85c42d625e67" {
		t.Errorf("expected MBID %q, got %q", "d262ea27-3ffe-40f7-b922-85c42d625e67", artist.MBID)
	}
	if artist.Name != "Pitbull" {
		t.Errorf("expected Name %q, got %q", "Pitbull", artist.Name)
	}
	if artist.SpotifyID != "0TnOYISbd1XYRBk9myaseg" {
		t.Errorf("expected SpotifyID %q, got %q", "0TnOYISbd1XYRBk9myaseg", artist.SpotifyID)
	}
}

func TestGetArtist_NoSpotifyRelation(t *testing.T) {
	response := musicbrainzArtistResponse{
		ID:   "some-mbid",
		Name: "Some Artist",
		Relations: []musicbrainzURLRelation{
			makeRelation("https://www.allmusic.com/artist/some-artist"),
			makeRelation("https://www.discogs.com/artist/some-artist"),
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer srv.Close()

	adapter := newTestMusicbrainzAdapter(srv.URL)
	artist, err := adapter.GetArtist("some-mbid")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if artist.SpotifyID != "" {
		t.Errorf("expected empty SpotifyID, got %q", artist.SpotifyID)
	}
}

func TestGetArtist_NoRelations(t *testing.T) {
	response := musicbrainzArtistResponse{ID: "some-mbid", Name: "Some Artist"}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer srv.Close()

	adapter := newTestMusicbrainzAdapter(srv.URL)
	artist, err := adapter.GetArtist("some-mbid")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if artist.SpotifyID != "" {
		t.Errorf("expected empty SpotifyID, got %q", artist.SpotifyID)
	}
}

func TestGetArtist_UsesFirstSpotifyRelation(t *testing.T) {
	response := musicbrainzArtistResponse{
		ID:   "some-mbid",
		Name: "Some Artist",
		Relations: []musicbrainzURLRelation{
			makeRelation("https://www.discogs.com/artist/some-artist"),
			makeRelation("https://open.spotify.com/artist/first-spotify-id"),
			makeRelation("https://open.spotify.com/artist/second-spotify-id"),
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer srv.Close()

	adapter := newTestMusicbrainzAdapter(srv.URL)
	artist, err := adapter.GetArtist("some-mbid")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if artist.SpotifyID != "first-spotify-id" {
		t.Errorf("expected SpotifyID %q, got %q", "first-spotify-id", artist.SpotifyID)
	}
}

func TestGetArtist_NonOKStatus(t *testing.T) {
	for _, status := range []int{http.StatusUnauthorized, http.StatusNotFound, http.StatusInternalServerError} {
		status := status
		t.Run(http.StatusText(status), func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(status)
			}))
			defer srv.Close()

			adapter := newTestMusicbrainzAdapter(srv.URL)
			_, err := adapter.GetArtist("some-mbid")
			if err == nil {
				t.Fatalf("expected error for status %d, got nil", status)
			}
			if !strings.Contains(err.Error(), "unexpected status") {
				t.Errorf("expected 'unexpected status' in error, got %q", err.Error())
			}
		})
	}
}

func TestGetArtist_NetworkError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	srv.Close()

	adapter := newTestMusicbrainzAdapter(srv.URL)
	_, err := adapter.GetArtist("some-mbid")
	if err == nil {
		t.Fatal("expected network error, got nil")
	}
	if !strings.Contains(err.Error(), "executing request") {
		t.Errorf("expected 'executing request' in error, got %q", err.Error())
	}
}

func TestGetArtist_MalformedJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("not json {{{"))
	}))
	defer srv.Close()

	adapter := newTestMusicbrainzAdapter(srv.URL)
	_, err := adapter.GetArtist("some-mbid")
	if err == nil {
		t.Fatal("expected decode error, got nil")
	}
	if !strings.Contains(err.Error(), "decoding response") {
		t.Errorf("expected 'decoding response' in error, got %q", err.Error())
	}
}

func TestGetArtist_SendsCorrectHeaders(t *testing.T) {
	var gotUserAgent, gotAccept string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUserAgent = r.Header.Get("User-Agent")
		gotAccept = r.Header.Get("Accept")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(musicbrainzArtistResponse{ID: "x", Name: "x"})
	}))
	defer srv.Close()

	adapter := newTestMusicbrainzAdapter(srv.URL)
	adapter.GetArtist("some-mbid")

	if gotUserAgent != "TestApp/1.0 ( test@example.com )" {
		t.Errorf("unexpected User-Agent: %q", gotUserAgent)
	}
	if gotAccept != "application/json" {
		t.Errorf("expected Accept %q, got %q", "application/json", gotAccept)
	}
}
