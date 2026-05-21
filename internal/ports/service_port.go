package ports

type SetlistService interface {
	SetlistToPlaylist(artist string) (string, error)
	SetlistToPlaylistAuthed(artist, spotifyAccessToken string) (string, error)
}
