package ports

import "context"

type SetlistService interface {
	SetlistToPlaylist(ctx context.Context, artist string) (string, error)
	SetlistToPlaylistAuthed(ctx context.Context, artist, spotifyAccessToken string) (string, error)
}
