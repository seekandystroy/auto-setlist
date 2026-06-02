package adapters

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
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
	Sets setlistfmSets  `json:"sets"`
	Tour *setlistfmTour `json:"tour,omitempty"`
}

type setlistfmSetlistsResponse struct {
	Setlists []setlistfmSetlist `json:"setlist"`
}

func NewSetlistfmAdapter(apiKey string) *setlistfmAdapter {
	return &setlistfmAdapter{
		apiKey:     apiKey,
		httpClient: &http.Client{},
		baseURL:    "https://api.setlist.fm/rest/1.0",
		sleepFn:    time.Sleep,
	}
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
	log := applog.LoggerFromCtx(ctx)
	log.Info("Getting setlists from SetlistFM", "artist", artist.Name)
	endpoint := fmt.Sprintf("%s/artist/%s/setlists?p=1", c.baseURL, artist.MBID)

	var lastErr error
	wait := time.Second
	for attempt := range 4 {
		if attempt > 0 {
			log.Warn("GET setlists got error, waiting and retrying", "wait", wait)
			c.sleepFn(wait)
			wait *= 2
		}

		req, err := http.NewRequest(http.MethodGet, endpoint, nil)
		if err != nil {
			return nil, fmt.Errorf("setlistfm: building request: %w", err)
		}
		req.Header.Set("x-api-key", c.apiKey)
		req.Header.Set("Accept", "application/json")
		req.Close = true

		resp, err := c.httpClient.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("setlistfm: executing request: %w", err)
			continue
		}

		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			lastErr = fmt.Errorf("setlistfm: unexpected status %d", resp.StatusCode)
			continue
		}

		var result setlistfmSetlistsResponse
		err = json.NewDecoder(resp.Body).Decode(&result)
		resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("setlistfm: decoding response: %w", err)
		}

		setlists := make([]domain.Setlist, len(result.Setlists))
		for i, sl := range result.Setlists {
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
			setlists[i] = domain.Setlist{Artist: artist, Tracks: tracks, Tour: tourName}
		}
		return setlists, nil
	}

	return nil, lastErr
}

func (c *setlistfmAdapter) GetSetlistsForTour(ctx context.Context, artist domain.Artist, tourName string) ([]domain.Setlist, error) {
	log := applog.LoggerFromCtx(ctx)
	log.Info("Getting setlists for tour from SetlistFM", "artist", artist.Name, "tour", tourName)
	// Keeping it to 1 page here as well. Doubt that 20 sets won't cover 99% of the songs played
	endpoint := fmt.Sprintf(
		"%s/search/setlists?artistMbid=%s&tourName=%s&p=1",
		c.baseURL,
		url.QueryEscape(artist.MBID),
		url.QueryEscape(tourName),
	)

	var lastErr error
	wait := time.Second
	for attempt := range 4 {
		if attempt > 0 {
			log.Warn("GET setlists for tour got error, waiting and retrying", "wait", wait)
			c.sleepFn(wait)
			wait *= 2
		}

		req, err := http.NewRequest(http.MethodGet, endpoint, nil)
		if err != nil {
			return nil, fmt.Errorf("setlistfm: building request: %w", err)
		}
		req.Header.Set("x-api-key", c.apiKey)
		req.Header.Set("Accept", "application/json")
		req.Close = true

		resp, err := c.httpClient.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("setlistfm: executing request: %w", err)
			continue
		}

		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			lastErr = fmt.Errorf("setlistfm: unexpected status %d", resp.StatusCode)
			continue
		}

		var result setlistfmSetlistsResponse
		err = json.NewDecoder(resp.Body).Decode(&result)
		resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("setlistfm: decoding response: %w", err)
		}

		setlists := make([]domain.Setlist, len(result.Setlists))
		for i, sl := range result.Setlists {
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
			var slTourName string
			if sl.Tour != nil {
				slTourName = sl.Tour.Name
			}
			setlists[i] = domain.Setlist{Artist: artist, Tracks: tracks, Tour: slTourName}
		}
		return setlists, nil
	}

	return nil, lastErr
}
