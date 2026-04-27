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

func NewSetlistFmAdapter(apiKey string) *setlistFMAdapter {
	return &setlistFMAdapter{
		apiKey:     apiKey,
		httpClient: &http.Client{},
		baseURL:    "https://api.setlist.fm/rest/1.0",
	}
}

func (c *setlistFMAdapter) SearchArtists(name string) (*domain.ArtistSearchResult, error) {
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

	var result domain.ArtistSearchResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("setlistfm: decoding response: %w", err)
	}

	return &result, nil
}
