package fakes

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

// notRegisteredMessage is the undocumented plain-text body Spotify sends, with a 403, for
// users who aren't on a development-mode app's allowlist.
const notRegisteredMessage = "The user is not registered for this application. Please check your settings on https://developer.spotify.com/dashboard."

const (
	accessTokenLifetime = time.Hour
	playlistModifyScope = "playlist-modify-public playlist-modify-private"
	maxItemsPerAdd      = 100
)

// Spotify fakes the Spotify Web API (https://developer.spotify.com/documentation/web-api)
// and the Accounts service's authorization code with PKCE flow.
// Mount API() at the Web API root (where "/v1" would be) and Accounts() at the accounts root.
type Spotify struct {
	mu        sync.Mutex
	users     map[string]SpotifyUser
	loginAs   string
	catalog   []Track
	tokens    map[string]issuedToken // access token -> owner
	refreshes map[string]string      // refresh token -> user ID
	codes     map[string]*issuedCode // authorization code -> pending grant
	playlists []*Playlist
}

// Playlist is a playlist created through the fake, as recorded for test assertions.
type Playlist struct {
	ID          string  `json:"id"`
	Owner       string  `json:"owner"`
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Tracks      []Track `json:"tracks"`
}

type issuedToken struct {
	userID    string
	scope     string
	expiresAt time.Time // zero means it never expires
}

type issuedCode struct {
	userID        string
	clientID      string
	redirectURI   string
	codeChallenge string
	scope         string
	used          bool
}

func NewSpotify() *Spotify {
	f := &Spotify{}
	f.Load(SpotifyScenario{})
	return f
}

// Load replaces all users, tokens, catalog and playlists with the given scenario.
func (f *Spotify) Load(s SpotifyScenario) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.users = make(map[string]SpotifyUser)
	f.tokens = make(map[string]issuedToken)
	f.refreshes = make(map[string]string)
	f.codes = make(map[string]*issuedCode)
	f.playlists = nil
	f.loginAs = s.LoginAs
	for i, u := range s.Users {
		f.users[u.ID] = u
		if f.loginAs == "" && i == 0 {
			f.loginAs = u.ID
		}
		if u.AccessToken != "" {
			f.tokens[u.AccessToken] = issuedToken{userID: u.ID, scope: playlistModifyScope}
		}
		if u.RefreshToken != "" {
			f.refreshes[u.RefreshToken] = u.ID
		}
	}
	f.catalog = make([]Track, len(s.Catalog))
	for i, t := range s.Catalog {
		if t.URI == "" {
			t.URI = "spotify:track:" + base62ID("track:"+t.Name+":"+strings.Join(t.Artists, ","))
		}
		f.catalog[i] = t
	}
}

// Playlists returns a snapshot of every playlist created so far, oldest first.
func (f *Spotify) Playlists() []Playlist {
	f.mu.Lock()
	defer f.mu.Unlock()
	result := make([]Playlist, len(f.playlists))
	for i, p := range f.playlists {
		result[i] = *p
		result[i].Tracks = append([]Track{}, p.Tracks...)
	}
	return result
}

// --- Web API ---

func (f *Spotify) API() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /search", f.withUser(f.search))
	mux.HandleFunc("POST /me/playlists", f.withUser(f.createPlaylist))
	mux.HandleFunc("POST /playlists/{id}/items", f.withUser(f.addItems))
	return mux
}

type apiHandler func(w http.ResponseWriter, r *http.Request, user SpotifyUser, token issuedToken)

