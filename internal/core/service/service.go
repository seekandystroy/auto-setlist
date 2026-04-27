package service

import (
	"fmt"

	"github.com/seekandystroy/auto-setlist/internal/core/domain"
	"github.com/seekandystroy/auto-setlist/internal/ports"
)

type service struct {
	searcher ports.SetlistFm
}

func NewService(setlistfm ports.SetlistFm) *service {
	return &service{searcher: setlistfm}
}

func (s *service) SearchArtistsJSON(name string) ([]domain.Artist, error) {
	if name == "" {
		return nil, fmt.Errorf("artist name must not be empty")
	}

	result, err := s.searcher.SearchArtists(name)
	if err != nil {
		return nil, fmt.Errorf("searching for artist %q: %w", name, err)
	}

	return result.Artists, nil
}
