package domain

type Artist struct {
	MBID           string `json:"mbid"`
	Name           string `json:"name"`
	SortName       string `json:"sortName"`
	Disambiguation string `json:"disambiguation,omitempty"`
	URL            string `json:"url"`
}

type ArtistSearchResult struct {
	Type         string   `json:"type"`
	ItemsPerPage int      `json:"itemsPerPage"`
	Page         int      `json:"page"`
	Total        int      `json:"total"`
	Artists      []Artist `json:"artist"`
}