func (f *Spotify) withUser(next apiHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth == "" {
			writeSpotifyError(w, http.StatusUnauthorized, "No token provided")
			return
		}
		bearer, ok := strings.CutPrefix(auth, "Bearer ")
		if !ok {
			writeSpotifyError(w, http.StatusBadRequest, "Only valid bearer authentication supported")
			return
		}

		f.mu.Lock()
		token, known := f.tokens[bearer]
		user := f.users[token.userID]
		f.mu.Unlock()

		switch {
		case !known:
			writeSpotifyError(w, http.StatusUnauthorized, "Invalid access token")
		case !token.expiresAt.IsZero() && time.Now().After(token.expiresAt):
			writeSpotifyError(w, http.StatusUnauthorized, "The access token expired")
		case user.NotAllowlisted:
			w.Header().Set("Content-Type", "text/plain")
			w.WriteHeader(http.StatusForbidden)
			w.Write([]byte(notRegisteredMessage))
		default:
			next(w, r, user, token)
		}
	}
}

func (f *Spotify) search(w http.ResponseWriter, r *http.Request, _ SpotifyUser, _ issuedToken) {
	q := r.URL.Query()
	if q.Get("q") == "" {
		writeSpotifyError(w, http.StatusBadRequest, "No search query")
		return
	}
	if !slices.Contains(strings.Split(q.Get("type"), ","), "track") {
		writeSpotifyError(w, http.StatusBadRequest, "Missing parameter type")
		return
	}
	limit, err := intParam(q, "limit", 20)
	if err != nil || limit < 1 || limit > 50 {
		writeSpotifyError(w, http.StatusBadRequest, "Invalid limit")
		return
	}
	offset, err := intParam(q, "offset", 0)
	if err != nil || offset < 0 {
		writeSpotifyError(w, http.StatusBadRequest, "Invalid offset")
		return
	}

	trackFilter, artistFilter := parseSearchQuery(q.Get("q"))
	f.mu.Lock()
	var matches []Track
	for _, t := range f.catalog {
		if matchesSearch(t, trackFilter, artistFilter) {
			matches = append(matches, t)
		}
	}
	f.mu.Unlock()

	items := []map[string]any{}
	if offset < len(matches) {
		for _, t := range matches[offset:min(offset+limit, len(matches))] {
			items = append(items, trackObject(t))
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"tracks": map[string]any{
			"href":     "https://api.spotify.com/v1/search?" + q.Encode(),
			"limit":    limit,
			"next":     nil,
			"offset":   offset,
			"previous": nil,
			"total":    len(matches),
			"items":    items,
		},
	})
}

// parseSearchQuery extracts the "track:" and "artist:" field filters from a search query.
// Real Spotify search is fuzzy and ranked; this fake matches case-insensitive substrings,
// which is enough to exercise the app's own matching of results.
func parseSearchQuery(q string) (track, artist string) {
	lower := strings.ToLower(q)
	trackIdx := strings.Index(lower, "track:")
	artistIdx := strings.Index(lower, "artist:")
	switch {
	case trackIdx >= 0 && artistIdx > trackIdx:
		track = q[trackIdx+len("track:") : artistIdx]
		artist = q[artistIdx+len("artist:"):]
	case artistIdx >= 0 && trackIdx > artistIdx:
		artist = q[artistIdx+len("artist:") : trackIdx]
		track = q[trackIdx+len("track:"):]
	case trackIdx >= 0:
		track = q[trackIdx+len("track:"):]
	case artistIdx >= 0:
		artist = q[artistIdx+len("artist:"):]
	default:
		track = q
	}
	return strings.ToLower(strings.TrimSpace(track)), strings.ToLower(strings.TrimSpace(artist))
}

func matchesSearch(t Track, track, artist string) bool {
	if track != "" && !strings.Contains(strings.ToLower(t.Name), track) {
		return false
	}
	if artist == "" {
		return true
	}
	return slices.ContainsFunc(t.Artists, func(a string) bool {
		return strings.Contains(strings.ToLower(a), artist)
	})
}

