# Spotify allowlist

The app's Spotify integration is in development mode, so Spotify only lets a short allowlist of accounts use it. Anyone else can still sign in on Spotify's page, but every request the app makes on their behalf is rejected. Those visitors need to learn why nothing works and what to do about it.

## Scenarios

### A visitor outside the allowlist is told to contact the maintainer

- **Given** a visitor whose Spotify account isn't on the app's allowlist, who has connected to Spotify
- **And** an artist with a latest show
- **When** they create a playlist for that artist
- **Then** they see the error "spotify: users outside of allowlist not supported. Contact the maintainer to use auto-setlist."
- **And** no playlist is created
