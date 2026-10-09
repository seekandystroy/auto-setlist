// Package fakes provides in-memory fakes of the external APIs auto-setlist talks to
// (Setlist.fm REST 1.0, Spotify Web API and Spotify Accounts), for acceptance tests.
//
// The fakes are seeded with a Scenario and answer with JSON shaped like the real APIs.
// They model only the endpoints and fields auto-setlist uses, plus enough realistic
// surrounding fields that a client relying on undocumented shapes would be caught.
package fakes

import (
	"encoding/json"
	"fmt"
)

// Scenario is the world the fakes simulate. Tests send it as JSON to acceptance/fakeserver.
type Scenario struct {
	Setlistfm SetlistfmScenario `json:"setlistfm"`
	Spotify   SpotifyScenario   `json:"spotify"`
}

type SetlistfmScenario struct {
	Artists []Artist `json:"artists"`
}

type Artist struct {
	Name string `json:"name"`
	// MBID defaults to a value derived from Name when empty.
	MBID string `json:"mbid,omitempty"`
	// Setlists are ordered newest first, like setlist.fm returns them.
	Setlists []Setlist `json:"setlists"`
}

type Setlist struct {
	// ID is setlist.fm's setlist ID, the last part of the setlist's link; defaults to a value
	// derived from the artist and the position.
	ID string `json:"id,omitempty"`
	// EventDate uses setlist.fm's dd-MM-yyyy format; defaults to a date derived from the position.
	EventDate string `json:"eventDate,omitempty"`
	Tour      string `json:"tour,omitempty"`
	Venue     string `json:"venue,omitempty"`
	City      string `json:"city,omitempty"`
	// Sets holds the main set and encores. A setlist with no sets is a show nobody has filled in yet.
	Sets [][]Song `json:"sets"`
}

// Song is a song played at a show. In JSON it is either a plain string (the song name)
// or an object {"name": "...", "coverOf": "Original Artist"}.
type Song struct {
	Name    string `json:"name"`
	CoverOf string `json:"coverOf,omitempty"`
}

func (s *Song) UnmarshalJSON(data []byte) error {
	var name string
	if err := json.Unmarshal(data, &name); err == nil {
		*s = Song{Name: name}
		return nil
	}
	type plain Song
	var p plain
	if err := json.Unmarshal(data, &p); err != nil {
		return fmt.Errorf("song must be a string or {name, coverOf}: %w", err)
	}
	*s = Song(p)
	return nil
}

type SpotifyScenario struct {
	Users []SpotifyUser `json:"users"`
	// LoginAs is the ID of the user who signs in on the fake authorization page;
	// defaults to the first user.
	LoginAs string `json:"loginAs,omitempty"`
	// Catalog is every track the fake Spotify knows about, in search ranking order.
	Catalog []Track `json:"catalog"`
}

type SpotifyUser struct {
	ID string `json:"id"`
	// AccessToken and RefreshToken are pre-issued tokens, for tests that start already connected.
	AccessToken  string `json:"accessToken,omitempty"`
	RefreshToken string `json:"refreshToken,omitempty"`
	// NotAllowlisted users are not registered for the app in Spotify's
	// development mode: they can sign in, but every Web API call is rejected.
	NotAllowlisted bool `json:"notAllowlisted,omitempty"`
}

type Track struct {
	Name    string   `json:"name"`
	Artists []string `json:"artists"`
	// URI defaults to a value derived from the name and artists when empty.
	URI string `json:"uri,omitempty"`
}
