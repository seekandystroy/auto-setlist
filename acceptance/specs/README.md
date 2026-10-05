# Feature specs

One file per user-facing feature of the web app. A spec describes what a **user** experiences, not how the code works. Each spec is the source for one Playwright file in `acceptance/tests/`.

## Format

```markdown
# <Feature name>            ← becomes the test.describe() title

<One paragraph: who the feature is for and what it lets them do.>

## Scenarios

### <Scenario title>        ← becomes the test() title, verbatim

- **Given** <the state of the world: Setlist.fm data, Spotify catalog, the user's connection>
- **When** <what the user does in the browser>
- **Then** <what the user sees, and what ends up in their Spotify account>
```

Rules:
- One scenario for the happy path, plus one for **every way the user's experience differs from it** (an error, a fallback, a different result). Internal resilience (retries, rate limiting, parsing details) belongs in unit tests, not here.
- Scenario titles are unique within a spec, written as a plain sentence, and never renamed casually: the matching test has the same title.
- Use concrete examples (specific artists, songs, tours) only where the example *is* the rule, e.g. an ordering, a matching rule, or exact output. Otherwise describe the situation generically ("an artist whose latest show…"). Test files always need concrete data; the spec doesn't.
- Write user-visible text (button labels, messages, playlist names) exactly as it should appear. Mark the parts that vary with placeholders, e.g. `no setlists found for "<artist>"`.
- Specs describe what the app actually does. When that behavior is worth improving, keep the spec accurate and add the improvement to "Future improvements" in the root `README.md`. Changing the behavior later means changing the spec first.
