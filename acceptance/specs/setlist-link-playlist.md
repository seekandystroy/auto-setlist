# Playlist from a Setlist.fm link

A connected visitor who wants a playlist of one particular show, for example one they went to, pastes that show's Setlist.fm link (`https://www.setlist.fm/setlist/<artist>/<year>/<venue>-<id>.html`) into the same field used for artist names. Any text starting with `http://` or `https://` is treated as a link. They get a playlist with the songs from that show, in the order they were played.

The playlist is named with Setlist.fm's spelling of the artist. Covers follow the same rules as for an artist name (see `covers.md`).

## Scenarios

### Creating a playlist from a Setlist.fm link

- **Given** a show by an artist on Setlist.fm, with its songs available on Spotify
- **And** a connected visitor
- **When** they paste the show's Setlist.fm link and click "Create Playlist"
- **Then** the button reads "Created!" and a "Listen on Spotify" link opens the new playlist in a new tab
- **And** their Spotify account has a playlist named "<artist> setlist by auto-setlist", described "auto-generated", with the songs from that show in the order they were played

### The "Songs from" choice is hidden for a link

- **Given** a connected visitor on the form
- **When** they type a link into the field
- **Then** the "Songs from" choice ("Latest show" / "Whole tour") is hidden
- **And** it shows again as soon as the field holds an artist name instead

### A link that isn't a Setlist.fm setlist shows an error

- **Given** a connected visitor
- **When** they paste a link that isn't a Setlist.fm setlist (another website, or a Setlist.fm page that isn't a setlist) and click "Create Playlist"
- **Then** they see the error `only setlist.fm setlist links are supported`
- **And** no playlist is created

### A link to a setlist Setlist.fm doesn't have shows an error

- **Given** a Setlist.fm setlist link whose setlist Setlist.fm doesn't know
- **When** a connected visitor pastes it and clicks "Create Playlist"
- **Then** they see the error `Setlist not found`
- **And** no playlist is created

### A link to a show with no songs listed shows an error

- **Given** a show on Setlist.fm that has no songs listed yet
- **When** a connected visitor pastes its link and clicks "Create Playlist"
- **Then** they see the error `this setlist has no songs yet`
- **And** no playlist is created

### A link to a show with none of its songs on Spotify shows an error

- **Given** a show on Setlist.fm whose songs Spotify doesn't have
- **When** a connected visitor pastes its link and clicks "Create Playlist"
- **Then** they see the error `songs from setlistfm for "<artist>" not found on Spotify`, with Setlist.fm's spelling of the artist
- **And** no playlist is created
