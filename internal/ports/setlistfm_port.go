package ports

import (
	"context"

	"github.com/seekandystroy/auto-setlist/internal/core/domain"
)

type Setlistfm interface {
	SearchArtists(ctx context.Context, name string) ([]domain.Artist, error)
	GetSetlists(ctx context.Context, artist domain.Artist) ([]domain.Setlist, error)
	GetSetlistsForTour(ctx context.Context, artist domain.Artist, tourName string) ([]domain.Setlist, error)
	// SetlistIDFromURL returns the ID of the setlist a setlist.fm link points to,
	// or domain.ErrInvalidSetlistURL if it isn't a setlist link.
	SetlistIDFromURL(rawURL string) (string, error)
	// GetSetlist returns nil, without error, when setlist.fm has no setlist with that ID.
	GetSetlist(ctx context.Context, setlistID string) (*domain.Setlist, error)
}
