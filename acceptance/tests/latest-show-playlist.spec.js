// Spec: acceptance/specs/latest-show-playlist.md
import { test, expect } from './support/fixtures.js';

const visitor = { id: 'visitor', accessToken: 'visitor-access-token' };

const hellripperCatalog = [
  { name: 'All Hail the Goat', artists: ['Hellripper'] },
  { name: "Vampire's Grave", artists: ['Hellripper'] },
  { name: 'The Nuckelavee', artists: ['Hellripper'] },
];

// A world where Hellripper has the given setlists (newest first) and the visitor can use Spotify.
function hellripperWith(setlists, catalog = hellripperCatalog) {
  return {
    setlistfm: { artists: [{ name: 'Hellripper', setlists }] },
    spotify: { users: [visitor], catalog },
  };
}

async function createPlaylist(page, artist) {
  await page.getByPlaceholder('Artist name').fill(artist);
  await page.getByRole('button', { name: 'Create Playlist' }).click();
}

test.describe('Playlist from the latest show', () => {
  test.beforeEach(async ({ page, connectAs }) => {
    await connectAs(visitor);
  });

  test("Creating a playlist from the artist's latest show", async ({ page, world }) => {
    await world.load(hellripperWith([{ sets: [['All Hail the Goat', "Vampire's Grave", 'The Nuckelavee']] }]));
    await page.goto('/');

    await createPlaylist(page, 'hellripper');

    const link = page.getByRole('link', { name: 'Listen on Spotify' });
    await expect(page.getByRole('button', { name: 'Created!' })).toBeVisible();
    await expect(link).toHaveAttribute('target', '_blank');

    const [playlist] = await world.playlists();
    await expect(link).toHaveAttribute('href', `https://open.spotify.com/playlist/${playlist.id}`);
    expect(playlist).toMatchObject({
      owner: 'visitor',
      name: 'Hellripper setlist by auto-setlist',
      description: 'auto-generated',
    });
    expect(playlist.tracks.map((t) => t.name)).toEqual(['All Hail the Goat', "Vampire's Grave", 'The Nuckelavee']);
  });

  test('Pressing Enter creates the playlist', async ({ page, world }) => {
    await world.load(hellripperWith([{ sets: [['All Hail the Goat']] }]));
    await page.goto('/');

    await page.getByPlaceholder('Artist name').fill('Hellripper');
    await page.getByPlaceholder('Artist name').press('Enter');

    await expect(page.getByRole('link', { name: 'Listen on Spotify' })).toBeVisible();
    expect(await world.playlists()).toHaveLength(1);
  });

  test("Songs that aren't on Spotify are left out", async ({ page, world }) => {
    await world.load(
      hellripperWith([{ sets: [['All Hail the Goat', 'Unreleased Song', "Vampire's Grave", 'The Nuckelavee']] }]),
    );
    await page.goto('/');

    await createPlaylist(page, 'Hellripper');

    await expect(page.getByRole('link', { name: 'Listen on Spotify' })).toBeVisible();
    const [playlist] = await world.playlists();
    expect(playlist.tracks.map((t) => t.name)).toEqual(['All Hail the Goat', "Vampire's Grave", 'The Nuckelavee']);
  });

  test('Songs are found even when Setlist.fm and Spotify capitalize them differently', async ({ page, world }) => {
    await world.load({
      setlistfm: { artists: [{ name: 'hellripper', setlists: [{ sets: [["Hell's rock 'n' roll"]] }] }] },
      spotify: { users: [visitor], catalog: [{ name: "Hell's Rock 'n' Roll", artists: ['Hellripper'] }] },
    });
    await page.goto('/');

    await createPlaylist(page, 'Hellripper');

    await expect(page.getByRole('link', { name: 'Listen on Spotify' })).toBeVisible();
    const [playlist] = await world.playlists();
    expect(playlist.tracks.map((t) => t.name)).toEqual(["Hell's Rock 'n' Roll"]);
  });

  test('Shows nobody has filled in yet are skipped', async ({ page, world }) => {
    await world.load(
      hellripperWith([{ sets: [] }, { sets: [] }, { sets: [["Vampire's Grave", 'The Nuckelavee']] }, { sets: [['All Hail the Goat']] }]),
    );
    await page.goto('/');

    await createPlaylist(page, 'Hellripper');

    await expect(page.getByRole('link', { name: 'Listen on Spotify' })).toBeVisible();
    const [playlist] = await world.playlists();
    expect(playlist.tracks.map((t) => t.name)).toEqual(["Vampire's Grave", 'The Nuckelavee']);
  });

  test("An artist Setlist.fm doesn't know shows an error", async ({ page, world }) => {
    await world.load(hellripperWith([{ sets: [['All Hail the Goat']] }]));
    await page.goto('/');

    await createPlaylist(page, 'Nonexistent Band');

    await expect(page.getByText('Artist not found', { exact: true })).toBeVisible();
    expect(await world.playlists()).toEqual([]);
  });

  test('An artist with no setlists shows an error', async ({ page, world }) => {
    await world.load(hellripperWith([]));
    await page.goto('/');

    await createPlaylist(page, 'Hellripper');

    await expect(page.getByText('no setlists found for "Hellripper"', { exact: true })).toBeVisible();
    expect(await world.playlists()).toEqual([]);
  });

  test('An artist whose shows all lack songs shows an error', async ({ page, world }) => {
    await world.load(hellripperWith([{ sets: [] }, { sets: [] }]));
    await page.goto('/');

    await createPlaylist(page, 'Hellripper');

    await expect(page.getByText('no non-empty setlists found for "Hellripper"', { exact: true })).toBeVisible();
    expect(await world.playlists()).toEqual([]);
  });

  test('A show with none of its songs on Spotify shows an error', async ({ page, world }) => {
    await world.load(hellripperWith([{ sets: [['Unreleased Song', 'Another Unreleased Song']] }]));
    await page.goto('/');

    await createPlaylist(page, 'Hellripper');

    await expect(
      page.getByText('songs from setlistfm for "Hellripper" not found on Spotify', { exact: true }),
    ).toBeVisible();
    expect(await world.playlists()).toEqual([]);
  });

  test("Errors show the artist's name exactly as typed", async ({ page, world }) => {
    await world.load({
      setlistfm: { artists: [{ name: '<b>Hellripper</b>', setlists: [] }] },
      spotify: { users: [visitor], catalog: hellripperCatalog },
    });
    await page.goto('/');

    await createPlaylist(page, '<b>Hellripper</b>');

    await expect(page.getByText('no setlists found for "<b>Hellripper</b>"', { exact: true })).toBeVisible();
  });

  test('The button is disabled until an artist is typed', async ({ page, world }) => {
    await world.load(hellripperWith([{ sets: [['All Hail the Goat']] }]));
    await page.goto('/');
    const artist = page.getByPlaceholder('Artist name');
    const button = page.getByRole('button', { name: 'Create Playlist' });

    await expect(button).toBeDisabled();
    await artist.fill('   ');
    await expect(button).toBeDisabled();
    await artist.fill('Hellripper');
    await expect(button).toBeEnabled();
  });

  test('Changing the artist after a playlist is created starts over', async ({ page, world }) => {
    await world.load(hellripperWith([{ sets: [['All Hail the Goat']] }]));
    await page.goto('/');
    await createPlaylist(page, 'Hellripper');
    await expect(page.getByRole('link', { name: 'Listen on Spotify' })).toBeVisible();

    await page.getByPlaceholder('Artist name').fill('Hellripper live');

    await expect(page.getByRole('link', { name: 'Listen on Spotify' })).toBeHidden();
    await expect(page.getByRole('button', { name: 'Create Playlist' })).toBeEnabled();
  });

  test('A visitor whose connection ran out while on the page is asked to connect again', async ({ page, world }) => {
    await world.load(hellripperWith([{ sets: [['All Hail the Goat']] }]));
    await page.clock.install();
    await page.goto('/');
    await expect(page.getByPlaceholder('Artist name')).toBeVisible();

    await page.clock.fastForward('02:00:00');
    await createPlaylist(page, 'Hellripper');

    await expect(page.getByRole('button', { name: 'Connect to Spotify' })).toBeVisible();
    expect(await world.playlists()).toEqual([]);
  });
});
