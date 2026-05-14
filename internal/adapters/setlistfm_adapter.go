package adapters

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/seekandystroy/auto-setlist/internal/core/domain"
)

type setlistFMAdapter struct {
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

func NewSetlistFmAdapter(apiKey string) *setlistFMAdapter {
	return &setlistFMAdapter{
		apiKey:     apiKey,
		httpClient: &http.Client{},
		baseURL:    "https://api.setlist.fm/rest/1.0",
	}
}

func (c *setlistFMAdapter) SearchArtists(name string) ([]domain.Artist, error) {
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
