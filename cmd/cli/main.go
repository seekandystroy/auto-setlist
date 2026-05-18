package main

import (
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/seekandystroy/auto-setlist/internal/adapters"
	"github.com/seekandystroy/auto-setlist/internal/core/service"
)

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})))

	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: auto-setlist <artist name>")
		os.Exit(1)
	}
	artistName := strings.Join(os.Args[1:], " ")

	apiKey := os.Getenv("SETLISTFM_API_KEY")
	if apiKey == "" {
		slog.Error("SETLISTFM_API_KEY environment variable is not set")
		os.Exit(1)
	}

	clientID := os.Getenv("SPOTIFY_CLIENT_ID")
	clientSecret := os.Getenv("SPOTIFY_CLIENT_SECRET")
	if clientID == "" || clientSecret == "" {
		slog.Error("SPOTIFY_CLIENT_ID and SPOTIFY_CLIENT_SECRET must be set")
		os.Exit(1)
	}

	spotifyAdapter, err := adapters.NewSpotifyAdapter(clientID, clientSecret, adapters.NewSpotifyCallbackAdapter())
	if err != nil {
		slog.Error(err.Error())
		os.Exit(1)
	}

	svc := service.NewService(
		adapters.NewSetlistfmAdapter(apiKey),
		spotifyAdapter,
		adapters.NewMusicbrainzAdapter(),
	)

	playlistID, err := svc.SetlistToPlaylist(artistName)
	if err != nil {
		slog.Error(err.Error())
		os.Exit(1)
	}
	fmt.Printf("Playlist created: https://open.spotify.com/playlist/%s\n", playlistID)
}
