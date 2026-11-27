---
name: pb-repair
description: "Repairs the pb data tool when a ./pb command fails or returns data that no longer looks right: tells an outage or login problem from a vendor changing a payload, and fixes the code until the unchanged tests pass. Use when a command errors or make test goes red."
---

# pb-repair

`pb` is the data layer: every agent's numbers come from it, so a silent decoder break poisons every run. Tests are **baselined and frozen** (CONTRIBUTING.md, "Tests are frozen"): when something breaks, the **code** changes until the unchanged tests pass. This skill is the routine for that, and it is not routine work: repair only what is broken.

## Rules
- **Never edit a test file** (`*_test.go`) to make it pass. A test changed to agree with the code has stopped being evidence.
- **Refreshing a fixture (`testdata/`) needs the user's explicit approval**, asked with the evidence (the live response's new shape against the fixture's old one). A fixture in this repository holds made-up data only: never commit a response recorded from a real account or with real personal data.
- **Never spoof a User-Agent and never bypass a bot check, a challenge page or a rate limit.** A 403 or 429 is an identity or reputation answer, not a parser bug. A broker API that refuses non-browser clients is read only by a connector, in the user's own browser.
- Never print `.env` values, tokens or cookies. Run commands that need them through `make test-live`, which sources `.env` itself.
- `make build && make test` must be green before any commit; commit only when the user asks.

## 1. Reproduce
Run the failing `./pb …` command again and keep its exact error. Note the source it hits:

| Command | Source package | Credential |
|---|---|---|
| `pb ly screen` | `internal/source/lightyear` (public screener HTML) | none |
| `pb ep fetch / etf / macro / news`, `pb news search` (US) | `internal/source/eulerpool` | `EULERPOOL_API_KEY` |
| a broker connector's import, `pb news search\|read` (non-US) | the connector's own package (outside this repository) | the connector's |
| `pb packs` | `internal/pack` (data/config.yaml, data/packs/) | none |
| `pb edgar` | `internal/source/edgar` | none (`SEC_USER_AGENT` optional) |
| `pb search`, `pb fetch-url` | `internal/source/tavily`, `internal/agent/livetool` | `TAVILY_API_KEY` |
| `pb derive`, `pb portfolio check` | `internal/rules`, `internal/stock/*` (no network) | none |

## 2. Run the gates, cheapest first
```sh
make build && make test      # hermetic: no network, no .env
make test-live               # Eulerpool, Tavily, EDGAR, Lightyear screener
```
- **A red `make test`** with no code change is a real regression in code or a dependency: fix the code.
- **A skip is not a pass.** A missing credential makes a live test skip; `go test -v` prints `--- SKIP`. Report what was not exercised.
- **A timeout is not a broken parser.** `LIVE_TIMEOUT` defaults to 30m; check the elapsed time on the FAIL line before reading the trace, and re-run with `LIVE_TIMEOUT=60m` on a slow link.
- **Broker data missing after a connector's sync** is usually a login problem, not a bug: a sync stops on a 401 and asks the user to log in. A shape change shows as a decode error in the connector's import on a fresh bundle: compare the bundle's file with the connector's fixture.

## 3. Outage, identity, or shape change?
The tell of a stale fixture is the pair: **the fixture test passes and the live test fails.** Then classify the live answer (the test output carries the status and body):

| Symptom | Reading |
|---|---|
| 5xx, connection reset, TLS timeout, empty body on a route that used to answer | **outage**: re-run later; change nothing |
| 401 | **credential** expired or missing: the user renews it |
| 403 / 429 | **client identity or reputation**, not shape. Do not "fix" it with invented headers or a browser User-Agent |
| 200 with a different JSON or HTML shape: a renamed key, a moved column, a new nesting level | **vendor changed shape**: step 4 |
| 200 with an HTML challenge or consent page where data was | **moved behind a gate**: record it; `internal/httpx` detects challenges; do not get around it |
| 200, same shape, but a field now empty for every name | an upstream data change: record it as a `data_gaps` line in the decoder's output rather than loosening a test |

## 4. Fix the decoder, not the test
1. Find the decoder: the failing test names the package; grep the field or key it asserts (`grep -rn '<key>' internal/source/<pkg>/`).
2. Find its fixture. The hermetic tests carry made-up payloads in the vendor's shape, written inline in the test file:
   | Source | Test file (fixture inside) | Live gate (`make test-live`) |
   |---|---|---|
   | Lightyear screener | `internal/source/lightyear/decode_test.go` (`fakePage`: a made-up explore page) | `TestLiveExplorePage` (`PB_LIVE_LIGHTYEAR`) |
   | Eulerpool | `internal/source/eulerpool/macrofx_test.go` (a fake server) | `TestLiveProfile` (`PB_LIVE_EULERPOOL`) |
   | EDGAR | `internal/source/edgar/ffo_test.go` (made-up XBRL facts) | `TestLiveRegistrant` (`PB_LIVE_EDGAR`, `SEC_USER_AGENT`) |
   | Tavily | `internal/source/tavily/search_test.go` (a fake server) | `TestLiveSearch` (`PB_LIVE_TAVILY`) |
   A vendor shape with no hermetic test yet: add one in the same style (made-up values in the live shape) as part of the fix, with the user's approval; it is a new test, so it does not break the frozen-test rule.
3. Make the decoder accept the **new** shape while still passing the **old** fixture tests unchanged (both shapes, if the vendor may serve either). If the old shape is gone for good and a frozen test can no longer pass on real data, stop and ask the user: show the old and new shapes and propose a fixture refresh. Only with their approval refresh the fixture (made-up data in the new shape), and name the refreshed file in the commit message.
4. If the vendor stopped publishing something, the honest change is a recorded `data_gaps` line ("missing data is absent, never a zero"), not a loosened assertion.
5. Keep CONTRIBUTING.md's code rules: imports point downward, outputs through `docfmt.MarshalYAML` and `store.WriteFileAtomic`, comments one or two lines saying why.

## 5. Verify and report
```sh
make build && make test && make test-live
```
then re-run the original `./pb …` command. Report: which gates ran, skipped or failed; the failing test, package and elapsed time; outage, credential, identity or shape change, and what decided it; the decoder file changed and why; any fixture refresh and the user's approval for it. Tests must be byte-identical to before (`git diff --stat -- '*_test.go'` prints nothing).
