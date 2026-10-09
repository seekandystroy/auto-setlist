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
	setlistID        string
	setlistIDErr     error
	setlist          *domain.Setlist
	setlistErr       error
	receivedURL      string
	receivedID       string
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

func (m *mockSetlistfm) SetlistIDFromURL(rawURL string) (string, error) {
	m.receivedURL = rawURL
	return m.setlistID, m.setlistIDErr
}

func (m *mockSetlistfm) GetSetlist(_ context.Context, setlistID string) (*domain.Setlist, error) {
	m.receivedID = setlistID
	return m.setlist, m.setlistErr
}

type mockSpotify struct {
	token                 string
	uris                  []string
	playlistID            string
	err                   error
	receivedSetlist       domain.Setlist
	receivedToken         string
	receivedIncludeCovers bool
	createPlaylistCalled  bool
	tokenRequested        bool
	receivedTourPlaylist  bool
}

func (m *mockSpotify) GetValidToken() (string, error) {
	m.tokenRequested = true
	return m.token, m.err
}

func (m *mockSpotify) GetSetlistTracks(_ context.Context, token string, s domain.Setlist, includeCovers bool) ([]string, error) {
	m.receivedSetlist = s
	m.receivedToken = token
	m.receivedIncludeCovers = includeCovers
	return m.uris, m.err
}

func (m *mockSpotify) CreatePlaylist(_ context.Context, token string, _ domain.Setlist, _ []string, tourPlaylist bool) (string, error) {
	m.receivedToken = token
	m.receivedTourPlaylist = tourPlaylist
	m.createPlaylistCalled = true
	return m.playlistID, m.err
}

func newSvc(setlistfm *mockSetlistfm, spotify *mockSpotify) *service {
	return NewService(setlistfm, spotify)
}

