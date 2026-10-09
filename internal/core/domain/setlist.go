package domain

import "errors"

// ErrInvalidSetlistURL is returned for a link that doesn't point to a setlist on setlist.fm.
var ErrInvalidSetlistURL = errors.New("only setlist.fm setlist links are supported")

type Track struct {
	Name              string
	CoveredArtistName string
}

type Setlist struct {
	Artist Artist
	Tracks []Track
	Tour   string
}
