package adapters

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	applog "github.com/seekandystroy/auto-setlist/internal"
	"github.com/seekandystroy/auto-setlist/internal/core/domain"
)

const spotifyArtistPrefix = "https://open.spotify.com/artist/"

type musicbrainzAdapter struct {
	httpClient *http.Client
	baseURL    string
	userAgent  string
}

type musicbrainzURLRelation struct {
	URL struct {
		Resource string `json:"resource"`
	} `json:"url"`
}

type musicbrainzArtistResponse struct {
	ID        string                   `json:"id"`
	Name      string                   `json:"name"`
	Relations []musicbrainzURLRelation `json:"relations"`
}

func NewMusicbrainzAdapter() *musicbrainzAdapter {
	return &musicbrainzAdapter{
		httpClient: &http.Client{},
		baseURL:    "https://musicbrainz.org/ws/2",
		userAgent:  "auto-setlist/0.1 ( https://github.com/seekandystroy )",
	}
}

func (a *musicbrainzAdapter) GetArtist(ctx context.Context, mbid string) (*domain.Artist, error) {
	applog.LoggerFromCtx(ctx).Info("Getting artist's Spotify ID from MusicBrainz", "mbid", mbid)
	endpoint := fmt.Sprintf("%s/artist/%s?inc=url-rels&fmt=json", a.baseURL, mbid)

	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("musicbrainz: building request: %w", err)
	}
	req.Header.Set("User-Agent", a.userAgent)
	req.Header.Set("Accept", "application/json")

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("musicbrainz: executing request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("musicbrainz: unexpected status %d", resp.StatusCode)
	}

	var result musicbrainzArtistResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("musicbrainz: decoding response: %w", err)
	}

	artist := &domain.Artist{MBID: result.ID, Name: result.Name}
	for _, rel := range result.Relations {
		if strings.HasPrefix(rel.URL.Resource, spotifyArtistPrefix) {
			artist.SpotifyID = strings.TrimPrefix(rel.URL.Resource, spotifyArtistPrefix)
			break
		}
	}

	return artist, nil
}
