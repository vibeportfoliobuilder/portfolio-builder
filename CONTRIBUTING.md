# Contributing

Everything needed to work on the code: `pb` (the Go data binary), the packs, the agent and skill
files, the report template and the tests. AGENTS.md is for the portfolio agent; this file is for
coding sessions.

## Toolchain, build and tests
- Users download a kit from the Releases page (`scripts/release.sh`, run by
  `.github/workflows/release.yml` on a version tag: `git tag v0.1.0 && git push origin v0.1.0`).
  Developers build from source:
- Go 1.26.2 or later. `make build` → `./pb`; `make test` is hermetic (no network, no `.env`);
  `make test-live` sources `.env` and runs the `PB_LIVE_*` gates (a gate without its key skips).
- **Tests are frozen.** Every existing `*_test.go` is a baseline: when something breaks, fix the
  **code** until the unchanged tests pass. New code gets new tests, which are then frozen too.
  Changing a test needs a stated reason in the pull request.
- **Fixtures are made-up data.** A fixture in this repository (`testdata/`, the test packs) holds
  invented figures in a real shape: never commit a response recorded from a real account, or real
  personal data. Unit tests run on the test packs (`internal/pack/packtest`: Testland, currency XTS,
  ISO 4217's code for testing, and Test Broker); start a package's tests with
  `packtest.Activate()` in `TestMain`.

## Packs: where the investor lives and where they buy
`internal/pack` holds a **country pack** (home currency, inflation target, withholding per source
country, REIT and fund-domicile rates, a home-market transaction tax, the switch's bond yields, the
macro areas), a **broker pack** (fees per market, FX cost, withholding overrides, whether it
credits the line's currency, the default markets) and the investor's **strategy**
(`data/profile/strategy.yaml`, `pack.Strategy`: every limit the rules judge by, the amounts and the
switch; nothing has a default, an unset key skips its rule, and an unknown key is an error).
`data/config.yaml` names the active pair;
`data/packs/<kind>/<name>.yaml` is read first, else a built-in. With no config the made-up
examples (`internal/pack/examples/`) are active. Code never names a country or a broker: it reads
`pack.Active()`, and code never holds an investor's number: it reads `pack.Active().Strategy`. A
broker whose prices are more than a commission formula registers a `pack.FeeModel`
(`pack.RegisterFeeModel`).

## Extension points (for a connector kept outside this repository)
A connector adds **files** to this tree; it never edits a public file, so it rebases cleanly. It
lives in a package of its own (`internal/<name>/…`) whose `init` registers it, and it is linked into
`pb` by one more file next to `main.go`, for example `private.go`:
```go
package main

import _ "github.com/vibeportfoliobuilder/portfolio-builder/internal/mybroker"
```
The hooks (names are case-insensitive; a broker hook is keyed by the broker pack's `key`, so it
runs only while that pack is active, and several connectors can live side by side):
- `pack.Register(kind, name, yaml)` and `pack.RegisterFeeModel(name, model)`: built-in packs.
- `pbcli.RegisterCommand(func(*pbcli.Env) *cobra.Command)`: a top-level command; `Env` offers
  `Path`, `ReadYAML`, `WriteYAML`, `PrintJSON`, `Today`, `Markdown`, `Out`, `ReadWatchlist`.
- `pbcli.RegisterScreener(brokerKey, open)`: the screener `pb ly screen` reads for that broker
  (default: the public example).
- `pbcli.RegisterScreenEnricher(brokerKey, f)`: `pb screen enrich --broker`.
- `pbcli.RegisterBrokerIDResolver(brokerKey, f)`: the broker's instrument id (a string: a number or
  a UUID) for `pb news`.
- `livetool.RegisterBrokerFeed(brokerKey, open)`: the broker's news feed for non-US names.
- `pbcli.RegisterReportExtension(f)` with `report.RegisterPage(page)`: data under the report's
  `extra`, and an extra report page (CSS, navigation button, view, script).
- `rules.RegisterNameCheck(f)` and `rules.RegisterPortfolioCheck(f)`: a rule of its own on every
  name's verdict or every portfolio check (an account's yearly deposit limit, say); it reads its
  limits from the strategy's `custom:` block.
- The per-instrument file a connector writes is `broker.yaml` (`internal/broker`), the account
  files go under `data/portfolio/`, the human view is `human_view.yaml` (its `broker_id` feeds the
  news feed).

## Code rules
- Imports point downward: `internal/pbcli` → `internal/rules` → `internal/stock/*`, `internal/pack`;
  `internal/report` is rendering only; `internal/source/*` → `internal/{httpx,cache,store,docfmt,core}`.
  Sources never import each other or the CLI.
- Every output file is written with `docfmt.MarshalYAML` (two-space YAML) through
  `store.WriteFileAtomic`; JSON stored on disk uses a two-space indent; JSON on stdout is compact (a top-level list one element per line, an object one key per line), to keep agent reads small.
- `data/` holds normalized YAML only, latest version overwritten in place. Raw responses go to the
  cache under `os.UserCacheDir()/pb`, never into the repository.
- Missing data is absent (`nil`, `needs_look`), never a zero. Comments: one or two lines saying why.
- Never print or commit `.env` values, tokens, cookies, account ids or balances.
- **Never spoof a browser User-Agent or bypass a bot check.**
- The report template's `<pb-model>` block mirrors `rules.ProjectYoC` and `rules.ProjectSwitch`;
  `internal/pbcli/model_test.go` runs it in node against pb, so change both together.

## Package map
```
main.go                      loads .env, runs internal/pbcli
internal/pbcli               the cobra tree; one file per command group
internal/pack                country and broker packs; examples/ (made up), packtest/ (tests)
internal/broker              the per-instrument schema a connector writes (broker.yaml)
internal/rules               the strategy in code: groups, safe dividend, cap bands, withholding,
                             the portfolio check, the yield-on-cost path, the switch to income
internal/report              pb report build: the page template and its extension slots
internal/source/eulerpool    Eulerpool API: dossiers, ETFs, macro, news; disk cache
internal/source/lightyear    Lightyear's public screener (no login): the example screener connector
internal/source/edgar        SEC filings index, FFO/AFFO from XBRL
internal/source/tavily       web search
internal/stock/screen        where a dividend is sourced, withholding tiers
internal/stock/divscore      the dividend safety score
internal/stock/returns       price and total returns, dividend growth, specials
internal/agent/livetool      search, fetch-url and news tools with citation ledgers
internal/{httpx,cache,store,docfmt,core,dotenv}   transport, raw cache, atomic writes, YAML
```

## Agents and skills
- Skills live once in `.agents/skills/` (Codex reads it); `.claude/skills` and `.grok/skills` are
  links to it. Keep a skill's `name` and `description` frontmatter; name no assistant-specific tool
  in a shared skill without saying what to do where it is absent.
- Analyst subagents are `.claude/agents/*.md`. An assistant without subagents runs each file as a
  prompt of its own, in the order AGENTS.md gives.
- Prompts never hold an investor's personal facts, a country's rates or a broker's prices: those come
  from the profile and the packs.
