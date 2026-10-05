import { defineConfig, devices } from '@playwright/test';
import { appPort, appURL, fakesPort, fakesURL } from './urls.js';

// Set ACCEPTANCE_LOGS=1 to see the app's and the fakes' logs.
const logs = process.env.ACCEPTANCE_LOGS ? 'pipe' : 'ignore';

export default defineConfig({
  testDir: './tests',
  // The fake server holds one shared world, reset by each test, so tests run one at a time.
  workers: 1,
  fullyParallel: false,
  forbidOnly: !!process.env.CI,
  reporter: process.env.CI ? 'line' : 'list',
  use: {
    baseURL: appURL,
    trace: 'retain-on-failure',
  },
  projects: [{ name: 'chromium', use: { ...devices['Desktop Chrome'] } }],
  webServer: [
    {
      command: 'go run ./acceptance/fakeserver',
      cwd: '..',
      env: { PORT: fakesPort },
      url: `${fakesURL}/control/health`,
      reuseExistingServer: !process.env.CI,
      timeout: 120_000,
      stdout: 'ignore',
      stderr: logs,
    },
    {
      // The real web app, exactly as deployed, pointed at the fakes.
      command: 'go run ./cmd/server',
      cwd: '..',
      env: {
        PORT: appPort,
        SETLISTFM_API_KEY: 'fake-setlistfm-key',
        SPOTIFY_CLIENT_ID: 'fake-spotify-client-id',
        SPOTIFY_CLIENT_SECRET: 'fake-spotify-client-secret',
        SETLISTFM_BASE_URL: `${fakesURL}/setlistfm`,
        SPOTIFY_API_BASE_URL: `${fakesURL}/spotify-api/v1`,
      },
      url: `${appURL}/`,
      reuseExistingServer: !process.env.CI,
      timeout: 120_000,
      stdout: 'ignore',
      stderr: logs,
    },
  ],
});
