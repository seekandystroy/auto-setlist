package adapters

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	applog "github.com/seekandystroy/auto-setlist/internal"
	"github.com/seekandystroy/auto-setlist/internal/ports"
)

type apiAdapter struct {
	svc ports.SetlistService
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (sr *statusRecorder) WriteHeader(status int) {
	sr.status = status
	sr.ResponseWriter.WriteHeader(status)
}

func NewAPIAdapter(svc ports.SetlistService) http.Handler {
	a := &apiAdapter{svc: svc}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /setlistjob", a.handleSetlistJob)
	return mux
}

type setlistJobRequest struct {
	Artist        string `json:"artist"`
	IncludeCovers bool   `json:"include_covers"`
}

type setlistJobResponse struct {
	PlaylistURL string `json:"playlist_url"`
}

func (a *apiAdapter) handleSetlistJob(w http.ResponseWriter, r *http.Request) {
	sr := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
	w = sr

	reqID := applog.GenerateRequestID()
	clientIP := applog.GetClientIP(r)
	token := r.Header.Get("Autosetlist-Spotify-Token")

	logger := slog.Default().With(
		"request_id", reqID,
		"spotify_token", applog.TruncateToken(token),
		"ip", clientIP,
	)
	ctx := applog.ContextWithLogger(r.Context(), logger)

	logger.Info("request", "method", r.Method, "path", r.URL.Path)
	var detail string
	defer func() { logger.Info("response", "method", r.Method, "path", r.URL.Path, "status", sr.status, "detail", detail) }()

	if token == "" {
		detail = "missing Autosetlist-Spotify-Token header"
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": detail})
		return
	}

	var req setlistJobRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		detail = "invalid JSON"
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": detail})
		return
	}
	req.Artist = strings.TrimSpace(req.Artist)
	if req.Artist == "" {
		detail = "artist is required"
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": detail})
		return
	}
	if len(req.Artist) > 100 {
		detail = "artist must be 100 characters or fewer"
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": detail})
		return
	}
	id, err := a.svc.SetlistToPlaylistAuthed(ctx, req.Artist, token, req.IncludeCovers)
	if err != nil {
		detail = err.Error()
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": detail})
		return
	}
	playlistURL := "https://open.spotify.com/playlist/" + id
	detail = playlistURL
	writeJSON(w, http.StatusOK, setlistJobResponse{
		PlaylistURL: playlistURL,
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
