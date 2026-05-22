package service

import (
	"context"
	"errors"
	"testing"

	"github.com/seekandystroy/auto-setlist/internal/core/domain"
)

type mockSetlistfm struct {
	result      []domain.Artist
	searchErr   error
	setlists    []domain.Setlist
	setlistsErr error
}

func (m *mockSetlistfm) SearchArtists(_ context.Context, name string) ([]domain.Artist, error) {
	return m.result, m.searchErr
}

func (m *mockSetlistfm) GetSetlists(_ context.Context, artist domain.Artist) ([]domain.Setlist, error) {
	return m.setlists, m.setlistsErr
}

type mockSpotify struct {
	token           string
	uris            []string
	playlistID      string
	err             error
	receivedSetlist domain.Setlist
	receivedToken   string
}

func (m *mockSpotify) GetValidToken() (string, error) {
	return m.token, m.err
}

func (m *mockSpotify) GetSetlistTracks(_ context.Context, token string, s domain.Setlist) ([]string, error) {
	m.receivedSetlist = s
	m.receivedToken = token
	return m.uris, m.err
}

func (m *mockSpotify) CreatePlaylist(_ context.Context, token string, _ domain.Setlist, _ []string) (string, error) {
	m.receivedToken = token
	return m.playlistID, m.err
}

type mockMusicbrainz struct {
	artist *domain.Artist
	err    error
}

func (m *mockMusicbrainz) GetArtist(_ context.Context, mbid string) (*domain.Artist, error) {
	return m.artist, m.err
}

func newSvc(setlistfm *mockSetlistfm, mb *mockMusicbrainz, spotify *mockSpotify) *service {
	return NewService(setlistfm, spotify, mb)
}

func TestGetArtistSetlists_HappyPath(t *testing.T) {
	artist := domain.Artist{MBID: "abc", Name: "Sprout"}
	enriched := domain.Artist{MBID: "abc", Name: "Sprout", SpotifyID: "spotify-123"}
	setlists := []domain.Setlist{{Artist: enriched, Tracks: []string{"Song A", "Song B"}}}
	expectedURIs := []string{"spotify:track:uri1", "spotify:track:uri2"}

	svc := newSvc(
		&mockSetlistfm{result: []domain.Artist{artist}, setlists: setlists},
		&mockMusicbrainz{artist: &enriched},
		&mockSpotify{uris: expectedURIs, playlistID: "playlist-abc"},
	)

	playlistID, err := svc.SetlistToPlaylist(context.Background(), "Sprout")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if playlistID != "playlist-abc" {
		t.Errorf("unexpected playlist ID: %s", playlistID)
	}
}

func TestGetArtistSetlists_SkipsEmptySetlists(t *testing.T) {
	artist := domain.Artist{MBID: "abc", Name: "Sprout"}
	thirdSetlist := domain.Setlist{Artist: artist, Tracks: []string{"Song A", "Song B"}}
	setlists := []domain.Setlist{
		{Artist: artist, Tracks: []string{}},
		{Artist: artist, Tracks: []string{}},
		thirdSetlist,
	}
	spotify := &mockSpotify{uris: []string{"spotify:track:uri1"}}

	svc := newSvc(
		&mockSetlistfm{result: []domain.Artist{artist}, setlists: setlists},
		&mockMusicbrainz{artist: &artist},
		spotify,
	)

	_, err := svc.SetlistToPlaylist(context.Background(), "Sprout")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(spotify.receivedSetlist.Tracks) != 2 {
		t.Errorf("expected third setlist (2 tracks) to be used, got %d tracks", len(spotify.receivedSetlist.Tracks))
	}
	if spotify.receivedSetlist.Tracks[0] != "Song A" {
		t.Errorf("unexpected first track: %s", spotify.receivedSetlist.Tracks[0])
	}
}

func TestGetArtistSetlists_EmptyName(t *testing.T) {
	svc := newSvc(&mockSetlistfm{}, &mockMusicbrainz{}, &mockSpotify{})

	_, err := svc.SetlistToPlaylist(context.Background(), "")
	if err == nil {
		t.Fatal("expected error for empty name, got nil")
	}
	if err.Error() != "artist name must not be empty" {
		t.Errorf("unexpected error message: %q", err.Error())
	}
}

func TestGetArtistSetlists_NoArtistsFound(t *testing.T) {
	svc := newSvc(&mockSetlistfm{result: []domain.Artist{}}, &mockMusicbrainz{}, &mockSpotify{})

	_, err := svc.SetlistToPlaylist(context.Background(), "Ghost")
	if err == nil {
		t.Fatal("expected error for empty results, got nil")
	}
}

