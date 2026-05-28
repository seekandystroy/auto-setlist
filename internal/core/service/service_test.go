package service

import (
	"context"
	"errors"
	"testing"

	"github.com/seekandystroy/auto-setlist/internal/core/domain"
)

type mockSetlistfm struct {
	result           []domain.Artist
	searchErr        error
	setlists         []domain.Setlist
	setlistsErr      error
	tourSetlists     []domain.Setlist
	tourSetlistsErr  error
	receivedTourName string
}

func (m *mockSetlistfm) SearchArtists(_ context.Context, name string) ([]domain.Artist, error) {
	return m.result, m.searchErr
}

func (m *mockSetlistfm) GetSetlists(_ context.Context, artist domain.Artist) ([]domain.Setlist, error) {
	return m.setlists, m.setlistsErr
}

func (m *mockSetlistfm) GetSetlistsForTour(_ context.Context, artist domain.Artist, tourName string) ([]domain.Setlist, error) {
	m.receivedTourName = tourName
	return m.tourSetlists, m.tourSetlistsErr
}

type mockSpotify struct {
	token                string
	uris                 []string
	playlistID           string
	err                  error
	receivedSetlist      domain.Setlist
	receivedToken        string
	receivedIncludeCovers bool
}

func (m *mockSpotify) GetValidToken() (string, error) {
	return m.token, m.err
}

