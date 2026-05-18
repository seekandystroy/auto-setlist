package ports

type SetlistService interface {
	SetlistToPlaylist(artist string) (string, error)
}
