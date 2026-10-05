# Playlist from the whole tour

A connected visitor who wants more than one night's songs picks "Songs from: Whole tour". The playlist then has every song the artist played on the tour of their latest show, without repeats.

Songs are ordered by where they were played in the show: everything that opened a show comes first, then everything played second, and so on. Within each position, the latest shows come first. A song keeps the first position it gets.

## Scenarios

### Creating a playlist from the artist's whole tour

- **Given** Hellripper's latest show is part of the "Goatkraft & Granite Europe 2026" tour, and Setlist.fm lists these shows on that tour, latest first:
  - "All Hail the Goat", "Vampire's Grave", "The Nuckelavee"
  - "All Hail the Goat", "Bastard of Hades", "Vampire's Grave", "Whiplash"
- **And** a connected visitor
- **When** they select "Whole tour", type "Hellripper" and click "Create Playlist"
- **Then** they get a "Listen on Spotify" link
- **And** their Spotify account has a playlist named "Hellripper - Goatkraft & Granite Europe 2026 setlist by auto-setlist" with "All Hail the Goat", "Vampire's Grave", "Bastard of Hades", "The Nuckelavee", "Whiplash", in that order

### Whole tour falls back to the latest show when it isn't part of a tour

- **Given** an artist whose latest show isn't part of any tour on Setlist.fm
- **When** a connected visitor selects "Whole tour" and creates a playlist for that artist
- **Then** the playlist is named "<artist> setlist by auto-setlist" (no tour name) and has the songs from the latest show only
