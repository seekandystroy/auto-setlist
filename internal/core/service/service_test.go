package service

import (
	"errors"
	"testing"

	"github.com/seekandystroy/auto-setlist/internal/core/domain"
)

type mockSearcher struct {
	result      []domain.Artist
	searchErr   error
	setlists    []domain.Setlist
	setlistsErr error
}

func (m *mockSearcher) SearchArtists(name string) ([]domain.Artist, error) {
	return m.result, m.searchErr
}

func (m *mockSearcher) GetSetlists(artist domain.Artist) ([]domain.Setlist, error) {
	return m.setlists, m.setlistsErr
}

type mockSpotify struct {
	token string
	err   error
}

func (m *mockSpotify) GetValidToken() (string, error) {
	return m.token, m.err
}

type mockMusicbrainz struct {
	artist *domain.Artist
	err    error
}

func (m *mockMusicbrainz) GetArtist(mbid string) (*domain.Artist, error) {
	return m.artist, m.err
}

func newSvc(searcher *mockSearcher, mb *mockMusicbrainz) *service {
	return NewService(searcher, &mockSpotify{}, mb)
}

func TestGetArtistSetlists_HappyPath(t *testing.T) {
	artist := domain.Artist{MBID: "abc", Name: "Sprout"}
	enriched := domain.Artist{MBID: "abc", Name: "Sprout", SpotifyID: "spotify-123"}
	setlists := []domain.Setlist{{Artist: enriched, Tracks: []string{"Song A", "Song B"}}}

	svc := newSvc(
		&mockSearcher{result: []domain.Artist{artist}, setlists: setlists},
		&mockMusicbrainz{artist: &enriched},
	)

	got, err := svc.GetArtistSetlists("Sprout")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 setlist, got %d", len(got))
	}
	if len(got[0].Tracks) != 2 {
		t.Errorf("expected 2 tracks, got %d", len(got[0].Tracks))
	}
	if got[0].Artist.SpotifyID != "spotify-123" {
		t.Errorf("expected SpotifyID %q, got %q", "spotify-123", got[0].Artist.SpotifyID)
	}
}

func TestGetArtistSetlists_EmptyName(t *testing.T) {
	svc := newSvc(&mockSearcher{}, &mockMusicbrainz{})

	_, err := svc.GetArtistSetlists("")
	if err == nil {
		t.Fatal("expected error for empty name, got nil")
	}
	if err.Error() != "artist name must not be empty" {
		t.Errorf("unexpected error message: %q", err.Error())
	}
}

func TestGetArtistSetlists_NoArtistsFound(t *testing.T) {
	svc := newSvc(&mockSearcher{result: []domain.Artist{}}, &mockMusicbrainz{})

	_, err := svc.GetArtistSetlists("Ghost")
	if err == nil {
		t.Fatal("expected error for empty results, got nil")
	}
}

func TestGetArtistSetlists_SearchError(t *testing.T) {
	underlying := errors.New("network failure")
	svc := newSvc(&mockSearcher{searchErr: underlying}, &mockMusicbrainz{})

	_, err := svc.GetArtistSetlists("Sprout")
	if !errors.Is(err, underlying) {
		t.Errorf("expected wrapped search error, got %v", err)
	}
}

func TestGetArtistSetlists_MusicbrainzError(t *testing.T) {
	underlying := errors.New("mbz down")
	svc := newSvc(
		&mockSearcher{result: []domain.Artist{{MBID: "abc", Name: "Sprout"}}},
		&mockMusicbrainz{err: underlying},
	)

	_, err := svc.GetArtistSetlists("Sprout")
	if !errors.Is(err, underlying) {
		t.Errorf("expected wrapped musicbrainz error, got %v", err)
	}
}

func TestGetArtistSetlists_SetlistsError(t *testing.T) {
	underlying := errors.New("setlists unavailable")
	artist := domain.Artist{MBID: "abc", Name: "Sprout"}
	svc := newSvc(
		&mockSearcher{result: []domain.Artist{artist}, setlistsErr: underlying},
		&mockMusicbrainz{artist: &artist},
	)

	_, err := svc.GetArtistSetlists("Sprout")
	if !errors.Is(err, underlying) {
		t.Errorf("expected wrapped setlists error, got %v", err)
	}
}
