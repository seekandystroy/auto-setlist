package service

import (
	"context"
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

func (s *service) SetlistToPlaylist(ctx context.Context, artistName string) (string, error) {
	token, err := s.spotify.GetValidToken()
	if err != nil {
		return "", fmt.Errorf("service: getting spotify token: %w", err)
	}
	return s.SetlistToPlaylistAuthed(ctx, artistName, token)
}

func (s *service) SetlistToPlaylistAuthed(ctx context.Context, artistName, token string) (string, error) {
	if artistName == "" {
		return "", fmt.Errorf("artist name must not be empty")
	}

	artists, err := s.searchArtists(ctx, artistName)
	if err != nil {
		return "", err
	}
	if len(artists) == 0 {
		return "", fmt.Errorf("no artist found for %q", artistName)
	}

	artist := artists[0]
	if err := s.fillSpotifyID(ctx, &artist); err != nil {
		return "", err
	}

	setlists, err := s.getSetlists(ctx, artist)
	if err != nil {
		return "", err
	}
	if len(setlists) == 0 {
		return "", fmt.Errorf("no setlists found for %q", artistName)
	}

	var setlist *domain.Setlist
	for _, s := range setlists {
		if len(s.Tracks) > 0 {
			setlist = &s
			break
		}
	}

	if setlist == nil {
		return "", fmt.Errorf("no non-empty setlists found for %q", artistName)
	}

	uris, err := s.spotify.GetSetlistTracks(ctx, token, *setlist)
	if err != nil {
		return "", err
	}

	playlistID, err := s.spotify.CreatePlaylist(ctx, token, *setlist, uris)
	if err != nil {
		return "", err
	}

	return playlistID, nil
}

func (s *service) searchArtists(ctx context.Context, artistName string) ([]domain.Artist, error) {
	result, err := s.setlistfm.SearchArtists(ctx, artistName)
	if err != nil {
		return nil, fmt.Errorf("searching for artist %q: %w", artistName, err)
	}
	return result, nil
}

func (s *service) fillSpotifyID(ctx context.Context, artist *domain.Artist) error {
	result, err := s.musicbrainz.GetArtist(ctx, artist.MBID)
	if err != nil {
		return fmt.Errorf("fetching artist from MusicBrainz: %w", err)
	}
	artist.SpotifyID = result.SpotifyID
	return nil
}

func (s *service) getSetlists(ctx context.Context, artist domain.Artist) ([]domain.Setlist, error) {
	setlists, err := s.setlistfm.GetSetlists(ctx, artist)
	if err != nil {
		return nil, fmt.Errorf("fetching setlists for %q: %w", artist.Name, err)
	}
	return setlists, nil
}
