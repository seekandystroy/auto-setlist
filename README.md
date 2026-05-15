# auto-setlist
Tool to transform concert setlists into spotify playlists.

Starting as a CLI-only tool that fetches the latest setlist for a given artist, and creates it on spotify under a user's account.

Learning Go with Claude's help, while being opinionated about architecture, concurrency and maintainability.

## Usage

### Pre-conditions
1. SetlistFM API key
2. Spotify client credentials

### Building and running

1. `go build ./..`
2. `export SETLISTFM_API_KEY=YOURKEY SPOTIFY_CLIENT_ID=YOURKEY SPOTIFY_CLIENT_SECRET=YOURKEY`
3. `go run ./cmd BANDNAME`

## MVP (current)
1. CLI-only
1. Get the first artist found on SetlistFM
2. Get that artist's most recent setlist
3. Create a setlist on the user's Spotify account with that setlist's songs

## Next
1. UI
2. Deployment

## Later
1. Support covers
2. Festival mode (create a setlist with top tracks from bands attending a festival)
3. Aggregate of top setlists
4. Artist choice