func (f *Spotify) createPlaylist(w http.ResponseWriter, r *http.Request, user SpotifyUser, token issuedToken) {
	if !strings.Contains(token.scope, "playlist-modify") {
		writeSpotifyError(w, http.StatusForbidden, "Insufficient client scope")
		return
	}
	var body struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Public      *bool  `json:"public"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeSpotifyError(w, http.StatusBadRequest, "Error parsing JSON.")
		return
	}
	if body.Name == "" {
		writeSpotifyError(w, http.StatusBadRequest, "Missing required field: name")
		return
	}
	public := body.Public == nil || *body.Public

	f.mu.Lock()
	p := &Playlist{
		ID:          base62ID(fmt.Sprintf("playlist:%d:%s", len(f.playlists), body.Name)),
		Owner:       user.ID,
		Name:        body.Name,
		Description: body.Description,
	}
	f.playlists = append(f.playlists, p)
	f.mu.Unlock()

	writeJSON(w, http.StatusCreated, map[string]any{
		"collaborative": false,
		"description":   p.Description,
		"external_urls": map[string]string{"spotify": "https://open.spotify.com/playlist/" + p.ID},
		"href":          "https://api.spotify.com/v1/playlists/" + p.ID,
		"id":            p.ID,
		"images":        []any{},
		"name":          p.Name,
		"owner": map[string]any{
			"display_name":  user.ID,
			"external_urls": map[string]string{"spotify": "https://open.spotify.com/user/" + user.ID},
			"href":          "https://api.spotify.com/v1/users/" + user.ID,
			"id":            user.ID,
			"type":          "user",
			"uri":           "spotify:user:" + user.ID,
		},
		"public":      public,
		"snapshot_id": base62ID("snapshot:" + p.ID + ":0"),
		"items":       map[string]any{"href": "https://api.spotify.com/v1/playlists/" + p.ID + "/items", "total": 0},
		"type":        "playlist",
		"uri":         "spotify:playlist:" + p.ID,
	})
}

func (f *Spotify) addItems(w http.ResponseWriter, r *http.Request, user SpotifyUser, _ issuedToken) {
	var body struct {
		URIs     []string `json:"uris"`
		Position *int     `json:"position"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeSpotifyError(w, http.StatusBadRequest, "Error parsing JSON.")
		return
	}
	if len(body.URIs) == 0 {
		writeSpotifyError(w, http.StatusBadRequest, "No uris provided")
		return
	}
	if len(body.URIs) > maxItemsPerAdd {
		writeSpotifyError(w, http.StatusBadRequest, "Too many ids requested")
		return
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	idx := slices.IndexFunc(f.playlists, func(p *Playlist) bool { return p.ID == r.PathValue("id") })
	if idx < 0 {
		writeSpotifyError(w, http.StatusNotFound, "Not found.")
		return
	}
	p := f.playlists[idx]
	if p.Owner != user.ID {
		writeSpotifyError(w, http.StatusForbidden, "You cannot add tracks to a playlist you don't own.")
		return
	}

	added := make([]Track, 0, len(body.URIs))
	for _, uri := range body.URIs {
		i := slices.IndexFunc(f.catalog, func(t Track) bool { return t.URI == uri })
		if i < 0 {
			writeSpotifyError(w, http.StatusBadRequest, "Invalid base62 id")
			return
		}
		added = append(added, f.catalog[i])
	}
	pos := len(p.Tracks)
	if body.Position != nil {
		if *body.Position < 0 || *body.Position > len(p.Tracks) {
			writeSpotifyError(w, http.StatusBadRequest, "Index out of bounds")
			return
		}
		pos = *body.Position
	}
	p.Tracks = slices.Insert(p.Tracks, pos, added...)

	writeJSON(w, http.StatusCreated, map[string]string{
		"snapshot_id": base62ID(fmt.Sprintf("snapshot:%s:%d", p.ID, len(p.Tracks))),
	})
}

