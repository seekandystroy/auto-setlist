package ports

import "context"

type SetlistService interface {
	SetlistToPlaylist(ctx context.Context, artist string, includeCovers, tourPlaylist bool) (string, error)
	SetlistToPlaylistAuthed(ctx context.Context, artist, spotifyAccessToken string, includeCovers, tourPlaylist bool) (string, error)
	SetlistURLToPlaylist(ctx context.Context, setlistURL string, includeCovers bool) (string, error)
	SetlistURLToPlaylistAuthed(ctx context.Context, setlistURL, spotifyAccessToken string, includeCovers bool) (string, error)
}