func TestGetArtistSetlists_HappyPath(t *testing.T) {
	artist := domain.Artist{MBID: "abc", Name: "Sprout"}
	setlists := []domain.Setlist{{Artist: artist, Tracks: []domain.Track{{Name: "Song A"}, {Name: "Song B"}}}}
	expectedURIs := []string{"spotify:track:uri1", "spotify:track:uri2"}

	svc := newSvc(
		&mockSetlistfm{result: []domain.Artist{artist}, setlists: setlists},
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
	svc := newSvc(&mockSetlistfm{}, &mockSpotify{})

	_, err := svc.SetlistToPlaylist(context.Background(), "", false, false)
	if err == nil {
		t.Fatal("expected error for empty name, got nil")
	}
	if err.Error() != "artist name must not be empty" {
		t.Errorf("unexpected error message: %q", err.Error())
	}
}

func TestGetArtistSetlists_NoArtistsFound(t *testing.T) {
	svc := newSvc(&mockSetlistfm{result: []domain.Artist{}}, &mockSpotify{})

	_, err := svc.SetlistToPlaylist(context.Background(), "Ghost", false, false)
	if err == nil {
		t.Fatal("expected error for empty results, got nil")
	}
	if err.Error() != "Artist not found" {
		t.Errorf("unexpected error message: %q", err.Error())
	}
}

func TestGetArtistSetlists_NoSetlistsFound(t *testing.T) {
	artist := domain.Artist{MBID: "abc", Name: "Sprout"}
	svc := newSvc(
		&mockSetlistfm{result: []domain.Artist{artist}, setlists: []domain.Setlist{}},
		&mockSpotify{},
	)

	_, err := svc.SetlistToPlaylist(context.Background(), "Sprout", false, false)
	if err == nil {
		t.Fatal("expected error for empty setlists, got nil")
	}
	if err.Error() != `no setlists found for "Sprout"` {
		t.Errorf("unexpected error message: %q", err.Error())
	}
}

func TestGetArtistSetlists_SearchError(t *testing.T) {
	underlying := errors.New("network failure")
	svc := newSvc(&mockSetlistfm{searchErr: underlying}, &mockSpotify{})

	_, err := svc.SetlistToPlaylist(context.Background(), "Sprout", false, false)
	if !errors.Is(err, underlying) {
		t.Errorf("expected wrapped search error, got %v", err)
	}
}

func TestGetArtistSetlists_SetlistsError(t *testing.T) {
	underlying := errors.New("setlists unavailable")
	artist := domain.Artist{MBID: "abc", Name: "Sprout"}
	svc := newSvc(
		&mockSetlistfm{result: []domain.Artist{artist}, setlistsErr: underlying},
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
		&mockSpotify{err: underlying},
	)

	_, err := svc.SetlistToPlaylist(context.Background(), "Sprout", false, false)
	if !errors.Is(err, underlying) {
		t.Errorf("expected wrapped spotify error, got %v", err)
	}
}

func TestSetlistToPlaylistAuthed_HappyPath(t *testing.T) {
	artist := domain.Artist{MBID: "abc", Name: "Sprout"}
	setlists := []domain.Setlist{{Artist: artist, Tracks: []domain.Track{{Name: "Song A"}, {Name: "Song B"}}}}
	spotify := &mockSpotify{uris: []string{"spotify:track:uri1"}, playlistID: "playlist-authed"}

	svc := newSvc(
		&mockSetlistfm{result: []domain.Artist{artist}, setlists: setlists},
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
	svc := newSvc(&mockSetlistfm{}, &mockSpotify{})

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
		&mockSpotify{err: underlying},
	)

	_, err := svc.SetlistToPlaylistAuthed(context.Background(), "Sprout", "tok", false, false)
	if !errors.Is(err, underlying) {
		t.Errorf("expected wrapped spotify error, got %v", err)
	}
}

func TestSetlistToPlaylistAuthed_NoSongsFoundOnSpotify(t *testing.T) {
	artist := domain.Artist{MBID: "abc", Name: "Sprout"}
	setlists := []domain.Setlist{{Artist: artist, Tracks: []domain.Track{{Name: "Unreleased Song"}}}}
	spotify := &mockSpotify{uris: nil, playlistID: "p1"}

	svc := newSvc(
		&mockSetlistfm{result: []domain.Artist{artist}, setlists: setlists},
		spotify,
	)

	_, err := svc.SetlistToPlaylistAuthed(context.Background(), "sprout", "tok", false, false)
	if err == nil {
		t.Fatal("expected error when no songs are found on Spotify, got nil")
	}
	if err.Error() != `songs from setlistfm for "sprout" not found on Spotify` {
		t.Errorf("unexpected error message: %q", err.Error())
	}
	if spotify.createPlaylistCalled {
		t.Error("expected no playlist to be created")
	}
}

func TestSetlistToPlaylistAuthed_PassesIncludeCoversToSpotify(t *testing.T) {
	artist := domain.Artist{MBID: "abc", Name: "Sprout"}
	setlists := []domain.Setlist{{Artist: artist, Tracks: []domain.Track{{Name: "Song A"}}}}
	spotify := &mockSpotify{uris: []string{"spotify:track:uri1"}, playlistID: "p1"}

	svc := newSvc(
		&mockSetlistfm{result: []domain.Artist{artist}, setlists: setlists},
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

	svc := newSvc(sfm, spotify)
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

	svc := newSvc(sfm, spotify)
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

	svc := newSvc(sfm, &mockSpotify{})
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

const beatlesLink = "https://www.setlist.fm/setlist/the-beatles/1964/hollywood-bowl-hollywood-ca-63de4613.html"

func beatlesSetlist() *domain.Setlist {
	return &domain.Setlist{
		Artist: domain.Artist{MBID: "b10bbbfc", Name: "The Beatles"},
		Tracks: []domain.Track{{Name: "Twist and Shout"}, {Name: "She Loves You"}},
		Tour:   "North American Tour 1964",
	}
}

func TestSetlistURLToPlaylistAuthed_HappyPath(t *testing.T) {
	setlistfm := &mockSetlistfm{setlistID: "63de4613", setlist: beatlesSetlist()}
	spotify := &mockSpotify{uris: []string{"spotify:track:uri1"}, playlistID: "playlist-from-link"}
	svc := newSvc(setlistfm, spotify)

	playlistID, err := svc.SetlistURLToPlaylistAuthed(context.Background(), beatlesLink, "user-token", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if playlistID != "playlist-from-link" {
		t.Errorf("unexpected playlist ID: %s", playlistID)
	}
	if setlistfm.receivedURL != beatlesLink || setlistfm.receivedID != "63de4613" {
		t.Errorf("expected link parsed and setlist 63de4613 fetched, got URL %q and ID %q", setlistfm.receivedURL, setlistfm.receivedID)
	}
	if spotify.receivedSetlist.Artist.Name != "The Beatles" || len(spotify.receivedSetlist.Tracks) != 2 {
		t.Errorf("expected the fetched setlist sent to spotify, got %+v", spotify.receivedSetlist)
	}
	if spotify.receivedToken != "user-token" {
		t.Errorf("expected token %q forwarded to spotify, got %q", "user-token", spotify.receivedToken)
	}
	if spotify.receivedTourPlaylist {
		t.Error("expected a single-show playlist, got a tour playlist")
	}
}

func TestSetlistURLToPlaylistAuthed_InvalidURL(t *testing.T) {
	setlistfm := &mockSetlistfm{setlistIDErr: domain.ErrInvalidSetlistURL}
	spotify := &mockSpotify{}
	svc := newSvc(setlistfm, spotify)

	_, err := svc.SetlistURLToPlaylistAuthed(context.Background(), "https://example.com", "tok", false)
	if !errors.Is(err, domain.ErrInvalidSetlistURL) {
		t.Fatalf("expected ErrInvalidSetlistURL, got %v", err)
	}
	if err.Error() != "only setlist.fm setlist links are supported" {
		t.Errorf("unexpected error message: %q", err.Error())
	}
	if setlistfm.receivedID != "" || spotify.createPlaylistCalled {
		t.Error("expected nothing fetched and no playlist created")
	}
}

func TestSetlistURLToPlaylistAuthed_SetlistNotFound(t *testing.T) {
	spotify := &mockSpotify{}
	svc := newSvc(&mockSetlistfm{setlistID: "deadbeef"}, spotify)

	_, err := svc.SetlistURLToPlaylistAuthed(context.Background(), beatlesLink, "tok", false)
	if err == nil || err.Error() != "Setlist not found" {
		t.Fatalf("expected %q, got %v", "Setlist not found", err)
	}
	if spotify.createPlaylistCalled {
		t.Error("expected no playlist to be created")
	}
}

func TestSetlistURLToPlaylistAuthed_SetlistWithoutSongs(t *testing.T) {
	empty := beatlesSetlist()
	empty.Tracks = nil
	spotify := &mockSpotify{}
	svc := newSvc(&mockSetlistfm{setlistID: "63de4613", setlist: empty}, spotify)

	_, err := svc.SetlistURLToPlaylistAuthed(context.Background(), beatlesLink, "tok", false)
	if err == nil || err.Error() != "this setlist has no songs yet" {
		t.Fatalf("expected %q, got %v", "this setlist has no songs yet", err)
	}
	if spotify.createPlaylistCalled {
		t.Error("expected no playlist to be created")
	}
}

func TestSetlistURLToPlaylistAuthed_NoSongsFoundOnSpotify(t *testing.T) {
	spotify := &mockSpotify{uris: nil, playlistID: "p1"}
	svc := newSvc(&mockSetlistfm{setlistID: "63de4613", setlist: beatlesSetlist()}, spotify)

	_, err := svc.SetlistURLToPlaylistAuthed(context.Background(), beatlesLink, "tok", false)
	if err == nil || err.Error() != `songs from setlistfm for "The Beatles" not found on Spotify` {
		t.Fatalf("unexpected error: %v", err)
	}
	if spotify.createPlaylistCalled {
		t.Error("expected no playlist to be created")
	}
}

func TestSetlistURLToPlaylistAuthed_SetlistfmError(t *testing.T) {
	underlying := errors.New("setlistfm down")
	svc := newSvc(&mockSetlistfm{setlistID: "63de4613", setlistErr: underlying}, &mockSpotify{})

	_, err := svc.SetlistURLToPlaylistAuthed(context.Background(), beatlesLink, "tok", false)
	if !errors.Is(err, underlying) {
		t.Errorf("expected wrapped setlistfm error, got %v", err)
	}
}

func TestSetlistURLToPlaylistAuthed_PassesIncludeCoversToSpotify(t *testing.T) {
	spotify := &mockSpotify{uris: []string{"spotify:track:uri1"}, playlistID: "p1"}
	svc := newSvc(&mockSetlistfm{setlistID: "63de4613", setlist: beatlesSetlist()}, spotify)

	if _, err := svc.SetlistURLToPlaylistAuthed(context.Background(), beatlesLink, "tok", true); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !spotify.receivedIncludeCovers {
		t.Error("expected includeCovers=true to be forwarded to spotify, got false")
	}
}

func TestSetlistURLToPlaylist_UsesStoredToken(t *testing.T) {
	spotify := &mockSpotify{token: "cli-token", uris: []string{"spotify:track:uri1"}, playlistID: "p1"}
	svc := newSvc(&mockSetlistfm{setlistID: "63de4613", setlist: beatlesSetlist()}, spotify)

	if _, err := svc.SetlistURLToPlaylist(context.Background(), beatlesLink, false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if spotify.receivedToken != "cli-token" {
		t.Errorf("expected token %q forwarded to spotify, got %q", "cli-token", spotify.receivedToken)
	}
}

func TestSetlistURLToPlaylist_InvalidURLDoesNotAskForToken(t *testing.T) {
	spotify := &mockSpotify{}
	svc := newSvc(&mockSetlistfm{setlistIDErr: domain.ErrInvalidSetlistURL}, spotify)

	_, err := svc.SetlistURLToPlaylist(context.Background(), "https://example.com", false)
	if !errors.Is(err, domain.ErrInvalidSetlistURL) {
		t.Fatalf("expected ErrInvalidSetlistURL, got %v", err)
	}
	if spotify.tokenRequested {
		t.Error("expected no spotify token to be requested for an invalid link")
	}
}
