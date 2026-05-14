package service

import (
	"fmt"

	"github.com/seekandystroy/auto-setlist/internal/core/domain"
	"github.com/seekandystroy/auto-setlist/internal/ports"
)

type service struct {
	setlistfm ports.SetlistFm
	spotify   ports.Spotify
}

func NewService(setlistfm ports.SetlistFm, spotify ports.Spotify) *service {
	return &service{setlistfm: setlistfm, spotify: spotify}
}

func (s *service) AuthWithSpotify() (string, error) {
	token, err := s.spotify.GetValidToken()
	if err != nil {
		return "", fmt.Errorf("authenticating with Spotify: %w", err)
	}
	return token, nil
}

func (s *service) SearchArtistsJSON(name string) ([]domain.Artist, error) {
	if name == "" {
		return nil, fmt.Errorf("artist name must not be empty")
	}

	result, err := s.setlistfm.SearchArtists(name)
	if err != nil {
		return nil, fmt.Errorf("searching for artist %q: %w", name, err)
	}

	return result, nil
}
