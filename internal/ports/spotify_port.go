package ports

import "context"

type Spotify interface {
	GetValidToken() (string, error)
}

type SpotifyCallbackReceiver interface {
	WaitForCode(ctx context.Context, expectedState string) (string, error)
}
