package fakes

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	setlistfmArtistsPerPage  = 30
	setlistfmSetlistsPerPage = 20
)

// Setlistfm fakes the parts of the Setlist.fm REST API 1.0 (https://api.setlist.fm/docs/1.0/)
// that auto-setlist uses. Mount it at the API root, i.e. where "/rest/1.0" would be.
type Setlistfm struct {
	mu      sync.RWMutex
	artists []Artist
	mux     *http.ServeMux
}

func NewSetlistfm() *Setlistfm {
	f := &Setlistfm{}
	f.mux = http.NewServeMux()
	f.mux.HandleFunc("GET /search/artists", f.searchArtists)
	f.mux.HandleFunc("GET /artist/{mbid}/setlists", f.artistSetlists)
	f.mux.HandleFunc("GET /search/setlists", f.searchSetlists)
	return f
}

// Load replaces the fake's data with the given scenario.
func (f *Setlistfm) Load(s SetlistfmScenario) {
	artists := make([]Artist, len(s.Artists))
	for i, a := range s.Artists {
		if a.MBID == "" {
			a.MBID = fakeMBID(a.Name)
		}
		artists[i] = a
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.artists = artists
}

func (f *Setlistfm) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("x-api-key") == "" {
		writeSetlistfmError(w, http.StatusForbidden, "Forbidden")
		return
	}
	// Without this header the real API answers in XML.
	if !strings.Contains(r.Header.Get("Accept"), "application/json") {
		writeSetlistfmError(w, http.StatusNotAcceptable, "Not Acceptable")
		return
	}
	f.mux.ServeHTTP(w, r)
}

func (f *Setlistfm) searchArtists(w http.ResponseWriter, r *http.Request) {
	query := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("artistName")))
	f.mu.RLock()
	defer f.mu.RUnlock()

	// Relevance: exact name matches first, then partial matches, each in scenario order.
	var exact, partial []setlistfmArtistJSON
	for _, a := range f.artists {
		name := strings.ToLower(a.Name)
		switch {
		case query != "" && name == query:
			exact = append(exact, artistJSON(a.Name, a.MBID))
		case query != "" && strings.Contains(name, query):
			partial = append(partial, artistJSON(a.Name, a.MBID))
		}
	}
	matches := append(exact, partial...)

	page, items, ok := paginate(r, matches, setlistfmArtistsPerPage)
	if !ok {
		writeSetlistfmError(w, http.StatusNotFound, "not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"type":         "artists",
		"itemsPerPage": setlistfmArtistsPerPage,
		"page":         page,
		"total":        len(matches),
		"artist":       items,
	})
}

func (f *Setlistfm) artistSetlists(w http.ResponseWriter, r *http.Request) {
	mbid := r.PathValue("mbid")
	f.mu.RLock()
	defer f.mu.RUnlock()

	var setlists []setlistJSON
	for _, a := range f.artists {
		if a.MBID == mbid {
			setlists = setlistsJSON(a)
		}
	}
	writeSetlistsPage(w, r, setlists)
}

func (f *Setlistfm) searchSetlists(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	mbid := q.Get("artistMbid")
	tour := strings.ToLower(q.Get("tourName"))
	f.mu.RLock()
	defer f.mu.RUnlock()

	var setlists []setlistJSON
	for _, a := range f.artists {
		if mbid != "" && a.MBID != mbid {
			continue
		}
		for _, sl := range setlistsJSON(a) {
			if tour != "" && (sl.Tour == nil || strings.ToLower(sl.Tour.Name) != tour) {
				continue
			}
			setlists = append(setlists, sl)
		}
	}
	writeSetlistsPage(w, r, setlists)
}

func writeSetlistsPage(w http.ResponseWriter, r *http.Request, setlists []setlistJSON) {
	page, items, ok := paginate(r, setlists, setlistfmSetlistsPerPage)
	if !ok {
		writeSetlistfmError(w, http.StatusNotFound, "not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"type":         "setlists",
		"itemsPerPage": setlistfmSetlistsPerPage,
		"page":         page,
		"total":        len(setlists),
		"setlist":      items,
	})
}

// paginate returns the requested page (parameter "p", 1-based). Like the real API, an empty
// result or a page past the end is reported as not found rather than as an empty list.
func paginate[T any](r *http.Request, all []T, perPage int) (int, []T, bool) {
	page := 1
	if p := r.URL.Query().Get("p"); p != "" {
		n, err := strconv.Atoi(p)
		if err != nil || n < 1 {
			return 0, nil, false
		}
		page = n
	}
	start := (page - 1) * perPage
	if start >= len(all) {
		return 0, nil, false
	}
	return page, all[start:min(start+perPage, len(all))], true
}

func writeSetlistfmError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]any{
		"code":      status,
		"status":    http.StatusText(status),
		"message":   message,
		"timestamp": time.Now().UTC().Format("2006-01-02T15:04:05.000-0700"),
	})
}

