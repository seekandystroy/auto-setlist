package ports

import (
	"context"

	"github.com/seekandystroy/auto-setlist/internal/core/domain"
)

type Setlistfm interface {
	SearchArtists(ctx context.Context, name string) ([]domain.Artist, error)
	GetSetlists(ctx context.Context, artist domain.Artist) ([]domain.Setlist, error)
	GetSetlistsForTour(ctx context.Context, artist domain.Artist, tourName string) ([]domain.Setlist, error)
}