func (m *mockSpotify) GetSetlistTracks(_ context.Context, token string, s domain.Setlist, includeCovers bool) ([]string, error) {
	m.receivedSetlist = s
	m.receivedToken = token
	m.receivedIncludeCovers = includeCovers
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
	setlists := []domain.Setlist{{Artist: enriched, Tracks: []domain.Track{{Name: "Song A"}, {Name: "Song B"}}}}
	expectedURIs := []string{"spotify:track:uri1", "spotify:track:uri2"}

	svc := newSvc(
		&mockSetlistfm{result: []domain.Artist{artist}, setlists: setlists},
		&mockMusicbrainz{artist: &enriched},
		&mockSpotify{uris: expectedURIs, playlistID: "playlist-abc"},
	)

	playlistID, err := svc.SetlistToPlaylist(context.Background(), "Sprout", false, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if playlistID != "playlist-abc" {
		t.Errorf("unexpected playlist ID: %s", playlistID)
	}
}

func TestGetArtistSetlists_SkipsEmptySetlists(t *testing.T) {
	artist := domain.Artist{MBID: "abc", Name: "Sprout"}
	thirdSetlist := domain.Setlist{Artist: artist, Tracks: []domain.Track{{Name: "Song A"}, {Name: "Song B"}}}
	setlists := []domain.Setlist{
		{Artist: artist, Tracks: []domain.Track{}},
		{Artist: artist, Tracks: []domain.Track{}},
		thirdSetlist,
	}
	spotify := &mockSpotify{uris: []string{"spotify:track:uri1"}}

	svc := newSvc(
		&mockSetlistfm{result: []domain.Artist{artist}, setlists: setlists},
		&mockMusicbrainz{artist: &artist},
		spotify,
	)

	_, err := svc.SetlistToPlaylist(context.Background(), "Sprout", false, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(spotify.receivedSetlist.Tracks) != 2 {
		t.Errorf("expected third setlist (2 tracks) to be used, got %d tracks", len(spotify.receivedSetlist.Tracks))
	}
	if spotify.receivedSetlist.Tracks[0].Name != "Song A" {
		t.Errorf("unexpected first track: %s", spotify.receivedSetlist.Tracks[0].Name)
	}
}

func TestGetArtistSetlists_EmptyName(t *testing.T) {
	svc := newSvc(&mockSetlistfm{}, &mockMusicbrainz{}, &mockSpotify{})

	_, err := svc.SetlistToPlaylist(context.Background(), "", false, false)
	if err == nil {
		t.Fatal("expected error for empty name, got nil")
	}
	if err.Error() != "artist name must not be empty" {
		t.Errorf("unexpected error message: %q", err.Error())
	}
}

func TestGetArtistSetlists_NoArtistsFound(t *testing.T) {
	svc := newSvc(&mockSetlistfm{result: []domain.Artist{}}, &mockMusicbrainz{}, &mockSpotify{})

	_, err := svc.SetlistToPlaylist(context.Background(), "Ghost", false, false)
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

	_, err := svc.SetlistToPlaylist(context.Background(), "Sprout", false, false)
	if err == nil {
		t.Fatal("expected error for empty setlists, got nil")
	}
}

func TestGetArtistSetlists_SearchError(t *testing.T) {
	underlying := errors.New("network failure")
	svc := newSvc(&mockSetlistfm{searchErr: underlying}, &mockMusicbrainz{}, &mockSpotify{})

	_, err := svc.SetlistToPlaylist(context.Background(), "Sprout", false, false)
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

	_, err := svc.SetlistToPlaylist(context.Background(), "Sprout", false, false)
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

	_, err := svc.SetlistToPlaylist(context.Background(), "Sprout", false, false)
	if !errors.Is(err, underlying) {
		t.Errorf("expected wrapped setlists error, got %v", err)
	}
}

func TestGetArtistSetlists_SpotifyError(t *testing.T) {
	underlying := errors.New("spotify unavailable")
	artist := domain.Artist{MBID: "abc", Name: "Sprout"}
	setlists := []domain.Setlist{{Artist: artist, Tracks: []domain.Track{{Name: "Song A"}}}}
	svc := newSvc(
		&mockSetlistfm{result: []domain.Artist{artist}, setlists: setlists},
		&mockMusicbrainz{artist: &artist},
		&mockSpotify{err: underlying},
	)

	_, err := svc.SetlistToPlaylist(context.Background(), "Sprout", false, false)
	if !errors.Is(err, underlying) {
		t.Errorf("expected wrapped spotify error, got %v", err)
	}
}

func TestSetlistToPlaylistAuthed_HappyPath(t *testing.T) {
	artist := domain.Artist{MBID: "abc", Name: "Sprout"}
	enriched := domain.Artist{MBID: "abc", Name: "Sprout", SpotifyID: "spotify-123"}
	setlists := []domain.Setlist{{Artist: enriched, Tracks: []domain.Track{{Name: "Song A"}, {Name: "Song B"}}}}
	spotify := &mockSpotify{uris: []string{"spotify:track:uri1"}, playlistID: "playlist-authed"}

	svc := newSvc(
		&mockSetlistfm{result: []domain.Artist{artist}, setlists: setlists},
		&mockMusicbrainz{artist: &enriched},
		spotify,
	)

	playlistID, err := svc.SetlistToPlaylistAuthed(context.Background(), "Sprout", "user-token", false, false)
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

	_, err := svc.SetlistToPlaylistAuthed(context.Background(), "", "tok", false, false)
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
	setlists := []domain.Setlist{{Artist: artist, Tracks: []domain.Track{{Name: "Song A"}}}}
	svc := newSvc(
		&mockSetlistfm{result: []domain.Artist{artist}, setlists: setlists},
		&mockMusicbrainz{artist: &artist},
		&mockSpotify{err: underlying},
	)

	_, err := svc.SetlistToPlaylistAuthed(context.Background(), "Sprout", "tok", false, false)
	if !errors.Is(err, underlying) {
		t.Errorf("expected wrapped spotify error, got %v", err)
	}
}

func TestSetlistToPlaylistAuthed_PassesIncludeCoversToSpotify(t *testing.T) {
	artist := domain.Artist{MBID: "abc", Name: "Sprout"}
	setlists := []domain.Setlist{{Artist: artist, Tracks: []domain.Track{{Name: "Song A"}}}}
	spotify := &mockSpotify{uris: []string{"spotify:track:uri1"}, playlistID: "p1"}

	svc := newSvc(
		&mockSetlistfm{result: []domain.Artist{artist}, setlists: setlists},
		&mockMusicbrainz{artist: &artist},
		spotify,
	)

	_, err := svc.SetlistToPlaylistAuthed(context.Background(), "Sprout", "tok", true, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !spotify.receivedIncludeCovers {
		t.Error("expected includeCovers=true to be forwarded to spotify, got false")
	}
}

func TestAllSongsFromLatestTour_MergesTracks(t *testing.T) {
	artist := domain.Artist{MBID: "abc", Name: "Sprout"}
	latestSetlist := domain.Setlist{
		Artist: artist,
		Tour:   "World Tour 2025",
		Tracks: []domain.Track{{Name: "Song A"}, {Name: "Song B"}},
	}
	tourSetlists := []domain.Setlist{
		{Artist: artist, Tour: "World Tour 2025", Tracks: []domain.Track{{Name: "Song A"}, {Name: "Song B"}, {Name: "Song C"}}},
		{Artist: artist, Tour: "World Tour 2025", Tracks: []domain.Track{{Name: "Song A"}, {Name: "Song D"}}},
	}
	spotify := &mockSpotify{uris: []string{"uri1"}, playlistID: "p1"}
	sfm := &mockSetlistfm{
		result:       []domain.Artist{artist},
		setlists:     []domain.Setlist{latestSetlist},
		tourSetlists: tourSetlists,
	}

	svc := newSvc(sfm, &mockMusicbrainz{artist: &artist}, spotify)
	_, err := svc.SetlistToPlaylistAuthed(context.Background(), "Sprout", "tok", false, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sfm.receivedTourName != "World Tour 2025" {
		t.Errorf("expected tour name %q forwarded to GetSetlistsForTour, got %q", "World Tour 2025", sfm.receivedTourName)
	}
	// Position 0: Song A (from both, deduplicated) → [Song A]
	// Position 1: Song B (setlist 0), Song D (setlist 1, new) → [Song A, Song B, Song D]
	// Position 2: Song C (setlist 0, new) → [Song A, Song B, Song D, Song C]
	expected := []string{"Song A", "Song B", "Song D", "Song C"}
	if len(spotify.receivedSetlist.Tracks) != len(expected) {
		t.Fatalf("expected %d tracks, got %d: %+v", len(expected), len(spotify.receivedSetlist.Tracks), spotify.receivedSetlist.Tracks)
	}
	for i, name := range expected {
		if spotify.receivedSetlist.Tracks[i].Name != name {
			t.Errorf("track[%d]: expected %q, got %q", i, name, spotify.receivedSetlist.Tracks[i].Name)
		}
	}
	if spotify.receivedSetlist.Tour != "World Tour 2025" {
		t.Errorf("expected merged setlist Tour %q, got %q", "World Tour 2025", spotify.receivedSetlist.Tour)
	}
}

func TestAllSongsFromLatestTour_FallsBackWhenNoTour(t *testing.T) {
	artist := domain.Artist{MBID: "abc", Name: "Sprout"}
	latestSetlist := domain.Setlist{
		Artist: artist,
		Tour:   "",
		Tracks: []domain.Track{{Name: "Song A"}},
	}
	sfm := &mockSetlistfm{
		result:   []domain.Artist{artist},
		setlists: []domain.Setlist{latestSetlist},
	}
	spotify := &mockSpotify{uris: []string{"uri1"}, playlistID: "p1"}

	svc := newSvc(sfm, &mockMusicbrainz{artist: &artist}, spotify)
	_, err := svc.SetlistToPlaylistAuthed(context.Background(), "Sprout", "tok", false, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sfm.receivedTourName != "" {
		t.Error("expected GetSetlistsForTour not to be called when setlist has no tour")
	}
	if len(spotify.receivedSetlist.Tracks) != 1 || spotify.receivedSetlist.Tracks[0].Name != "Song A" {
		t.Errorf("expected single-setlist fallback, got %+v", spotify.receivedSetlist.Tracks)
	}
}

func TestAllSongsFromLatestTour_GetSetlistsForTourError(t *testing.T) {
	underlying := errors.New("tour fetch failed")
	artist := domain.Artist{MBID: "abc", Name: "Sprout"}
	latestSetlist := domain.Setlist{Artist: artist, Tour: "Some Tour", Tracks: []domain.Track{{Name: "Song A"}}}
	sfm := &mockSetlistfm{
		result:          []domain.Artist{artist},
		setlists:        []domain.Setlist{latestSetlist},
		tourSetlistsErr: underlying,
	}

	svc := newSvc(sfm, &mockMusicbrainz{artist: &artist}, &mockSpotify{})
	_, err := svc.SetlistToPlaylistAuthed(context.Background(), "Sprout", "tok", false, true)
	if !errors.Is(err, underlying) {
		t.Errorf("expected wrapped tour fetch error, got %v", err)
	}
}

func TestMergeSetlistTracks_PositionOrder(t *testing.T) {
	setlists := []domain.Setlist{
		{Tracks: []domain.Track{{Name: "A"}, {Name: "B"}}},
		{Tracks: []domain.Track{{Name: "A"}, {Name: "C"}, {Name: "D"}}},
	}
	result := mergeSetlistTracks(setlists)
	expected := []string{"A", "B", "C", "D"}
	if len(result) != len(expected) {
		t.Fatalf("expected %d tracks, got %d: %+v", len(expected), len(result), result)
	}
	for i, name := range expected {
		if result[i].Name != name {
			t.Errorf("track[%d]: expected %q, got %q", i, name, result[i].Name)
		}
	}
}

func TestMergeSetlistTracks_Empty(t *testing.T) {
	if result := mergeSetlistTracks(nil); len(result) != 0 {
		t.Errorf("expected empty result for nil setlists, got %+v", result)
	}
	if result := mergeSetlistTracks([]domain.Setlist{}); len(result) != 0 {
		t.Errorf("expected empty result for empty setlists, got %+v", result)
	}
}
