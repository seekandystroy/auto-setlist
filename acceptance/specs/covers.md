# Cover songs

Bands often play covers, and Spotify usually doesn't have the band's own recording of them. To avoid gaps in the playlist, the web app always uses the original artist's version when the band's version isn't on Spotify. (The CLI makes this optional with `--include-covers`; the web app always turns it on.)

## Scenarios

### A cover the band never recorded gets the original artist's version

- **Given** Hellripper's latest show ends with "Whiplash", which Setlist.fm marks as a Metallica cover
- **And** Spotify has "Whiplash" by Metallica but not by Hellripper
- **When** a connected visitor creates a playlist for Hellripper
- **Then** the playlist ends with "Whiplash" by Metallica

### A cover the band recorded gets the band's version

- **Given** Hellripper's latest show includes "Whiplash", which Setlist.fm marks as a Metallica cover
- **And** Spotify has "Whiplash" by both Hellripper and Metallica
- **When** a connected visitor creates a playlist for Hellripper
- **Then** the playlist has "Whiplash" by Hellripper
