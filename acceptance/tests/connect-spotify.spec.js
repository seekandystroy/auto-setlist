// Spec: acceptance/specs/connect-spotify.md
import { test, expect } from './support/fixtures.js';

const hellripper = {
  setlistfm: { artists: [{ name: 'Hellripper', setlists: [{ sets: [['All Hail the Goat']] }] }] },
};
const catalog = [{ name: 'All Hail the Goat', artists: ['Hellripper'] }];

test.describe('Connecting to Spotify', () => {
  test('A first-time visitor is asked to connect to Spotify', async ({ page, world }) => {
    await world.load({ ...hellripper, spotify: { users: [{ id: 'visitor' }], catalog } });

    await page.goto('/');

    await expect(page.getByRole('button', { name: 'Connect to Spotify' })).toBeVisible();
    await expect(page.getByText('(auto-setlist does not store any personal information)')).toBeVisible();
    await expect(page.getByPlaceholder('Artist name')).toBeHidden();
  });

  test('Connecting to Spotify leads to the artist form', async ({ page, world }) => {
    await world.load({ ...hellripper, spotify: { users: [{ id: 'visitor' }], catalog } });
    await page.goto('/');

    await page.getByRole('button', { name: 'Connect to Spotify' }).click();
    await page.getByRole('link', { name: 'Agree' }).click();

    await expect(page.getByPlaceholder('Artist name')).toBeVisible();
    await expect(page).not.toHaveURL(/code=/);
  });

  test("Cancelling on Spotify's authorization page leaves the visitor able to try again", async ({ page, world }) => {
    await world.load({ ...hellripper, spotify: { users: [{ id: 'visitor' }], catalog } });
    await page.goto('/');

    await page.getByRole('button', { name: 'Connect to Spotify' }).click();
    await page.getByRole('link', { name: 'Cancel' }).click();

    await expect(page).toHaveURL(/error=access_denied/);
    await expect(page.getByRole('button', { name: 'Connect to Spotify' })).toBeVisible();
  });

  test('A failed authorization tells the visitor why and asks them to connect again', async ({ page, world }) => {
    await world.load({ ...hellripper, spotify: { users: [{ id: 'visitor' }], catalog } });

    await page.goto('/?code=already-used-code');

    await expect(page.getByText('Spotify auth error: Invalid authorization code', { exact: true })).toBeVisible();
    await expect(page.getByRole('button', { name: 'Connect to Spotify' })).toBeVisible();
    await expect(page.getByPlaceholder('Artist name')).toBeHidden();
    await expect(page).not.toHaveURL(/code=/);
  });

  test('A returning visitor goes straight to the artist form', async ({ page, world, connectAs }) => {
    const visitor = { id: 'visitor', accessToken: 'visitor-access-token' };
    await world.load({ ...hellripper, spotify: { users: [visitor], catalog } });
    await connectAs(visitor);

    await page.goto('/');

    await expect(page.getByPlaceholder('Artist name')).toBeVisible();
    await expect(page.getByRole('button', { name: 'Connect to Spotify' })).toBeHidden();
  });

  test('An expired connection is renewed without asking the visitor', async ({ page, world, connectAs }) => {
    const visitor = { id: 'visitor', accessToken: 'expired-access-token', refreshToken: 'visitor-refresh-token' };
    // Spotify only honours the refresh token; the old access token is gone.
    await world.load({ ...hellripper, spotify: { users: [{ id: 'visitor', refreshToken: visitor.refreshToken }], catalog } });
    await connectAs(visitor, { expired: true });

    await page.goto('/');
    await expect(page.getByPlaceholder('Artist name')).toBeVisible();
    await page.getByPlaceholder('Artist name').fill('Hellripper');
    await page.getByRole('button', { name: 'Create Playlist' }).click();

    await expect(page.getByRole('link', { name: 'Listen on Spotify' })).toBeVisible();
    const [playlist] = await world.playlists();
    expect(playlist.owner).toBe('visitor');
  });

  test("A connection that can't be renewed asks the visitor to connect again", async ({ page, world, connectAs }) => {
    await world.load({ ...hellripper, spotify: { users: [{ id: 'visitor' }], catalog } });
    await connectAs({ accessToken: 'expired-access-token', refreshToken: 'revoked-refresh-token' }, { expired: true });

    await page.goto('/');

    await expect(page.getByRole('button', { name: 'Connect to Spotify' })).toBeVisible();
    await expect(page.getByPlaceholder('Artist name')).toBeHidden();
  });
});