func trackObject(t Track) map[string]any {
	id := strings.TrimPrefix(t.URI, "spotify:track:")
	artists := make([]map[string]any, len(t.Artists))
	for i, name := range t.Artists {
		artistID := base62ID("artist:" + name)
		artists[i] = map[string]any{
			"external_urls": map[string]string{"spotify": "https://open.spotify.com/artist/" + artistID},
			"href":          "https://api.spotify.com/v1/artists/" + artistID,
			"id":            artistID,
			"name":          name,
			"type":          "artist",
			"uri":           "spotify:artist:" + artistID,
		}
	}
	albumID := base62ID("album:" + t.Name)
	return map[string]any{
		"album": map[string]any{
			"album_type":             "album",
			"artists":                artists,
			"external_urls":          map[string]string{"spotify": "https://open.spotify.com/album/" + albumID},
			"href":                   "https://api.spotify.com/v1/albums/" + albumID,
			"id":                     albumID,
			"images":                 []any{},
			"name":                   t.Name,
			"release_date":           "2020-01-01",
			"release_date_precision": "day",
			"total_tracks":           10,
			"type":                   "album",
			"uri":                    "spotify:album:" + albumID,
		},
		"artists":       artists,
		"disc_number":   1,
		"duration_ms":   240000,
		"explicit":      false,
		"external_ids":  map[string]string{"isrc": "PTXX" + strings.ToUpper(id[:8])},
		"external_urls": map[string]string{"spotify": "https://open.spotify.com/track/" + id},
		"href":          "https://api.spotify.com/v1/tracks/" + id,
		"id":            id,
		"is_local":      false,
		"name":          t.Name,
		"popularity":    50,
		"track_number":  1,
		"type":          "track",
		"uri":           t.URI,
	}
}

func writeSpotifyError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]any{
		"error": map[string]any{"status": status, "message": message},
	})
}

func intParam(q url.Values, name string, fallback int) (int, error) {
	v := q.Get(name)
	if v == "" {
		return fallback, nil
	}
	return strconv.Atoi(v)
}

// --- Accounts service ---

func (f *Spotify) Accounts() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /authorize", f.authorize)
	mux.HandleFunc("POST /api/token", f.token)
	mux.HandleFunc("OPTIONS /api/token", f.token)
	return mux
}

var consentPage = template.Must(template.New("consent").Parse(`<!DOCTYPE html>
<html lang="en">
<head><meta charset="UTF-8"><title>Authorize - Spotify (fake)</title></head>
<body>
  <h1>auto-setlist would like to access your Spotify account</h1>
  <p>Logged in as {{.User}}</p>
  <p>Scopes: {{.Scope}}</p>
  <a href="{{.CancelURL}}">Cancel</a>
  <a href="{{.AgreeURL}}">Agree</a>
</body>
</html>`))

