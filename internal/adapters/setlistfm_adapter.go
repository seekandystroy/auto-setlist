package adapters

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	applog "github.com/seekandystroy/auto-setlist/internal"
	"github.com/seekandystroy/auto-setlist/internal/core/domain"
)

type setlistfmAdapter struct {
	apiKey     string
	httpClient *http.Client
	baseURL    string
	sleepFn    func(time.Duration)
}

type setlistfmArtist struct {
	MBID           string `json:"mbid"`
	Name           string `json:"name"`
	SortName       string `json:"sortName"`
	Disambiguation string `json:"disambiguation,omitempty"`
	URL            string `json:"url"`
}

type setlistfmSearchResult struct {
	Type         string            `json:"type"`
	ItemsPerPage int               `json:"itemsPerPage"`
	Page         int               `json:"page"`
	Total        int               `json:"total"`
	Artists      []setlistfmArtist `json:"artist"`
}

type setlistfmCoverArtist struct {
	Name string `json:"name"`
}

type setlistfmSong struct {
	Name  string                `json:"name"`
	Cover *setlistfmCoverArtist `json:"cover,omitempty"`
}

type setlistfmSet struct {
	Songs []setlistfmSong `json:"song"`
}

type setlistfmSets struct {
	Set []setlistfmSet `json:"set"`
}

type setlistfmTour struct {
	Name string `json:"name"`
}

type setlistfmSetlist struct {
	Artist setlistfmArtist `json:"artist"`
	Sets   setlistfmSets   `json:"sets"`
	Tour   *setlistfmTour  `json:"tour,omitempty"`
}

type setlistfmSetlistsResponse struct {
	Setlists []setlistfmSetlist `json:"setlist"`
}

// setlistPathPattern matches the path of a setlist link, /setlist/<artist>/<year>/<venue>-<id>.html,
// capturing the ID (8 hex digits).
var setlistPathPattern = regexp.MustCompile(`^/setlist/[^/]+/\d{4}/[^/]*-([0-9a-f]{8})\.html$`)

// SetlistfmOption configures optional settings on the Setlist.fm adapter.
type SetlistfmOption func(*setlistfmAdapter)

// WithSetlistfmBaseURL points the adapter at a different Setlist.fm API root,
// e.g. a fake server in acceptance tests.
func WithSetlistfmBaseURL(baseURL string) SetlistfmOption {
	return func(c *setlistfmAdapter) { c.baseURL = baseURL }
}

func NewSetlistfmAdapter(apiKey string, opts ...SetlistfmOption) *setlistfmAdapter {
	c := &setlistfmAdapter{
		apiKey:     apiKey,
		httpClient: &http.Client{},
		baseURL:    "https://api.setlist.fm/rest/1.0",
		sleepFn:    time.Sleep,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

func (c *setlistfmAdapter) SearchArtists(ctx context.Context, artistName string) ([]domain.Artist, error) {
	applog.LoggerFromCtx(ctx).Info("Searching for artist on SetlistFM", "name", artistName)
	// Intentionally just getting the first page of results, artist choice later
	endpoint := fmt.Sprintf(
		"%s/search/artists?artistName=%s&p=1&sort=relevance",
		c.baseURL,
		url.QueryEscape(artistName),
	)

	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("setlistfm: building request: %w", err)
	}
	req.Header.Set("x-api-key", c.apiKey)
	req.Header.Set("Accept", "application/json")
	req.Close = true

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("setlistfm: executing request: %w", err)
	}
	defer resp.Body.Close()

	// Setlist.fm answers a search with no matches with 404.
	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("setlistfm: unexpected status %d", resp.StatusCode)
	}

	var result setlistfmSearchResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("setlistfm: decoding response: %w", err)
	}

	artists := make([]domain.Artist, len(result.Artists))
	for i, a := range result.Artists {
		artists[i] = domain.Artist{MBID: a.MBID, Name: a.Name}
	}
	return artists, nil
}

func (c *setlistfmAdapter) GetSetlists(ctx context.Context, artist domain.Artist) ([]domain.Setlist, error) {
	applog.LoggerFromCtx(ctx).Info("Getting setlists from SetlistFM", "artist", artist.Name)
	endpoint := fmt.Sprintf("%s/artist/%s/setlists?p=1", c.baseURL, artist.MBID)
	return c.fetchSetlists(ctx, endpoint, artist)
}

