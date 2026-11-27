---
name: broker-connector
description: Writes the investor's broker pack (fees, FX cost) from the broker's published price list and, if wanted, a read-only connector for holdings, fund yields and news. Use at first setup or when the broker changes.
---

# broker-connector

The broker pack is what an order costs. `pb portfolio check` prices every monthly buy with it, so the
fee rule is only as good as this file.

## Rules
- **Prices come from the broker's own published price list or fee page**, cited with the date in
  `sources`. Never from a forum, a comparison site alone, or memory.
- A market the broker does not offer is `offered: false`, never left at zero.
- A connector reads the investor's account **only through their own logged-in browser, read-only**:
  never type, store or print credentials, tokens or cookies; never spoof a User-Agent or bypass a bot
  check; stop and ask the investor to log in when a session is missing.
- Write in English, whatever language the investor reads.

## Steps: the broker pack
1. Ask the broker's name, and the markets the investor wants to buy on (their home exchange, US,
   London, Xetra, …).
2. From the price list, for each market key (the schema's `markets`): `currency`, `kind`
   (`share`/`etf`), `fixed`, `pct_fee`, `min`, `max`, and a venue tax the broker passes on
   (`tax_pct` with `tax_label`: UK stamp duty, the French FTT). The home market's own transaction
   tax belongs to the country pack, not here, so it is never charged twice.
3. `fx_cost_pct` (the conversion charge), `withholding_overrides` and `reit_withholding` where the
   broker withholds otherwise than the country pack says (no treaty relief filed, say), and
   `credits_line_currency` / `levy_no_exemption` if they apply.
4. `default_etf_market`, `market_by_country` and `default_market`: where a line is bought when the
   mix names no market.
5. Write `data/packs/broker/<name>.yaml` in the schema of
   `internal/pack/examples/examplebroker.yaml`, set `broker: <name>` in `data/config.yaml`, run
   `./pb packs`, then `./pb portfolio check` on any mix to see the fees it now prices.

## Optional: a read-only connector
Only if the investor wants holdings, the broker's fund yields or its news in the run. It is code in
the investor's own copy (a private fork), never in this repository:
1. Start from the example screener client (`internal/source/lightyear`: a public page, no login) and
   CONTRIBUTING.md's "Extension points": a package of its own that registers, keyed by the broker
   pack's `key`, any of `pbcli.RegisterScreener`, `RegisterScreenEnricher`, `RegisterBrokerIDResolver`,
   `livetool.RegisterBrokerFeed`, `RegisterCommand` (an import command), plus a skill that saves the
   logged-in pages from the investor's browser and runs the import.
2. It writes `data/stocks|etfs/<T>/broker.yaml` (`internal/broker`) and `data/portfolio/*.yaml`.
3. Tests use made-up data in the broker's shape; never commit a recorded response from the
   investor's account.