// authorize renders a consent page for the scenario's logged-in user, standing in for
// Spotify's login and consent screens. "Agree" and "Cancel" redirect back like the real flow.
func (f *Spotify) authorize(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	redirectURI := q.Get("redirect_uri")
	switch {
	case q.Get("client_id") == "":
		http.Error(w, "INVALID_CLIENT: Invalid client", http.StatusBadRequest)
		return
	case redirectURI == "":
		http.Error(w, "INVALID_CLIENT: Invalid redirect URI", http.StatusBadRequest)
		return
	case q.Get("response_type") != "code":
		http.Error(w, "unsupported_response_type", http.StatusBadRequest)
		return
	case q.Get("code_challenge_method") != "S256" || q.Get("code_challenge") == "":
		http.Error(w, "invalid_request: code_challenge required with S256", http.StatusBadRequest)
		return
	}

	f.mu.Lock()
	userID := f.loginAs
	_, ok := f.users[userID]
	code := randomToken()
	if ok {
		f.codes[code] = &issuedCode{
			userID:        userID,
			clientID:      q.Get("client_id"),
			redirectURI:   redirectURI,
			codeChallenge: q.Get("code_challenge"),
			scope:         q.Get("scope"),
		}
	}
	f.mu.Unlock()
	if !ok {
		http.Error(w, "fake spotify: scenario has no user to log in as", http.StatusInternalServerError)
		return
	}

	back := func(params url.Values) string {
		if state := q.Get("state"); state != "" {
			params.Set("state", state)
		}
		u, _ := url.Parse(redirectURI)
		u.RawQuery = params.Encode()
		return u.String()
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	consentPage.Execute(w, map[string]any{
		"User":      userID,
		"Scope":     q.Get("scope"),
		"AgreeURL":  template.URL(back(url.Values{"code": {code}})),
		"CancelURL": template.URL(back(url.Values{"error": {"access_denied"}})),
	})
}

func (f *Spotify) token(w http.ResponseWriter, r *http.Request) {
	// PKCE token requests come straight from the browser.
	w.Header().Set("Access-Control-Allow-Origin", "*")
	if r.Method == http.MethodOptions {
		w.Header().Set("Access-Control-Allow-Methods", "POST")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if err := r.ParseForm(); err != nil {
		writeOAuthError(w, "invalid_request", "Invalid form body")
		return
	}

	switch r.PostForm.Get("grant_type") {
	case "authorization_code":
		f.exchangeCode(w, r.PostForm)
	case "refresh_token":
		f.refresh(w, r.PostForm)
	default:
		writeOAuthError(w, "unsupported_grant_type", "grant_type parameter is missing")
	}
}

func (f *Spotify) exchangeCode(w http.ResponseWriter, form url.Values) {
	f.mu.Lock()
	defer f.mu.Unlock()

	grant, ok := f.codes[form.Get("code")]
	switch {
	case !ok || grant.used:
		writeOAuthError(w, "invalid_grant", "Invalid authorization code")
		return
	case form.Get("client_id") != grant.clientID:
		writeOAuthError(w, "invalid_client", "Invalid client")
		return
	case form.Get("redirect_uri") != grant.redirectURI:
		writeOAuthError(w, "invalid_grant", "Invalid redirect URI")
		return
	case pkceChallenge(form.Get("code_verifier")) != grant.codeChallenge:
		writeOAuthError(w, "invalid_grant", "code_verifier was incorrect")
		return
	}
	grant.used = true
	f.issueTokens(w, grant.userID, grant.scope)
}

func (f *Spotify) refresh(w http.ResponseWriter, form url.Values) {
	f.mu.Lock()
	defer f.mu.Unlock()

	userID, ok := f.refreshes[form.Get("refresh_token")]
	if !ok {
		writeOAuthError(w, "invalid_grant", "Invalid refresh token")
		return
	}
	// With PKCE, Spotify rotates refresh tokens.
	delete(f.refreshes, form.Get("refresh_token"))
	f.issueTokens(w, userID, playlistModifyScope)
}

// issueTokens must be called with f.mu held.
func (f *Spotify) issueTokens(w http.ResponseWriter, userID, scope string) {
	access, refresh := randomToken(), randomToken()
	f.tokens[access] = issuedToken{userID: userID, scope: scope, expiresAt: time.Now().Add(accessTokenLifetime)}
	f.refreshes[refresh] = userID
	writeJSON(w, http.StatusOK, map[string]any{
		"access_token":  access,
		"token_type":    "Bearer",
		"scope":         scope,
		"expires_in":    int(accessTokenLifetime.Seconds()),
		"refresh_token": refresh,
	})
}

func writeOAuthError(w http.ResponseWriter, code, description string) {
	writeJSON(w, http.StatusBadRequest, map[string]string{"error": code, "error_description": description})
}

func pkceChallenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func randomToken() string {
	b := make([]byte, 24)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// base62ID derives a stable 22-character Spotify-style ID from a seed.
func base62ID(seed string) string {
	const alphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	sum := sha256.Sum256([]byte(seed))
	id := make([]byte, 22)
	for i := range id {
		id[i] = alphabet[int(sum[i])%len(alphabet)]
	}
	return string(id)
}
