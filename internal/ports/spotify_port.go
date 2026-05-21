package ports

import (
	"context"

	"github.com/seekandystroy/auto-setlist/internal/core/domain"
)

type Spotify interface {
	GetValidToken() (string, error)
	GetSetlistTracks(token string, setlist domain.Setlist) ([]string, error)
	CreatePlaylist(token string, setlist domain.Setlist, uris []string) (string, error)
}

type SpotifyCallbackReceiver interface {
	WaitForCode(ctx context.Context, expectedState string) (string, error)
}
