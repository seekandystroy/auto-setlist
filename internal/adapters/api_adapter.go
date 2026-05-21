package adapters

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

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
	Artist string `json:"artist"`
}

type setlistJobResponse struct {
	PlaylistURL string `json:"playlist_url"`
}

func (a *apiAdapter) handleSetlistJob(w http.ResponseWriter, r *http.Request) {
	sr := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
	w = sr
	slog.Info("request", "method", r.Method, "path", r.URL.Path, "from", r.RemoteAddr)
	defer func() { slog.Info("response", "method", r.Method, "path", r.URL.Path, "status", sr.status) }()

	token := r.Header.Get("Autosetlist-Spotify-Token")
	if token == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing Autosetlist-Spotify-Token header"})
		return
	}

	var req setlistJobRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	req.Artist = strings.TrimSpace(req.Artist)
	if req.Artist == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "artist is required"})
		return
	}
	if len(req.Artist) > 100 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "artist must be 100 characters or fewer"})
		return
	}
	id, err := a.svc.SetlistToPlaylistAuthed(req.Artist, token)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, setlistJobResponse{
		PlaylistURL: "https://open.spotify.com/playlist/" + id,
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
