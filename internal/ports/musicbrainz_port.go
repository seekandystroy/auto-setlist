package ports

import "github.com/seekandystroy/auto-setlist/internal/core/domain"

type Musicbrainz interface {
	GetArtist(MBID string) (*domain.Artist, error)
}
