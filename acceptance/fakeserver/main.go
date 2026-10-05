// Command fakeserver serves in-memory fakes of Setlist.fm and Spotify for the acceptance tests.
// The real web app (cmd/server) is pointed at it through SETLISTFM_BASE_URL and
// SPOTIFY_API_BASE_URL; the browser reaches the fake accounts service through a Playwright route.
//
// Routes:
//
//	/setlistfm/                         fake Setlist.fm REST API 1.0
//	/spotify-api/v1/                    fake Spotify Web API
//	/spotify-accounts/                  fake Spotify Accounts (authorize + token)
//	POST /control/scenario              replace all fake state with a fakes.Scenario (JSON)
//	GET  /control/spotify/playlists     playlists created so far (JSON)
//
// Usage:
//
//	PORT=3101 go run ./acceptance/fakeserver
package main

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"os"

	"github.com/seekandystroy/auto-setlist/acceptance/fakes"
)

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})))

	port := os.Getenv("PORT")
	if port == "" {
		port = "3101"
	}

	setlistfm := fakes.NewSetlistfm()
	spotify := fakes.NewSpotify()

	mux := http.NewServeMux()
	mux.Handle("/setlistfm/", http.StripPrefix("/setlistfm", setlistfm))
	mux.Handle("/spotify-api/v1/", http.StripPrefix("/spotify-api/v1", spotify.API()))
	mux.Handle("/spotify-accounts/", http.StripPrefix("/spotify-accounts", spotify.Accounts()))

	mux.HandleFunc("POST /control/scenario", func(w http.ResponseWriter, r *http.Request) {
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()
		var s fakes.Scenario
		if err := dec.Decode(&s); err != nil {
			http.Error(w, "invalid scenario: "+err.Error(), http.StatusBadRequest)
			return
		}
		setlistfm.Load(s.Setlistfm)
		spotify.Load(s.Spotify)
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("GET /control/spotify/playlists", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(spotify.Playlists())
	})
	mux.HandleFunc("GET /control/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	slog.Info("fake server starting", "port", port)
	if err := http.ListenAndServe("127.0.0.1:"+port, mux); err != nil {
		slog.Error(err.Error())
		os.Exit(1)
	}
}
