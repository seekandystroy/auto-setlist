# Playlist from the latest show

A connected visitor types an artist's name and gets a Spotify playlist with the songs from that artist's most recent show on Setlist.fm, in the order they were played. This is the default mode ("Songs from: Latest show").

The artist used is Setlist.fm's best match for the typed name, and the playlist is named with Setlist.fm's spelling of the artist.

## Scenarios

### Creating a playlist from the artist's latest show

- **Given** Setlist.fm lists Hellripper's latest show with "All Hail the Goat", "Vampire's Grave" and "The Nuckelavee", all available on Spotify
- **And** a connected visitor
- **When** they type "hellripper" and click "Create Playlist"
- **Then** the button reads "Created!" and a "Listen on Spotify" link opens the new playlist in a new tab
- **And** their Spotify account has a playlist named "Hellripper setlist by auto-setlist", described "auto-generated", with those three songs in the order they were played

### Pressing Enter creates the playlist

- **Given** an artist with a latest show, and a connected visitor
- **When** they type the artist's name and press Enter
- **Then** the playlist is created, as when clicking "Create Playlist"

### Songs that aren't on Spotify are left out

- **Given** an artist whose latest show includes a song Spotify doesn't have
- **When** a connected visitor creates a playlist for that artist
- **Then** the playlist has every other song from the show, in order, and nothing in place of the missing one

### Songs are found even when Setlist.fm and Spotify capitalize them differently

- **Given** Setlist.fm lists "Hell's rock 'n' roll" by "hellripper", and Spotify has "Hell's Rock 'n' Roll" by "Hellripper"
- **When** a connected visitor creates a playlist for Hellripper
- **Then** the playlist contains "Hell's Rock 'n' Roll"

### Shows nobody has filled in yet are skipped

- **Given** an artist whose most recent shows on Setlist.fm have no songs listed yet (upcoming or not yet filled in), and the show before them has songs
- **When** a connected visitor creates a playlist for that artist
- **Then** the playlist has the songs from the most recent show that has songs

### An artist Setlist.fm doesn't know shows an error

- **Given** no artist on Setlist.fm matches "Nonexistent Band"
- **When** a connected visitor types "Nonexistent Band" and clicks "Create Playlist"
- **Then** they see the error `Artist not found`
- **And** no playlist is created

### An artist with no setlists shows an error

- **Given** an artist Setlist.fm knows but has no setlists for
- **When** a connected visitor creates a playlist for that artist
- **Then** they see the error `no setlists found for "<artist>"` right away
- **And** no playlist is created

### An artist whose shows all lack songs shows an error

- **Given** an artist whose shows on Setlist.fm all have no songs listed yet
- **When** a connected visitor creates a playlist for that artist
- **Then** they see the error `no non-empty setlists found for "<artist>"`
- **And** no playlist is created

### A show with none of its songs on Spotify creates an empty playlist

- **Given** an artist whose latest show only has songs Spotify doesn't have
- **When** a connected visitor creates a playlist for that artist
- **Then** they get a "Listen on Spotify" link
- **And** their Spotify account has an empty playlist named "<artist> setlist by auto-setlist"

### The button is disabled until an artist is typed

- **Given** a connected visitor on the artist form
- **When** the artist field is empty or only spaces
- **Then** "Create Playlist" is disabled
- **And** it becomes enabled as soon as they type a name

### Changing the artist after a playlist is created starts over

- **Given** a connected visitor who just created a playlist
- **When** they edit the artist field
- **Then** the "Listen on Spotify" link disappears and the button reads "Create Playlist" again

### A visitor whose connection ran out while on the page is asked to connect again

- **Given** a visitor on the artist form whose access token expired while the page was open, with no refresh token
- **When** they click "Create Playlist"
- **Then** they see the "Connect to Spotify" button
- **And** no playlist is created
