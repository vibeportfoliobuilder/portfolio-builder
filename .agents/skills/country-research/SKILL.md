---
name: country-research
description: Writes the country pack for the investor's residence (home currency, withholding, dividend tax, transaction taxes, bonds), every figure cited and dated. Use at first setup, on a move, or when a tax rule changes.
---

# country-research

The country pack is what the engine knows about where the investor lives. Every tax figure an agent
quotes comes from it, so a wrong rate here is wrong everywhere. Research it once, cite every figure,
and leave out what you cannot source.

## Rules
- **Every figure has a source and a date** in `sources` (a government page, the tax authority, a
  treaty text, a broker's published tax page). Use `./pb search "<question>"` and
  `./pb fetch-url <url>`; quote no more than a short phrase.
- **Never guess a rate.** A source country you cannot source is left out: the engine then shows a
  dash and flags the line ("withholding unknown"), which is honest. A zero is a claim that nothing is
  withheld.
- State the rate **withheld at source**, not a treaty rate the investor can only reclaim later; put
  the reclaimable part in a comment.
- Mark the pack at its top: "Researched by an AI from public sources on <date>; verify with a tax
  adviser." This is not tax advice.
- Write in English, whatever language the investor reads.

## Steps
1. Ask in one batch: the country of residence, the tax-advantaged account the plan is held in (if
   any), and whether the investor already knows any rate (their own figure wins, noted as theirs).
2. Research, with sources:
   - `currency` (ISO 4217) and `inflation_pct`: the central bank's long-run target, or the long-run
     average with its window; `inflation_note` says which.
   - `withholding`: for each country the investor may hold (start from the ones in
     `internal/pack/examples/exampleland.yaml` and the investor's own list), the rate a resident
     suffers at source; `reit_withholding` where a REIT's rate differs; `fund_withholding` by fund
     domicile (an Irish or Luxembourg UCITS fund usually distributes gross: state it only when sourced).
   - `unrated`: codes no single honest rate exists for.
   - `transaction_tax`: a tax on buying shares on the home market (`market` is the broker pack's
     market key for the home exchange; `rate_pct`, `cap` and `exempt_up_to` in the home currency), or
     `null` when there is none.
   - `account`: the tax-advantaged account's name, and in a comment its rules (deposit windows,
     lock-up, what tax it removes).
   - `bonds`: the government bonds a retail investor buys (`name`, `currency`, `yields_pct`: one or
     two current yields with the date, `tax_free` if so).
   - `macro_areas`: the areas `pb ep macro` always covers (the home country, US, EU and the income
     currencies' areas).
3. Write `data/packs/country/<name>.yaml` in the schema of `internal/pack/examples/exampleland.yaml`
   (`./pb packs` prints the active one), set `country: <name>` in `data/config.yaml`, and run
   `./pb packs` to check it loads (a schema error names the line).
4. Show the investor the table of rates with their sources, and the gaps you left out. Write a
   memory (`data/memory/`) with the date researched, so a later run knows when to refresh it.
