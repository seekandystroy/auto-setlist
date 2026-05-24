# auto-setlist
Tool to transform concert setlists into spotify playlists. Learning Go with Claude's help, while being opinionated about architecture, concurrency and maintainability.

Currently fetches the latest setlist on [SetlistFM](https://www.setlist.fm/) for a given artist, and creates a playlist with those tracks on Spotify under the authorized user's account.

I have a build deployed [here](https://auto-setlist.onrender.com/), but Spotify introduced [limitations](https://developer.spotify.com/documentation/web-api/concepts/quota-modes) to their API's usage that prevent the current implementation from working for spotify accounts outside of my 5-user allowlist.

## Building and running

### Pre-conditions
1. SetlistFM API key
2. Spotify client credentials

### CLI
1. `export SETLISTFM_API_KEY=YOURKEY SPOTIFY_CLIENT_ID=YOURKEY SPOTIFY_CLIENT_SECRET=YOURKEY`
2. `go build -o auto-setlist ./cmd/cli`
3. `./auto-setlist BANDNAME`.

### Web app
1. `export SETLISTFM_API_KEY=YOURKEY SPOTIFY_CLIENT_ID=YOURKEY SPOTIFY_CLIENT_SECRET=YOURKEY`
2. `go build -o auto-setlist ./cmd/server`
3. `./auto-setlist` and open `localhost:3000` on the browser.

## Current state
1. CLI and Web app available (deployed [here](https://auto-setlist.onrender.com/) with Render's free tier)
1. Get the first artist found on SetlistFM
2. Get that artist's most recent setlist
3. Create a setlist on the user's Spotify account with that setlist's songs

## WIP
1. Figuring out an alternative to Spotify's 5-user restriction

## Next
1. Support covers

## Later
1. Festival mode (create a setlist with top tracks from bands attending a festival)
2. Aggregate of top setlists
3. Artist choice