package ports

import (
	"context"

	"github.com/seekandystroy/auto-setlist/internal/core/domain"
)

type Musicbrainz interface {
	GetArtist(ctx context.Context, MBID string) (*domain.Artist, error)
}
