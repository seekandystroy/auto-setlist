package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/seekandystroy/auto-setlist/internal/adapters"
	"github.com/seekandystroy/auto-setlist/internal/core/service"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: auto-setlist <artist name>")
		os.Exit(1)
	}
	artistName := strings.Join(os.Args[1:], " ")

	apiKey := os.Getenv("SETLISTFM_API_KEY")
	if apiKey == "" {
		fmt.Fprintln(os.Stderr, "error: SETLISTFM_API_KEY environment variable is not set")
		os.Exit(1)
	}

	clientID := os.Getenv("SPOTIFY_CLIENT_ID")
	clientSecret := os.Getenv("SPOTIFY_CLIENT_SECRET")
	if clientID == "" || clientSecret == "" {
		fmt.Fprintln(os.Stderr, "error: SPOTIFY_CLIENT_ID and SPOTIFY_CLIENT_SECRET must be set")
		os.Exit(1)
	}

	spotifyAdapter, err := adapters.NewSpotifyAdapter(clientID, clientSecret, adapters.NewSpotifyCallbackAdapter())
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	svc := service.NewService(
		adapters.NewSetlistfmAdapter(apiKey),
		spotifyAdapter,
		adapters.NewMusicbrainzAdapter(),
	)

	playlistID, uris, err := svc.SetlistToPlaylist(artistName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("playlist: %s\ntracks: %+v\n", playlistID, uris)
}
