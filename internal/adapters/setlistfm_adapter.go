package adapters

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/seekandystroy/auto-setlist/internal/core/domain"
)

type setlistfmAdapter struct {
	apiKey     string
	httpClient *http.Client
	baseURL    string
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

type setlistfmSong struct {
	Name string `json:"name"`
}

type setlistfmSet struct {
	Songs []setlistfmSong `json:"song"`
}

type setlistfmSets struct {
	Set []setlistfmSet `json:"set"`
}

type setlistfmSetlist struct {
	Sets setlistfmSets `json:"sets"`
}

type setlistfmSetlistsResponse struct {
	Setlists []setlistfmSetlist `json:"setlist"`
}

func NewSetlistfmAdapter(apiKey string) *setlistfmAdapter {
	return &setlistfmAdapter{
		apiKey:     apiKey,
		httpClient: &http.Client{},
		baseURL:    "https://api.setlist.fm/rest/1.0",
	}
}

func (c *setlistfmAdapter) SearchArtists(name string) ([]domain.Artist, error) {
	// Intentionally just getting the first page of results, artist choice later
	endpoint := fmt.Sprintf(
		"%s/search/artists?artistName=%s&p=1&sort=relevance",
		c.baseURL,
		url.QueryEscape(name),
	)

	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("setlistfm: building request: %w", err)
	}
	req.Header.Set("x-api-key", c.apiKey)
	req.Header.Set("Accept", "application/json")

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

func (c *setlistfmAdapter) GetSetlists(artist domain.Artist) ([]domain.Setlist, error) {
	endpoint := fmt.Sprintf("%s/artist/%s/setlists?p=1", c.baseURL, artist.MBID)

	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("setlistfm: building request: %w", err)
	}
	req.Header.Set("x-api-key", c.apiKey)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("setlistfm: executing request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("setlistfm: unexpected status %d", resp.StatusCode)
	}

	var result setlistfmSetlistsResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("setlistfm: decoding response: %w", err)
	}

	setlists := make([]domain.Setlist, len(result.Setlists))
	for i, sl := range result.Setlists {
		var tracks []string
		for _, set := range sl.Sets.Set {
			for _, song := range set.Songs {
				if song.Name != "" {
					tracks = append(tracks, song.Name)
				}
			}
		}
		setlists[i] = domain.Setlist{Artist: artist, Tracks: tracks}
	}
	return setlists, nil
}
