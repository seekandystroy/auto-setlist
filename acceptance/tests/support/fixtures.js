// Shared fixtures for the acceptance tests.
//
// Every test runs against the real web app (cmd/server), pointed at in-memory fakes of
// Setlist.fm and Spotify served by acceptance/fakeserver. Use the `world` fixture to describe
// what those services contain before the test, and to inspect what the app did to them afterwards.
import { test as base, expect } from '@playwright/test';
import { fakesURL } from '../../urls.js';

export { expect };

export const test = base.extend({
  page: async ({ page }, use) => {
    // Keep tests hermetic: no real third-party traffic.
    await page.route('https://kit.fontawesome.com/**', (route) => route.abort());
    // app.js talks to accounts.spotify.com directly; send it to the fake accounts service.
    await page.route('https://accounts.spotify.com/**', async (route) => {
      const url = new URL(route.request().url());
      const response = await route.fetch({
        url: `${fakesURL}/spotify-accounts${url.pathname}${url.search}`,
        maxRedirects: 0,
      });
      await route.fulfill({ response });
    });
    await use(page);
  },

  world: async ({ request }, use) => {
    await use(new World(request));
  },

  // connectAs(user, { expired }) makes the browser start as a user who connected to Spotify
  // earlier: the app finds their tokens in localStorage. `user` is a scenario user with an
  // accessToken (and a refreshToken, if the test needs one). Pass { expired: true } to
  // simulate tokens that have expired since. Call it before the first page.goto().
  connectAs: async ({ page, baseURL }, use) => {
    await use((user, options) => seedTokens(page, new URL(baseURL).origin, user, options));
  },
});

class World {
  constructor(request) {
    this.request = request;
  }

  // Replaces everything the fakes know. See acceptance/fakes/scenario.go for the shape:
  // { setlistfm: { artists: [...] }, spotify: { users: [...], catalog: [...] } }
  async load(scenario) {
    const response = await this.request.post(`${fakesURL}/control/scenario`, { data: scenario });
    expect(response.status(), await response.text()).toBe(204);
  }

  // Playlists created on the fake Spotify so far, oldest first:
  // [{ id, owner, name, description, tracks: [{ name, artists, uri }] }]
  async playlists() {
    const response = await this.request.get(`${fakesURL}/control/spotify/playlists`);
    expect(response.ok()).toBe(true);
    return response.json();
  }
}

async function seedTokens(page, origin, user, { expired = false } = {}) {
  await page.addInitScript(
    ({ origin, accessToken, refreshToken, expired }) => {
      // Seed once per tab, so the app is free to change or clear the tokens afterwards.
      if (window.location.origin !== origin || sessionStorage.getItem('acceptance:seeded')) return;
      sessionStorage.setItem('acceptance:seeded', '1');
      if (accessToken) localStorage.setItem('spotify_access_token', accessToken);
      if (refreshToken) localStorage.setItem('spotify_refresh_token', refreshToken);
      const hour = 60 * 60 * 1000;
      localStorage.setItem('spotify_expires_at', String(Date.now() + (expired ? -hour : hour)));
    },
    { origin, accessToken: user.accessToken, refreshToken: user.refreshToken, expired },
  );
}
