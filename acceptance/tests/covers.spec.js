// Spec: acceptance/specs/covers.md
import { test, expect } from './support/fixtures.js';

const visitor = { id: 'visitor', accessToken: 'visitor-access-token' };

const showEndingWithWhiplash = {
  artists: [
    {
      name: 'Hellripper',
      setlists: [{ sets: [['All Hail the Goat'], [{ name: 'Whiplash', coverOf: 'Metallica' }]] }],
    },
  ],
};

async function createPlaylist(page, artist) {
  await page.goto('/');
  await page.getByPlaceholder('Artist name').fill(artist);
  await page.getByRole('button', { name: 'Create Playlist' }).click();
  await expect(page.getByRole('link', { name: 'Listen on Spotify' })).toBeVisible();
}

test.describe('Cover songs', () => {
  test.beforeEach(async ({ connectAs }) => {
    await connectAs(visitor);
  });

  test("A cover the band never recorded gets the original artist's version", async ({ page, world }) => {
    await world.load({
      setlistfm: showEndingWithWhiplash,
      spotify: {
        users: [visitor],
        catalog: [
          { name: 'All Hail the Goat', artists: ['Hellripper'] },
          { name: 'Whiplash', artists: ['Metallica'] },
        ],
      },
    });

    await createPlaylist(page, 'Hellripper');

    const [playlist] = await world.playlists();
    expect(playlist.tracks.map((t) => [t.name, t.artists])).toEqual([
      ['All Hail the Goat', ['Hellripper']],
      ['Whiplash', ['Metallica']],
    ]);
  });

  test("A cover the band recorded gets the band's version", async ({ page, world }) => {
    await world.load({
      setlistfm: showEndingWithWhiplash,
      spotify: {
        users: [visitor],
        catalog: [
          { name: 'All Hail the Goat', artists: ['Hellripper'] },
          { name: 'Whiplash', artists: ['Metallica'] },
          { name: 'Whiplash', artists: ['Hellripper'] },
        ],
      },
    });

    await createPlaylist(page, 'Hellripper');

    const [playlist] = await world.playlists();
    expect(playlist.tracks.map((t) => [t.name, t.artists])).toEqual([
      ['All Hail the Goat', ['Hellripper']],
      ['Whiplash', ['Hellripper']],
    ]);
  });
});
