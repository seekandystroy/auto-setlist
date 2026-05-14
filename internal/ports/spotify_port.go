package ports

import (
	"context"

	"github.com/seekandystroy/auto-setlist/internal/core/domain"
)

type Spotify interface {
	GetValidToken() (string, error)
	GetSetlistTracks(domain.Setlist) ([]string, error)
}

type SpotifyCallbackReceiver interface {
	WaitForCode(ctx context.Context, expectedState string) (string, error)
}
