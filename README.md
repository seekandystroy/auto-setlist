# auto-setlist
Tool to transform concert setlists into spotify playlists. Learning Go with Claude's help, while being opinionated about architecture, concurrency and maintainability.

Given an artist, fetches either the latest setlist or all songs from the latest available tour, and creates a playlist with those tracks on Spotify under the authorized user's account. Uses [SetlistFM](https://www.setlist.fm/) API to obtain setlists.

I have a build deployed [here](https://auto-setlist.onrender.com/), but Spotify introduced [limitations](https://developer.spotify.com/documentation/web-api/concepts/quota-modes) to their API's usage that prevent the current implementation from working for spotify accounts outside of my 5-user allowlist.

## Building and running

### Pre-conditions
1. SetlistFM API key
2. Spotify client credentials

### CLI
1. `export SETLISTFM_API_KEY=YOURKEY SPOTIFY_CLIENT_ID=YOURKEY SPOTIFY_CLIENT_SECRET=YOURKEY`
2. `go build -o auto-setlist ./cmd/cli`
3. `./auto-setlist BANDNAME`. (options: --include-covers and --tour-playlist)

### Web app
1. `export SETLISTFM_API_KEY=YOURKEY SPOTIFY_CLIENT_ID=YOURKEY SPOTIFY_CLIENT_SECRET=YOURKEY`
2. `go build -o auto-setlist ./cmd/server`
3. `./auto-setlist` and open `127.0.0.1:3000` on the browser.

### Tests
- Unit tests: `go test ./...`
- Acceptance tests (requires Node): `cd acceptance && npm ci && npx playwright install chromium && npx playwright test`. These drive the web app in a real browser against in-memory fakes of Setlist.fm and Spotify, so no API keys are needed. Feature specs live in `acceptance/specs/`.

## Current state
1. CLI and Web app available (deployed [here](https://auto-setlist.onrender.com/) with Render's free tier)
1. Get the first artist found on SetlistFM
2. Get that artist's most recent setlist
3. Create a setlist on the user's Spotify account with that setlist's songs

## WIP
1. Figuring out an alternative to Spotify's 5-user restriction

## Next
1. Festival mode (create a setlist with top tracks from bands attending a festival)

## Later
1. Specific setlist
2. Aggregate of top setlists
3. Artist choice

## Future improvements
Behavior the acceptance specs (`acceptance/specs/`) currently pin as-is, but that would be better for users:
1. **Unknown artist error.** Setlist.fm answers an empty artist search with 404, so users see `searching for artist "…": setlistfm: unexpected status 404` instead of `no artist found for "…"`. Treat a 404 from search as "no results". (`latest-show-playlist.md`)
2. **Artist with no setlists.** Setlist.fm answers with 404, and the app retries it with backoff (~7s) before showing `fetching setlists for "…": setlistfm: unexpected status 404`. Don't retry 404s, and say `no setlists found for "…"`. (`latest-show-playlist.md`)
3. **Failed Spotify authorization is silent.** When the token exchange fails, the "Spotify auth error: …" message is written inside the hidden artist form, so users only see the Connect button again. Show the error next to it. (`connect-spotify.md`)
4. **Empty playlists.** When none of the songs are found on Spotify, an empty playlist is still created and linked. Show an error and skip creating it. (`latest-show-playlist.md`)