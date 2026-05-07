package ports

import (
	"context"

	"github.com/seekandystroy/auto-setlist/internal/core/domain"
)

type Spotify interface {
	GetValidToken() (*domain.SpotifyToken, error)
}

type SpotifyCallbackReceiver interface {
	WaitForCode(ctx context.Context, expectedState string) (string, error)
}
