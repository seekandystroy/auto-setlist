package service

import (
	"fmt"

	"github.com/seekandystroy/auto-setlist/internal/core/domain"
	"github.com/seekandystroy/auto-setlist/internal/ports"
)

type service struct {
	setlistfm   ports.Setlistfm
	spotify     ports.Spotify
	musicbrainz ports.Musicbrainz
}

func NewService(setlistfm ports.Setlistfm, spotify ports.Spotify, musicbrainz ports.Musicbrainz) *service {
	return &service{setlistfm: setlistfm, spotify: spotify, musicbrainz: musicbrainz}
}

// MVP using the first Artist and first setlist found in the first page
func (s *service) SetlistToPlaylist(name string) (string, []string, error) {
	if name == "" {
		return "", nil, fmt.Errorf("artist name must not be empty")
	}

	artists, err := s.searchArtists(name)
	if err != nil {
		return "", nil, err
	}
	if len(artists) == 0 {
		return "", nil, fmt.Errorf("no artist found for %q", name)
	}

	artist := artists[0]
	if err := s.fillSpotifyID(&artist); err != nil {
		return "", nil, err
	}

	setlists, err := s.getSetlists(artist)
	if err != nil {
		return "", nil, err
	}
	if len(setlists) == 0 {
		return "", nil, fmt.Errorf("no setlists found for %q", name)
	}

	var setlist *domain.Setlist
	for _, s := range setlists {
		if len(s.Tracks) > 0 {
			setlist = &s
			break
		}
	}

	if setlist == nil {
		return "", nil, fmt.Errorf("no non-empty setlists found for %q", name)
	}

	uris, err := s.spotify.GetSetlistTracks(*setlist)
	if err != nil {
		return "", nil, err
	}

	playlistID, err := s.spotify.CreatePlaylist(*setlist, uris)
	if err != nil {
		return "", nil, err
	}

	return playlistID, uris, nil
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