func (c *setlistfmAdapter) GetSetlistsForTour(ctx context.Context, artist domain.Artist, tourName string) ([]domain.Setlist, error) {
	applog.LoggerFromCtx(ctx).Info("Getting setlists for tour from SetlistFM", "artist", artist.Name, "tour", tourName)
	// Keeping it to 1 page here as well. Doubt that 20 sets won't cover 99% of the songs played
	endpoint := fmt.Sprintf(
		"%s/search/setlists?artistMbid=%s&tourName=%s&p=1",
		c.baseURL,
		url.QueryEscape(artist.MBID),
		url.QueryEscape(tourName),
	)
	return c.fetchSetlists(ctx, endpoint, artist)
}

// SetlistIDFromURL extracts the setlist ID from a setlist.fm setlist link, e.g.
// https://www.setlist.fm/setlist/the-beatles/1964/hollywood-bowl-hollywood-ca-63de4613.html.
func (c *setlistfmAdapter) SetlistIDFromURL(rawURL string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return "", domain.ErrInvalidSetlistURL
	}
	host := strings.ToLower(u.Hostname())
	if host != "setlist.fm" && !strings.HasSuffix(host, ".setlist.fm") {
		return "", domain.ErrInvalidSetlistURL
	}
	m := setlistPathPattern.FindStringSubmatch(u.Path)
	if m == nil {
		return "", domain.ErrInvalidSetlistURL
	}
	return m[1], nil
}

func (c *setlistfmAdapter) GetSetlist(ctx context.Context, setlistID string) (*domain.Setlist, error) {
	applog.LoggerFromCtx(ctx).Info("Getting setlist from SetlistFM", "setlist_id", setlistID)
	endpoint := fmt.Sprintf("%s/setlist/%s", c.baseURL, url.PathEscape(setlistID))

	var result setlistfmSetlist
	found, err := c.getJSON(ctx, endpoint, &result)
	if err != nil || !found {
		return nil, err
	}
	setlist := toDomainSetlist(result, domain.Artist{MBID: result.Artist.MBID, Name: result.Artist.Name})
	return &setlist, nil
}

// fetchSetlists GETs a page of setlists.
func (c *setlistfmAdapter) fetchSetlists(ctx context.Context, endpoint string, artist domain.Artist) ([]domain.Setlist, error) {
	var result setlistfmSetlistsResponse
	found, err := c.getJSON(ctx, endpoint, &result)
	if err != nil || !found {
		return nil, err
	}

	setlists := make([]domain.Setlist, len(result.Setlists))
	for i, sl := range result.Setlists {
		setlists[i] = toDomainSetlist(sl, artist)
	}
	return setlists, nil
}

// getJSON GETs endpoint and decodes the response into v, retrying failures with exponential backoff.
// It reports found=false, without error, when setlist.fm answers 404.
func (c *setlistfmAdapter) getJSON(ctx context.Context, endpoint string, v any) (bool, error) {
	log := applog.LoggerFromCtx(ctx)

	var lastErr error
	wait := time.Second
	for attempt := range 4 {
		if attempt > 0 {
			log.Warn("GET from SetlistFM got error, waiting and retrying", "wait", wait)
			c.sleepFn(wait)
			wait *= 2
		}

		req, err := http.NewRequest(http.MethodGet, endpoint, nil)
		if err != nil {
			return false, fmt.Errorf("setlistfm: building request: %w", err)
		}
		req.Header.Set("x-api-key", c.apiKey)
		req.Header.Set("Accept", "application/json")
		req.Close = true

		resp, err := c.httpClient.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("setlistfm: executing request: %w", err)
			continue
		}

		// Setlist.fm answers with 404 when there's nothing to return. Retrying won't change that.
		if resp.StatusCode == http.StatusNotFound {
			resp.Body.Close()
			return false, nil
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			lastErr = fmt.Errorf("setlistfm: unexpected status %d", resp.StatusCode)
			continue
		}

		err = json.NewDecoder(resp.Body).Decode(v)
		resp.Body.Close()
		if err != nil {
			return false, fmt.Errorf("setlistfm: decoding response: %w", err)
		}
		return true, nil
	}

	return false, lastErr
}

func toDomainSetlist(sl setlistfmSetlist, artist domain.Artist) domain.Setlist {
	var tracks []domain.Track
	for _, set := range sl.Sets.Set {
		for _, song := range set.Songs {
			if song.Name == "" {
				continue
			}
			var coverName string
			if song.Cover != nil {
				coverName = song.Cover.Name
			}
			tracks = append(tracks, domain.Track{Name: song.Name, CoveredArtistName: coverName})
		}
	}
	var tourName string
	if sl.Tour != nil {
		tourName = sl.Tour.Name
	}
	return domain.Setlist{Artist: artist, Tracks: tracks, Tour: tourName}
}
