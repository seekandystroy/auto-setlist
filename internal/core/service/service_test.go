package service

import (
	"errors"
	"strings"
	"testing"

	"github.com/seekandystroy/auto-setlist/internal/core/domain"
)

type mockSearcher struct {
	result   []domain.Artist
	setlists []domain.Setlist
	err      error
}

func (m *mockSearcher) SearchArtists(name string) ([]domain.Artist, error) {
	return m.result, m.err
}

func (m *mockSearcher) GetSetlists(artist domain.Artist) ([]domain.Setlist, error) {
	return m.setlists, m.err
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

func TestGetSetlists_ReturnsSetlists(t *testing.T) {
	artist := domain.Artist{MBID: "abc", Name: "Sprout"}
	expected := []domain.Setlist{
		{Artist: artist, Tracks: []string{"Song A", "Song B"}},
	}
	mock := &mockSearcher{setlists: expected}
	svc := NewService(mock, &mockSpotify{}, &mockMusicbrainz{})

	got, err := svc.GetSetlists(artist)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 setlist, got %d", len(got))
	}
	if len(got[0].Tracks) != 2 {
		t.Errorf("expected 2 tracks, got %d", len(got[0].Tracks))
	}
}

func TestGetSetlists_WrapsError(t *testing.T) {
	underlying := errors.New("connection refused")
	mock := &mockSearcher{err: underlying}
	svc := NewService(mock, &mockSpotify{}, &mockMusicbrainz{})

	_, err := svc.GetSetlists(domain.Artist{MBID: "abc", Name: "Sprout"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, underlying) {
		t.Errorf("expected wrapped error to contain %v, got %v", underlying, err)
	}
	if !strings.Contains(err.Error(), "Sprout") {
		t.Errorf("expected error to mention artist name, got %q", err.Error())
	}
}

func TestSearchArtistsJSON_EmptyName(t *testing.T) {
	svc := NewService(&mockSearcher{}, &mockSpotify{}, &mockMusicbrainz{})
	_, err := svc.SearchArtistsJSON("")
	if err == nil {
		t.Fatal("expected error for empty name, got nil")
	}
	if err.Error() != "artist name must not be empty" {
		t.Errorf("unexpected error message: %q", err.Error())
	}
}

func TestSearchArtistsJSON_ReturnsArtists(t *testing.T) {
	artists := []domain.Artist{
		{MBID: "abc", Name: "Sprout"},
	}
	mock := &mockSearcher{result: artists}
	svc := NewService(mock, &mockSpotify{}, &mockMusicbrainz{})

	got, err := svc.SearchArtistsJSON("Sprout")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 artist, got %d", len(got))
	}
	if got[0].Name != "Sprout" {
		t.Errorf("expected Name %q, got %q", "Sprout", got[0].Name)
	}
}

func TestSearchArtistsJSON_WrapsSearcherError(t *testing.T) {
	underlying := errors.New("connection refused")
	mock := &mockSearcher{err: underlying}
	svc := NewService(mock, &mockSpotify{}, &mockMusicbrainz{})

	_, err := svc.SearchArtistsJSON("Sprout")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, underlying) {
		t.Errorf("expected wrapped error to contain %v, got %v", underlying, err)
	}
	if !strings.Contains(err.Error(), "Sprout") {
		t.Errorf("expected error to mention artist name, got %q", err.Error())
	}
}
