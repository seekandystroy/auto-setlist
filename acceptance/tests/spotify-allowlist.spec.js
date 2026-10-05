// Spec: acceptance/specs/spotify-allowlist.md
import { test, expect } from './support/fixtures.js';

test.describe('Spotify allowlist', () => {
  test('A visitor outside the allowlist is told to contact the maintainer', async ({ page, world, connectAs }) => {
    const outsider = { id: 'outsider', accessToken: 'outsider-access-token', notAllowlisted: true };
    await world.load({
      setlistfm: { artists: [{ name: 'Hellripper', setlists: [{ sets: [['All Hail the Goat']] }] }] },
      spotify: { users: [outsider], catalog: [{ name: 'All Hail the Goat', artists: ['Hellripper'] }] },
    });
    await connectAs(outsider);
    await page.goto('/');

    await page.getByPlaceholder('Artist name').fill('Hellripper');
    await page.getByRole('button', { name: 'Create Playlist' }).click();

    await expect(
      page.getByText(
        'spotify: users outside of allowlist not supported. Contact the maintainer to use auto-setlist.',
        { exact: true },
      ),
    ).toBeVisible();
    expect(await world.playlists()).toEqual([]);
  });
});
