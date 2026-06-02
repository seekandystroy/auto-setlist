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

	spotifyAdapter, err := adapters.NewSpotifyAdapter(clientID, clientSecret, adapters.NewSpotifyCallbackAdapter())
	if err != nil {
		slog.Error(err.Error())
		os.Exit(1)
	}

	svc := service.NewService(
		adapters.NewSetlistfmAdapter(apiKey),
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