func TestGetArtistSetlists_NoSetlistsFound(t *testing.T) {
	artist := domain.Artist{MBID: "abc", Name: "Sprout"}
	svc := newSvc(
		&mockSetlistfm{result: []domain.Artist{artist}, setlists: []domain.Setlist{}},
		&mockMusicbrainz{artist: &artist},
		&mockSpotify{},
	)

	_, err := svc.SetlistToPlaylist(context.Background(), "Sprout")
	if err == nil {
		t.Fatal("expected error for empty setlists, got nil")
	}
}

func TestGetArtistSetlists_SearchError(t *testing.T) {
	underlying := errors.New("network failure")
	svc := newSvc(&mockSetlistfm{searchErr: underlying}, &mockMusicbrainz{}, &mockSpotify{})

	_, err := svc.SetlistToPlaylist(context.Background(), "Sprout")
	if !errors.Is(err, underlying) {
		t.Errorf("expected wrapped search error, got %v", err)
	}
}

func TestGetArtistSetlists_MusicbrainzError(t *testing.T) {
	underlying := errors.New("mbz down")
	svc := newSvc(
		&mockSetlistfm{result: []domain.Artist{{MBID: "abc", Name: "Sprout"}}},
		&mockMusicbrainz{err: underlying},
		&mockSpotify{},
	)

	_, err := svc.SetlistToPlaylist(context.Background(), "Sprout")
	if !errors.Is(err, underlying) {
		t.Errorf("expected wrapped musicbrainz error, got %v", err)
	}
}

func TestGetArtistSetlists_SetlistsError(t *testing.T) {
	underlying := errors.New("setlists unavailable")
	artist := domain.Artist{MBID: "abc", Name: "Sprout"}
	svc := newSvc(
		&mockSetlistfm{result: []domain.Artist{artist}, setlistsErr: underlying},
		&mockMusicbrainz{artist: &artist},
		&mockSpotify{},
	)

	_, err := svc.SetlistToPlaylist(context.Background(), "Sprout")
	if !errors.Is(err, underlying) {
		t.Errorf("expected wrapped setlists error, got %v", err)
	}
}

func TestGetArtistSetlists_SpotifyError(t *testing.T) {
	underlying := errors.New("spotify unavailable")
	artist := domain.Artist{MBID: "abc", Name: "Sprout"}
	setlists := []domain.Setlist{{Artist: artist, Tracks: []string{"Song A"}}}
	svc := newSvc(
		&mockSetlistfm{result: []domain.Artist{artist}, setlists: setlists},
		&mockMusicbrainz{artist: &artist},
		&mockSpotify{err: underlying},
	)

	_, err := svc.SetlistToPlaylist(context.Background(), "Sprout")
	if !errors.Is(err, underlying) {
		t.Errorf("expected wrapped spotify error, got %v", err)
	}
}

func TestSetlistToPlaylistAuthed_HappyPath(t *testing.T) {
	artist := domain.Artist{MBID: "abc", Name: "Sprout"}
	enriched := domain.Artist{MBID: "abc", Name: "Sprout", SpotifyID: "spotify-123"}
	setlists := []domain.Setlist{{Artist: enriched, Tracks: []string{"Song A", "Song B"}}}
	spotify := &mockSpotify{uris: []string{"spotify:track:uri1"}, playlistID: "playlist-authed"}

	svc := newSvc(
		&mockSetlistfm{result: []domain.Artist{artist}, setlists: setlists},
		&mockMusicbrainz{artist: &enriched},
		spotify,
	)

	playlistID, err := svc.SetlistToPlaylistAuthed(context.Background(), "Sprout", "user-token")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if playlistID != "playlist-authed" {
		t.Errorf("unexpected playlist ID: %s", playlistID)
	}
	if spotify.receivedToken != "user-token" {
		t.Errorf("expected token %q forwarded to spotify, got %q", "user-token", spotify.receivedToken)
	}
}

func TestSetlistToPlaylistAuthed_EmptyName(t *testing.T) {
	svc := newSvc(&mockSetlistfm{}, &mockMusicbrainz{}, &mockSpotify{})

	_, err := svc.SetlistToPlaylistAuthed(context.Background(), "", "tok")
	if err == nil {
		t.Fatal("expected error for empty name, got nil")
	}
	if err.Error() != "artist name must not be empty" {
		t.Errorf("unexpected error message: %q", err.Error())
	}
}

func TestSetlistToPlaylistAuthed_SpotifyOperationError(t *testing.T) {
	underlying := errors.New("spotify op failed")
	artist := domain.Artist{MBID: "abc", Name: "Sprout"}
	setlists := []domain.Setlist{{Artist: artist, Tracks: []string{"Song A"}}}
	svc := newSvc(
		&mockSetlistfm{result: []domain.Artist{artist}, setlists: setlists},
		&mockMusicbrainz{artist: &artist},
		&mockSpotify{err: underlying},
	)

	_, err := svc.SetlistToPlaylistAuthed(context.Background(), "Sprout", "tok")
	if !errors.Is(err, underlying) {
		t.Errorf("expected wrapped spotify error, got %v", err)
	}
}
