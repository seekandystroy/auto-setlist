// Spec: acceptance/specs/whole-tour-playlist.md
import { test, expect } from './support/fixtures.js';

const visitor = { id: 'visitor', accessToken: 'visitor-access-token' };
const tour = 'Goatkraft & Granite Europe 2026';
const catalog = ['All Hail the Goat', "Vampire's Grave", 'The Nuckelavee', 'Bastard of Hades', 'Whiplash'].map(
  (name) => ({ name, artists: ['Hellripper'] }),
);

test.describe('Playlist from the whole tour', () => {
  test.beforeEach(async ({ connectAs }) => {
    await connectAs(visitor);
  });

  test("Creating a playlist from the artist's whole tour", async ({ page, world }) => {
    await world.load({
      setlistfm: {
        artists: [
          {
            name: 'Hellripper',
            setlists: [
              { tour, sets: [['All Hail the Goat', "Vampire's Grave", 'The Nuckelavee']] },
              { tour, sets: [['All Hail the Goat', 'Bastard of Hades', "Vampire's Grave", 'Whiplash']] },
            ],
          },
        ],
      },
      spotify: { users: [visitor], catalog },
    });
    await page.goto('/');

    await page.getByRole('button', { name: 'Whole tour' }).click();
    await page.getByPlaceholder('Artist name').fill('Hellripper');
    await page.getByRole('button', { name: 'Create Playlist' }).click();

    await expect(page.getByRole('link', { name: 'Listen on Spotify' })).toBeVisible();
    const [playlist] = await world.playlists();
    expect(playlist.name).toBe(`Hellripper - ${tour} setlist by auto-setlist`);
    expect(playlist.tracks.map((t) => t.name)).toEqual([
      'All Hail the Goat',
      "Vampire's Grave",
      'Bastard of Hades',
      'The Nuckelavee',
      'Whiplash',
    ]);
  });

  test("Whole tour falls back to the latest show when it isn't part of a tour", async ({ page, world }) => {
    await world.load({
      setlistfm: {
        artists: [
          {
            name: 'Hellripper',
            setlists: [{ sets: [['All Hail the Goat', 'The Nuckelavee']] }, { tour, sets: [['Bastard of Hades']] }],
          },
        ],
      },
      spotify: { users: [visitor], catalog },
    });
    await page.goto('/');

    await page.getByRole('button', { name: 'Whole tour' }).click();
    await page.getByPlaceholder('Artist name').fill('Hellripper');
    await page.getByRole('button', { name: 'Create Playlist' }).click();

    await expect(page.getByRole('link', { name: 'Listen on Spotify' })).toBeVisible();
    const [playlist] = await world.playlists();
    expect(playlist.name).toBe('Hellripper setlist by auto-setlist');
    expect(playlist.tracks.map((t) => t.name)).toEqual(['All Hail the Goat', 'The Nuckelavee']);
  });
});
