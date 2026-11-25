---
name: profile
description: Rebuilds or updates data/profile/investor_profile.md, the investor profile every agent reads first, from the user's own statements. Keeps it a standalone document with no citations, shows the user a diff, and writes it only after their approval. Use when the user states a new goal, rule, preference, cap, belief or circumstance, when /portfolio-builder marked answers REVISED, or when the user asks to review or rebuild the profile.
---

# profile

`data/profile/investor_profile.md` is the investor. Every agent and skill reads it first, and it wins over any older plan, note or questionnaire. It is gitignored (personal), so this skill keeps its own history.

## Rules
- **The user's statements are the only source.** Their words in this conversation, and for a rebuild `data/profile/raw/user_messages.md` and `data/profile/raw/user_answers.md` (their earlier messages and questionnaire answers, newest statement wins). Never put in a belief, a rule or a number the user did not state. Agents' findings (a verdict, a challenge in `idea_challenge.md`) are not profile content unless the user adopts them in their own words.
- **Standalone, no citations.** The file explains the investor without needing any other file: no source links, no session ids, no "per memory" notes. Plain, short sentences.
- **Keep the structure:** 1 Snapshot, 2 Situation, 3 Goals and beliefs, 4 Risk, 5 Tax, 6 Strategy rules (groups, safe-dividend rules, market cap, currencies and reinvestment matching, stock and ETF split, caps, ETFs, growth sleeve, liked names, exclusions), 7 Sectors and regions, 8 Broker and costs, 9 How agents should work, 10 Not yet settled. Add a section only if a statement fits none of these.
- **Bump the version line** (`Version YYYY-MM-DD, reviewed by the investor.`) to today on every approved change.
- **Contradictions:** when a new statement conflicts with the file, the newest statement wins, but ask the user to confirm it in the same batch as the diff; never resolve a conflict silently. An item the user leaves open goes to §10 as open: never with a default number.
- **Secrets:** never write account ids, balances, tokens, passwords or personal identifiers (email, phone, ID numbers) into the profile.
- Every number the code judges by lives in `data/profile/strategy.yaml` (below), written with the profile from the same statements; the country-specific facts (bond yields, inflation, withholding, transaction taxes) live in the country pack and the broker's prices in the broker pack (`./pb packs`). Changing a strategy number is an edit of that file (this skill), never a code change.

## A first profile: the questionnaire
With no profile yet, ask the questionnaire (the Q1–Q38 list in /portfolio-builder, step a), in the language `./pb packs` names, one topic per batch: situation, goals, risk, structure, the future, sectors, process. Save the user's answers verbatim to `data/profile/raw/user_answers.md`, then write the first profile from them with the steps below (step 1 has no old file to read; show the whole file instead of a diff).

## The strategy file: the numbers the code judges by
`data/profile/strategy.yaml` holds every limit `pb` applies, and **nothing has a default**: a rule
whose limit is unset is skipped and says so. Write it with the profile, from the user's own answers
only (the questionnaire's structure, sectors and process topics), and show it with the profile diff.
Its keys (all optional; percent unless named; `./pb packs` prints `strategy_unset`, the keys still open).
A misspelled key is an error, so copy the names exactly:
- the money (home currency): `start_amount`, `monthly` (what a mix with no amounts is projected on),
  `target_monthly` (the net monthly income the switch searches for), `higher_monthly` (a higher
  contribution shown beside the mix's own);
- groups: `group_yield_pct`, `goat_growth_pct`, `grower_growth_pct`, `stable_grower_years`,
  `stable_payer_years`, `low_yield_pct`, `large_cap_only_groups` (a list of group names);
- total-return flags: `high_yield_flag_pct`, `high_yield_min_price_cagr_pct`, `min_yield_plus_growth_pct`;
- safe dividend: `max_payout_pct`, `max_reit_payout_pct`, `max_debt_to_fcf` (a multiple), `min_cover` (a multiple);
- portfolio: `etf_min_pct`, `etf_max_pct`, `weight_cap_pct`, `income_cap_pct`, `etf_income_cap_pct`,
  `moonshot_income_cap_pct`, `currency_income_cap_pct` (a map, e.g. `{CAD: 25}`), `fee_max_pct`,
  `yoc_year10_pct`, `price_growth_pct`, `price_growth_tolerance_pct`;
- the projection: `projection_growth_haircut` (a factor, e.g. 0.75) and `projection_growth_cap_pct`
  (what a dividend growth read from history is shrunk by and capped at; unset: none), and
  `stock_reinvest_currencies` (a list: currencies whose dividends go back into the stocks bought in
  them, because no suitable fund trades in them);
- the switch to income: `switch_years` (a list), `switch_sell_pct`, `switch_stock_share_pct`, `bond_cap_pct`;
- buying on weakness: `double_down_etf_drop_pct`, `double_down_stock_drop_pct`, `double_down_multiple`;
- the screen: `income_currencies` (a list), `excluded_tickers` (a map: ticker to why; also looked for
  inside the funds), `excluded_fund_words` (a list, e.g. `bond`), `liked_tickers` (a list);
- `custom:` holds keys a private layer's own rules read (anything else is an error).
When the user has no view on a number, ask once; if they still have none, leave the key out and
list it in profile §10 ("not yet settled"). Never copy a number from an example, a memory or another
investor. `./pb portfolio check` and `./pb derive` then report which rules were skipped.

## Steps
1. Read `data/profile/investor_profile.md` and `data/profile/strategy.yaml` whole.
2. Collect what changed: the user's new statements (ask in **one batch** for anything unclear: exact figures, which of two conflicting statements stands, whether a preference is a hard rule or a liking).
3. Write the proposed files to `data/profile/investor_profile.proposed.md` and `data/profile/strategy.proposed.yaml`. Edit only the passages and keys the statements touch, unless the user asked for a rebuild.
4. Show the diff and a one-paragraph summary of what changes for the agents:
   ```sh
   diff -u data/profile/investor_profile.md data/profile/investor_profile.proposed.md
   diff -u data/profile/strategy.yaml data/profile/strategy.proposed.yaml
   ```
5. **Wait for the user's approval.** Apply their corrections and show the diff again if they change anything.
6. On approval: keep the old version as `data/profile/history/investor_profile_<its version date>.md` (create the folder if needed), then move the proposed files over `data/profile/investor_profile.md` and `data/profile/strategy.yaml` (the strategy's old version beside the profile's in `history/`). On rejection, delete the proposed files.
7. Tell the user which rules in code, if any, now disagree with the profile, and which agent outputs in the newest `data/runs/<date>/` were written under the old profile.
