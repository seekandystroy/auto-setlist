package service

import (
	"fmt"

	"github.com/seekandystroy/auto-setlist/internal/core/domain"
	"github.com/seekandystroy/auto-setlist/internal/ports"
)

type service struct {
	setlistfm   ports.SetlistFm
	spotify     ports.Spotify
	musicbrainz ports.Musicbrainz
}

func NewService(setlistfm ports.SetlistFm, spotify ports.Spotify, musicbrainz ports.Musicbrainz) *service {
	return &service{setlistfm: setlistfm, spotify: spotify, musicbrainz: musicbrainz}
}

func (s *service) AuthWithSpotify() (string, error) {
	token, err := s.spotify.GetValidToken()
	if err != nil {
		return "", fmt.Errorf("authenticating with Spotify: %w", err)
	}
	return token, nil
}

func (s *service) GetArtistSetlists(name string) ([]domain.Setlist, error) {
	if name == "" {
		return nil, fmt.Errorf("artist name must not be empty")
	}

	artists, err := s.searchArtists(name)
	if err != nil {
		return nil, err
	}
	if len(artists) == 0 {
		return nil, fmt.Errorf("no artist found for %q", name)
	}

	artist := artists[0]
	if err := s.fillSpotifyID(&artist); err != nil {
		return nil, err
	}

	return s.getSetlists(artist)
}

func (s *service) searchArtists(name string) ([]domain.Artist, error) {
	result, err := s.setlistfm.SearchArtists(name)
	if err != nil {
		return nil, fmt.Errorf("searching for artist %q: %w", name, err)
	}
	return result, nil
}

func (s *service) fillSpotifyID(artist *domain.Artist) error {
	result, err := s.musicbrainz.GetArtist(artist.MBID)
	if err != nil {
		return fmt.Errorf("fetching artist from MusicBrainz: %w", err)
	}
	artist.SpotifyID = result.SpotifyID
	return nil
}

func (s *service) getSetlists(artist domain.Artist) ([]domain.Setlist, error) {
	setlists, err := s.setlistfm.GetSetlists(artist)
	if err != nil {
		return nil, fmt.Errorf("fetching setlists for %q: %w", artist.Name, err)
	}
	return setlists, nil
}
