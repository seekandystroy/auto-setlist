# Connecting to Spotify

auto-setlist creates playlists in the visitor's own Spotify account, so before anything else the visitor connects their Spotify account (authorization code flow with PKCE, entirely in the browser). Once connected, they stay connected across visits until Spotify stops accepting their tokens.

## Scenarios

### A first-time visitor is asked to connect to Spotify

- **Given** a visitor who has never connected to Spotify
- **When** they open auto-setlist
- **Then** they see a "Connect to Spotify" button and the note "(auto-setlist does not store any personal information)"
- **And** they don't see the artist form

### Connecting to Spotify leads to the artist form

- **Given** a visitor who has never connected to Spotify
- **When** they click "Connect to Spotify" and agree on Spotify's authorization page
- **Then** they are back on auto-setlist and see the artist form
- **And** the address bar no longer contains the authorization code

### Cancelling on Spotify's authorization page leaves the visitor able to try again

- **Given** a visitor who has never connected to Spotify
- **When** they click "Connect to Spotify" and cancel on Spotify's authorization page
- **Then** they are back on auto-setlist and see the "Connect to Spotify" button again

### A failed authorization tells the visitor why and asks them to connect again

- **Given** a visitor coming back from Spotify with an authorization code that Spotify rejects (for example, one that was already used)
- **When** the page loads
- **Then** they see the error `Spotify auth error: <Spotify's reason>` next to the "Connect to Spotify" button
- **And** the address bar no longer contains the authorization code

### A returning visitor goes straight to the artist form

- **Given** a visitor who connected earlier and whose access token is still valid
- **When** they open auto-setlist
- **Then** they see the artist form without being asked to connect

### An expired connection is renewed without asking the visitor

- **Given** a visitor who connected earlier, whose access token has expired but whose refresh token is still valid
- **When** they open auto-setlist
- **Then** they see the artist form without being asked to connect
- **And** they can create a playlist

### A connection that can't be renewed asks the visitor to connect again

- **Given** a visitor who connected earlier, whose access token has expired and whose refresh token Spotify no longer accepts
- **When** they open auto-setlist
- **Then** they see the "Connect to Spotify" button
