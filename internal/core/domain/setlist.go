package domain

type Track struct {
	Name              string
	CoveredArtistName string
}

type Setlist struct {
	Artist Artist
	Tracks []Track
}
