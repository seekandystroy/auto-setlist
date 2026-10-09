# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project

auto-setlist turns concert setlists into Spotify playlists. Given an artist, it takes their latest show (or every song from the tour of that show) from Setlist.fm and creates a playlist in the user's Spotify account. Given a Setlist.fm setlist link instead, it uses that specific show. Covers can fall back to the original artist's recording.

There are two entry points that share the same core:
- **Web app** (`cmd/server`): a vanilla JS frontend in `static/` plus a JSON API (`POST /setlistjob`, which takes either `artist` or a Setlist.fm `url`). Spotify auth (PKCE) happens in the browser, and the token is sent in the `Autosetlist-Spotify-Token` header. Deployed on Render.
- **CLI** (`cmd/cli`): `auto-setlist [--include-covers|-ic] [--tour-playlist|-tp] <artist>`, or `auto-setlist [--include-covers|-ic] --url|-u <setlist.fm link>`. Spotify auth goes through a local callback server, and the token is cached in `./spotify_token.json`.

Both need `SETLISTFM_API_KEY`, `SPOTIFY_CLIENT_ID` and `SPOTIFY_CLIENT_SECRET`. The server also reads `PORT` (default 3000), plus `SETLISTFM_BASE_URL` and `SPOTIFY_API_BASE_URL`, which are optional, default to the real APIs, and exist so the acceptance tests can point the app at fakes.

## Dependencies

- **Go: standard library only.** Never add a Go module dependency, including in tests or in `acceptance/`.
- **Frontend runtime: vanilla JS only.** No frameworks (React, Vue, …) and no build step for `static/`.
- **Frontend dev tooling is allowed.** Playwright lives in `acceptance/package.json`.

## Commands

```bash
go build ./cmd/...                              # build both binaries
go vet ./...                                    # static analysis
go test ./...                                   # all Go unit tests
go test -run TestName ./internal/adapters/      # a single Go test

# Acceptance tests (Playwright; Node required). Run from acceptance/:
npm ci && npx playwright install chromium       # first time only
npx playwright test                             # whole suite (starts the fake server and the app itself)
npx playwright test tests/covers.spec.js        # one spec file
npx playwright test -g "Pressing Enter"         # one scenario by title
ACCEPTANCE_LOGS=1 npx playwright test              # include app and fake logs
```

Before calling any change done, run all three: `go vet ./...`, `go test ./...` and `npx playwright test`.

## Architecture

Hexagonal architecture (Ports & Adapters):

- **`internal/core/domain/`**: pure structs (`Artist`, `Setlist`, `Track`) and domain logic only.
- **`internal/ports/`**: interfaces only. The service depends only on these, never on concrete adapters.
- **`internal/adapters/`**: implementations of the ports.
  - Driven adapters: Setlist.fm, Spotify, and the CLI's OAuth callback.
  - Driving adapter: the HTTP API (`api_adapter.go`).
  - Constructors take functional options for overrides (`WithSetlistfmBaseURL`, `WithSpotifyAPIBaseURL`).
- **`internal/core/service/`**: the business logic. It receives adapters through ports.
- **`cmd/server/`** and **`cmd/cli/`**: the composition roots. They read config (env vars, flags), build the adapters and the service, and run. All wiring lives here, never in `internal/`.
- **`internal/log.go`** (package `applog`): request-scoped `slog` logger in the context, request IDs, client IP.

## Tests

There are three layers. Pick the lowest one that can show the behavior, and add an acceptance scenario whenever the user's experience changes.

1. **Adapter unit tests** (`internal/adapters/*_test.go`):
   - `net/http/httptest` servers, with adapters built as struct literals so tests can inject `sleepFn`, base URLs and so on.
   - Cover protocol details: request shape, status handling, retries, 429, parsing, track matching rules.
2. **Service unit tests** (`internal/core/service/service_test.go`): inline mock structs for the ports (`mockSetlistfm`, `mockSpotify`). They cover orchestration and domain rules, such as merging tour setlists.
3. **Acceptance tests** (`acceptance/`): full stack in a real browser.
   - Playwright starts two processes: `acceptance/fakeserver`, which serves in-memory fakes of Setlist.fm, the Spotify Web API and Spotify Accounts (`acceptance/fakes`), and the real `cmd/server`, pointed at the fakes through the base URL env vars. The browser's calls to `accounts.spotify.com` are routed to the fake accounts service by the fixtures in `acceptance/tests/support/fixtures.js`.
   - The fakes answer like the real APIs do. For example, Setlist.fm returns 404 for an empty search, and Spotify returns a plain-text 403 for users outside the allowlist. Keep them faithful to the official API docs when extending them; don't bend a fake to make a test pass.
   - Tests describe the world with `world.load({ setlistfm, spotify })` (shape in `acceptance/fakes/scenario.go`), start connected with `connectAs(user)`, and check results with `world.playlists()`. Assert on what the user sees (roles, labels, text) and on what ends up in their Spotify account, not on internals.
   - No acceptance tests for the CLI.

No mocking libraries anywhere. Hand-written fakes and mocks only.

## Feature workflow (spec → failing tests → code)

Any change to what a user experiences in the web app follows these steps, in order:

1. **Spec.** Write or update `acceptance/specs/<feature>.md` following `acceptance/specs/README.md`: Given/When/Then scenarios, one for the happy path and one for each way the experience differs from it. Show the spec to the user and **stop until they approve it**.
2. **Failing acceptance tests.** In `acceptance/tests/<feature>.spec.js`, use `test.describe('<spec title>')` with one `test('<scenario title>')` per scenario. Titles must match the spec verbatim. Extend the fakes if the feature touches new API endpoints. Run the tests and confirm that the new scenarios fail for the expected reason (the missing behavior, not a broken test), and that existing ones still pass.
3. **Code.** Implement with unit tests at the right layer (see "Tests"), keeping the hexagonal boundaries. Iterate until `go vet ./...`, `go test ./...` and `npx playwright test` all pass.
4. **Docs.** Update the README's "Current state", and add or remove items under "Future improvements".

Rules for specs:
- Specs describe what the app actually does.
- If you notice behavior worth improving that the user didn't ask to change, don't fix it silently. Keep the spec accurate and propose the improvement under "Future improvements" in `README.md`.
- To change existing behavior, change the spec first, then the test, then the code.

## Conventions

- Errors are wrapped with context (`fmt.Errorf("fetching setlists for %q: %w", …)`). Today the web UI shows the error text to users as-is, so wording changes in errors are user-visible and acceptance tests pin them.
- "Nothing found" is not a failure. Setlist.fm answers empty searches and artists without setlists with 404, so the adapter returns empty results for a 404 (and doesn't retry it). The service turns empty results into the user-facing errors (`Artist not found`, `no setlists found for "…"`, `songs from setlistfm for "…" not found on Spotify`) and never creates an empty playlist.
- In `static/`, never use `innerHTML`: error texts can contain what the user typed. Build content with the `el()` helper in `app.js` (strings become text) or `textContent`.
- Log through `applog.LoggerFromCtx(ctx)` so request IDs follow the request.
- Commit messages use Conventional Commits (`feat:`, `fix:`, `chore:`, `feat(webapp):`, …).
