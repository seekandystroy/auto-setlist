// Spec: acceptance/specs/setlist-link-playlist.md
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

const linkTo = (id) => `https://www.setlist.fm/setlist/hellripper/2026/the-venue-lisbon-${id}.html`;

async function createPlaylist(page, text) {
  await page.getByPlaceholder('Artist name').fill(text);
  await page.getByRole('button', { name: 'Create Playlist' }).click();
}

test.describe('Playlist from a Setlist.fm link', () => {
  test.beforeEach(async ({ connectAs }) => {
    await connectAs(visitor);
  });

  test('Creating a playlist from a Setlist.fm link', async ({ page, world }) => {
    // The link points to an older show, so using the latest one instead would be caught.
    await world.load(
      hellripperWith([
        { id: '4bd1e3a2', sets: [['All Hail the Goat']] },
        { id: '63de4613', sets: [["Vampire's Grave", 'The Nuckelavee']] },
      ]),
    );
    await page.goto('/');

    await createPlaylist(page, linkTo('63de4613'));

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
    expect(playlist.tracks.map((t) => t.name)).toEqual(["Vampire's Grave", 'The Nuckelavee']);
  });

  test('The "Songs from" choice is hidden for a link', async ({ page, world }) => {
    await world.load(hellripperWith([{ id: '63de4613', sets: [['All Hail the Goat']] }]));
    await page.goto('/');
    const field = page.getByPlaceholder('Artist name');
    const latestShow = page.getByRole('button', { name: 'Latest show' });
    const wholeTour = page.getByRole('button', { name: 'Whole tour' });

    await field.fill(linkTo('63de4613'));
    await expect(page.getByText('Songs from:')).toBeHidden();
    await expect(latestShow).toBeHidden();
    await expect(wholeTour).toBeHidden();

    await field.fill('Hellripper');
    await expect(page.getByText('Songs from:')).toBeVisible();
    await expect(latestShow).toBeVisible();
    await expect(wholeTour).toBeVisible();
  });

  test("A link that isn't a Setlist.fm setlist shows an error", async ({ page, world }) => {
    await world.load(hellripperWith([{ id: '63de4613', sets: [['All Hail the Goat']] }]));
    await page.goto('/');

    for (const notASetlist of [
      'https://example.com/setlist/hellripper/2026/the-venue-lisbon-63de4613.html',
      'https://www.setlist.fm/setlists/hellripper-4bd6a3e2.html',
    ]) {
      await createPlaylist(page, notASetlist);

      await expect(
        page.getByText('only setlist.fm setlist links are supported', { exact: true }),
      ).toBeVisible();
    }
    expect(await world.playlists()).toEqual([]);
  });

  test("A link to a setlist Setlist.fm doesn't have shows an error", async ({ page, world }) => {
    await world.load(hellripperWith([{ id: '63de4613', sets: [['All Hail the Goat']] }]));
    await page.goto('/');

    await createPlaylist(page, linkTo('deadbeef'));

    await expect(page.getByText('Setlist not found', { exact: true })).toBeVisible();
    expect(await world.playlists()).toEqual([]);
  });

  test('A link to a show with no songs listed shows an error', async ({ page, world }) => {
    await world.load(hellripperWith([{ id: '63de4613', sets: [] }]));
    await page.goto('/');

    await createPlaylist(page, linkTo('63de4613'));

    await expect(page.getByText('this setlist has no songs yet', { exact: true })).toBeVisible();
    expect(await world.playlists()).toEqual([]);
  });

  test('A link to a show with none of its songs on Spotify shows an error', async ({ page, world }) => {
    await world.load(hellripperWith([{ id: '63de4613', sets: [['Unreleased Song', 'Another Unreleased Song']] }]));
    await page.goto('/');

    await createPlaylist(page, linkTo('63de4613'));

    await expect(
      page.getByText('songs from setlistfm for "Hellripper" not found on Spotify', { exact: true }),
    ).toBeVisible();
    expect(await world.playlists()).toEqual([]);
  });
});
