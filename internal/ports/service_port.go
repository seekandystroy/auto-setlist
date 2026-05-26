package ports

import "context"

type SetlistService interface {
	SetlistToPlaylist(ctx context.Context, artist string, includeCovers bool) (string, error)
	SetlistToPlaylistAuthed(ctx context.Context, artist, spotifyAccessToken string, includeCovers bool) (string, error)
}
