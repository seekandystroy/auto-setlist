package service

import (
	"errors"
	"strings"
	"testing"

	"github.com/seekandystroy/auto-setlist/internal/core/domain"
)

type mockSearcher struct {
	result []domain.Artist
	err    error
}

func (m *mockSearcher) SearchArtists(name string) ([]domain.Artist, error) {
	return m.result, m.err
}

type mockSpotify struct {
	token string
	err   error
}

func (m *mockSpotify) GetValidToken() (string, error) {
	return m.token, m.err
}

func TestSearchArtistsJSON_EmptyName(t *testing.T) {
	svc := NewService(&mockSearcher{}, &mockSpotify{})
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
	svc := NewService(mock, &mockSpotify{})

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
	svc := NewService(mock, &mockSpotify{})

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
