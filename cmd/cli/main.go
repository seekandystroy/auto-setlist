package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/seekandystroy/auto-setlist/internal/adapters"
	"github.com/seekandystroy/auto-setlist/internal/core/service"
)

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})))

	var includeCovers bool
	var tourPlaylist bool
	flag.BoolVar(&includeCovers, "include-covers", false, "include cover songs from the original artist when searching Spotify")
	flag.BoolVar(&includeCovers, "ic", false, "shorthand for --include-covers")
	flag.BoolVar(&tourPlaylist, "tour-playlist", false, "build a playlist from all songs played across the latest tour")
	flag.BoolVar(&tourPlaylist, "tp", false, "shorthand for --tour-playlist")
	flag.Parse()

	if flag.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "usage: auto-setlist [--include-covers] <artist name>")
		os.Exit(1)
	}
	artistName := strings.Join(flag.Args(), " ")

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
	)

	playlistID, err := svc.SetlistToPlaylist(context.Background(), artistName, includeCovers, tourPlaylist)
	if err != nil {
		slog.Error(err.Error())
		os.Exit(1)
	}
	fmt.Printf("Playlist created: https://open.spotify.com/playlist/%s\n", playlistID)
}
