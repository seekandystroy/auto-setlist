package domain

import "time"

type SpotifyToken struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	TokenType    string    `json:"token_type"`
	ExpiresAt    time.Time `json:"expires_at"`
	Scope        string    `json:"scope"`
}

// IsValid reports whether the token is non-nil, non-empty, and not expiring within 30 seconds.
func (t *SpotifyToken) IsValid() bool {
	if t == nil || t.AccessToken == "" {
		return false
	}
	return time.Now().Add(30 * time.Second).Before(t.ExpiresAt)
}
