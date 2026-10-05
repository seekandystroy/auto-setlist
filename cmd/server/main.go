package main

import (
	"log/slog"
	"mime"
	"net/http"
	"os"

	"github.com/seekandystroy/auto-setlist/internal/adapters"
	"github.com/seekandystroy/auto-setlist/internal/core/service"
)

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})))

	apiKey := os.Getenv("SETLISTFM_API_KEY")
	if apiKey == "" {
		slog.Error("SETLISTFM_API_KEY is not set")
		os.Exit(1)
	}
	clientID := os.Getenv("SPOTIFY_CLIENT_ID")
	clientSecret := os.Getenv("SPOTIFY_CLIENT_SECRET")
	if clientID == "" || clientSecret == "" {
		slog.Error("SPOTIFY_CLIENT_ID and SPOTIFY_CLIENT_SECRET must be set")
		os.Exit(1)
	}

	// Optional API roots, for pointing the app at fakes (see acceptance/); default to the real APIs.
	var setlistfmOpts []adapters.SetlistfmOption
	if baseURL := os.Getenv("SETLISTFM_BASE_URL"); baseURL != "" {
		setlistfmOpts = append(setlistfmOpts, adapters.WithSetlistfmBaseURL(baseURL))
	}
	var spotifyOpts []adapters.SpotifyOption
	if baseURL := os.Getenv("SPOTIFY_API_BASE_URL"); baseURL != "" {
		spotifyOpts = append(spotifyOpts, adapters.WithSpotifyAPIBaseURL(baseURL))
	}

	spotifyAdapter, err := adapters.NewSpotifyAdapter(clientID, clientSecret, adapters.NewSpotifyCallbackAdapter(), spotifyOpts...)
	if err != nil {
		slog.Error(err.Error())
		os.Exit(1)
	}

	svc := service.NewService(
		adapters.NewSetlistfmAdapter(apiKey, setlistfmOpts...),
		spotifyAdapter,
	)

	mime.AddExtensionType(".js", "application/javascript")

	mux := http.NewServeMux()
	mux.Handle("POST /setlistjob", adapters.NewAPIAdapter(svc))
	mux.Handle("/", http.FileServer(http.Dir("static")))

	port := getEnvWithFallback("PORT", "3000")
	slog.Info("server starting", "port", port)
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		slog.Error(err.Error())
		os.Exit(1)
	}
}

func getEnvWithFallback(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}
