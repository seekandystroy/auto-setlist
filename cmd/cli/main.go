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
	var setlistURL string
	flag.BoolVar(&includeCovers, "include-covers", false, "include cover songs from the original artist when searching Spotify")
	flag.BoolVar(&includeCovers, "ic", false, "shorthand for --include-covers")
	flag.BoolVar(&tourPlaylist, "tour-playlist", false, "build a playlist from all songs played across the latest tour")
	flag.BoolVar(&tourPlaylist, "tp", false, "shorthand for --tour-playlist")
	flag.StringVar(&setlistURL, "url", "", "build a playlist from a specific setlist.fm setlist link")
	flag.StringVar(&setlistURL, "u", "", "shorthand for --url")
	flag.Parse()

	artistName := strings.Join(flag.Args(), " ")
	switch {
	case setlistURL != "" && artistName != "":
		fmt.Fprintln(os.Stderr, "use either an artist or --url, not both")
		os.Exit(1)
	case setlistURL != "" && tourPlaylist:
		fmt.Fprintln(os.Stderr, "--tour-playlist can't be used with --url")
		os.Exit(1)
	case setlistURL == "" && artistName == "":
		fmt.Fprintln(os.Stderr, "usage: auto-setlist [--include-covers|-ic] [--tour-playlist|-tp] <artist name>")
		fmt.Fprintln(os.Stderr, "       auto-setlist [--include-covers|-ic] --url|-u <setlist.fm link>")
		os.Exit(1)
	}

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

	var playlistID string
	if setlistURL != "" {
		playlistID, err = svc.SetlistURLToPlaylist(context.Background(), setlistURL, includeCovers)
	} else {
		playlistID, err = svc.SetlistToPlaylist(context.Background(), artistName, includeCovers, tourPlaylist)
	}
	if err != nil {
		slog.Error(err.Error())
		os.Exit(1)
	}
	fmt.Printf("Playlist created: https://open.spotify.com/playlist/%s\n", playlistID)
}
