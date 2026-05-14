package ports

import "github.com/seekandystroy/auto-setlist/internal/core/domain"

type SetlistFm interface {
	SearchArtists(name string) ([]domain.Artist, error)
	GetSetlists(artist domain.Artist) ([]domain.Setlist, error)
}
