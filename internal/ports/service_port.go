package ports

import "context"

type SetlistService interface {
	SetlistToPlaylist(ctx context.Context, artist string, includeCovers, tourPlaylist bool) (string, error)
	SetlistToPlaylistAuthed(ctx context.Context, artist, spotifyAccessToken string, includeCovers, tourPlaylist bool) (string, error)
}