// JSON shapes of setlist.fm's artist and setlist types.

type setlistfmArtistJSON struct {
	MBID           string `json:"mbid"`
	Name           string `json:"name"`
	SortName       string `json:"sortName"`
	Disambiguation string `json:"disambiguation"`
	URL            string `json:"url"`
}

type setlistJSON struct {
	ID          string              `json:"id"`
	VersionID   string              `json:"versionId"`
	EventDate   string              `json:"eventDate"`
	LastUpdated string              `json:"lastUpdated"`
	Artist      setlistfmArtistJSON `json:"artist"`
	Venue       venueJSON           `json:"venue"`
	Tour        *tourJSON           `json:"tour,omitempty"`
	Sets        setsJSON            `json:"sets"`
	URL         string              `json:"url"`
}

type venueJSON struct {
	ID   string   `json:"id"`
	Name string   `json:"name"`
	City cityJSON `json:"city"`
	URL  string   `json:"url"`
}

type cityJSON struct {
	ID      string      `json:"id"`
	Name    string      `json:"name"`
	Coords  coordsJSON  `json:"coords"`
	Country countryJSON `json:"country"`
}

type coordsJSON struct {
	Lat  float64 `json:"lat"`
	Long float64 `json:"long"`
}

type countryJSON struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

type tourJSON struct {
	Name string `json:"name"`
}

type setsJSON struct {
	Set []setJSON `json:"set"`
}

type setJSON struct {
	Encore int        `json:"encore,omitempty"`
	Song   []songJSON `json:"song"`
}

type songJSON struct {
	Name  string               `json:"name"`
	Cover *setlistfmArtistJSON `json:"cover,omitempty"`
}

func artistJSON(name, mbid string) setlistfmArtistJSON {
	return setlistfmArtistJSON{
		MBID:     mbid,
		Name:     name,
		SortName: name,
		URL:      fmt.Sprintf("https://www.setlist.fm/setlists/%s-%s.html", slug(name), shortHash(name)),
	}
}

func setlistsJSON(a Artist) []setlistJSON {
	result := make([]setlistJSON, len(a.Setlists))
	newest := time.Date(2026, time.September, 26, 0, 0, 0, 0, time.UTC)
	for i, sl := range a.Setlists {
		date := sl.EventDate
		if date == "" {
			date = newest.AddDate(0, 0, -2*i).Format("02-01-2006")
		}
		venue := sl.Venue
		if venue == "" {
			venue = "The Venue"
		}
		city := sl.City
		if city == "" {
			city = "Lisbon"
		}
		id := shortHash(fmt.Sprintf("%s/%d", a.MBID, i))

		sets := setsJSON{Set: []setJSON{}}
		for j, songs := range sl.Sets {
			set := setJSON{Song: []songJSON{}}
			if j > 0 {
				set.Encore = j
			}
			for _, s := range songs {
				song := songJSON{Name: s.Name}
				if s.CoverOf != "" {
					cover := artistJSON(s.CoverOf, fakeMBID(s.CoverOf))
					song.Cover = &cover
				}
				set.Song = append(set.Song, song)
			}
			sets.Set = append(sets.Set, set)
		}

		var tour *tourJSON
		if sl.Tour != "" {
			tour = &tourJSON{Name: sl.Tour}
		}

		result[i] = setlistJSON{
			ID:          id,
			VersionID:   "g" + shortHash(id),
			EventDate:   date,
			LastUpdated: newest.Format("2006-01-02T15:04:05.000-0700"),
			Artist:      artistJSON(a.Name, a.MBID),
			Venue: venueJSON{
				ID:   shortHash(venue),
				Name: venue,
				City: cityJSON{
					ID:      "2267057",
					Name:    city,
					Coords:  coordsJSON{Lat: 38.716, Long: -9.133},
					Country: countryJSON{Code: "PT", Name: "Portugal"},
				},
				URL: fmt.Sprintf("https://www.setlist.fm/venue/%s-%s.html", slug(venue), shortHash(venue)),
			},
			Tour: tour,
			Sets: sets,
			URL:  fmt.Sprintf("https://www.setlist.fm/setlist/%s/%s.html", slug(a.Name), id),
		}
	}
	return result
}

// fakeMBID derives a stable, UUID-shaped MusicBrainz ID from a name.
func fakeMBID(name string) string {
	h := sha1.Sum([]byte("mbid:" + name))
	x := hex.EncodeToString(h[:16])
	return x[0:8] + "-" + x[8:12] + "-" + x[12:16] + "-" + x[16:20] + "-" + x[20:32]
}

func shortHash(s string) string {
	h := sha1.Sum([]byte(s))
	return hex.EncodeToString(h[:4])
}

func slug(s string) string {
	return strings.Join(strings.Fields(strings.ToLower(s)), "-")
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json;charset=UTF-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
