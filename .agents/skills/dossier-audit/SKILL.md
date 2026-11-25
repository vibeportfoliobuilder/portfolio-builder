---
name: dossier-audit
description: Audits agent output in data/runs/<date>/*.md against house rule 1 - every figure must trace to a data/ file, an upstream agent file the agent was entitled to read, or a ./pb output that left a ledger row. Use after a /portfolio-run (before gate 3 or publishing), after any single agent or skill wrote a run file, or when asked whether an analysis invented a number.
---

# dossier-audit

House rule 1 of every agent and skill is the load-bearing rule of the whole run, and **no code checks it**:

> **Never fabricate a number.** Every figure must trace to a file under `data/`, to an upstream file in this run's folder, or to a `./pb …` command you actually ran. If you don't have it, write `DATA UNAVAILABLE` and proceed.

A tool that reaches outside writes down what it found before the agent sees it (`./pb search` and `./pb fetch-url` append to `data/stocks/<T>/sources.yaml` or `data/research/sources.yaml`; `./pb news search` writes `news.yaml`), so "a command you actually ran" leaves a row. A figure whose only provenance is a tool call must resolve to a ledger row; one that resolves to nothing is a finding. `./pb news read` stores nothing: a figure from it is **tool-only**.

Run from the repo root. `RUN=data/runs/<date>`.

## 1. The allowed source set per file
Every agent reads `data/profile/investor_profile.md`, `data/watchlist.yaml`, and per name `data/stocks/<T>/*.yaml` or `data/etfs/<T>/*.yaml`, plus `data/macro.yaml`. On top of that, upstream files in `$RUN`:

| File | Writer | Upstream files it may cite |
|---|---|---|
| `moat_review.md` | moat-reviewer (Hamilton) | `screen_notes.md` |
| `ceo_review.md` | ceo-filter (William) | `moat_review.md`, `screen_notes.md`; `data/stocks/<T>/earnings_call*.pdf` |
| `scenarios.md` | scenario-analyst | `moat_review.md`, `ceo_review.md`, `screen_notes.md` |
| `news_macro_<date>.md` | news-macro-analyst (Nellie) | `moat_review.md`, `ceo_review.md`, `screen_notes.md`, an earlier run's `verdicts.md`, `data/market_news.yaml` |
| `verdicts.md` | committee-chair (Charlie) | `moat_review.md`, `ceo_review.md`, `scenarios.md`, `news_macro_<date>.md`, `screen_notes.md` |
| `fit.md` | portfolio-strategist (Harry) | all of the above, `data/portfolio/*.yaml`, an earlier `allocation_final.yaml`, `fit_probe.yaml` + `fit_probe_check.yaml` |
| `idea_challenge.md`, `blueprint.md`, `allocation_{balanced,income,growth}.yaml` | portfolio-builder (Jack) | all of the above, `check_<mix>.yaml` |
| `stress.md` | stress-tester (Nassim) | `allocation_<mix>.yaml`, `check_<mix>.yaml`, `stress_dividend_<mix>.yaml`, `check_stress_<mix>.yaml`, `verdicts.md`, `fit.md`, `scenarios.md`, `data/portfolio/*.yaml` |
| `screen_notes.md` | screen-analyst (Joel) | `data/screen/*.yaml` |
| `news_macro_<date>_final.md` | news-macro-analyst, scope: final mix | as `news_macro_<date>.md`, plus `allocation_final.yaml` and the run's `verdicts.md` |
| `rebalance_<date>.md` | portfolio-builder outside a run | `allocation_final.yaml`, `check_final.yaml`, `data/portfolio/*.yaml`, the run's `verdicts.md` and the newest `news_macro_*.md` |

A figure that traces only to a file outside the writer's set is a provenance violation even though the file exists (Hamilton citing `verdicts.md` of the same run, say, which did not exist when he wrote).

## 2. Extract every figure
```sh
F=$RUN/verdicts.md
grep -nE '[-+]?[0-9][0-9,.]*\s*(%|bps|x|×)?|[$€£][0-9]|[0-9]+(\.[0-9]+)?\s*(bn|B|m|M|k|K|tn|T|USD|EUR|GBP|[A-Z]{3})\b' "$F"
```
Skip what is not a claim about a company or the portfolio: heading and gate numbers, dimension numbers, a year used only as a label, the horizon ("5–10 years"), the profile's own targets and caps quoted as such, and arithmetic the file states on figures it already cited (a CAGR is traceable if both ends are).

## 3. Trace each one, by the section's ticker
Stop at the first hit:
```sh
T=KO; N='<the number, and without separators>'
grep -n "$N" data/stocks/$T/{financial_data,derived,human_view,broker,edgar}.yaml 2>/dev/null
grep -n "$N" data/etfs/$T/*.yaml data/macro.yaml data/portfolio/*.yaml 2>/dev/null
grep -n "$N" data/stocks/$T/{sources,news,news_eulerpool}.yaml data/research/sources.yaml 2>/dev/null
for u in <the upstream files allowed in step 1>; do grep -Hn "$N" "$RUN/$u"; done
```
Allow for restatement: `financial_data.yaml` holds money in **millions** of the reporting currency (its `units:` line says so), so `$10.5bn` and `10540.8` are the same figure. Allow rounding to the precision the output states, and no further. The human view figures must come from `human_view.yaml`; a fair value or moat rating found nowhere there is invented (rule 7: moat and fair value come from humans).

Classify each figure:
- **traced**: names its file, and the value is there at the stated precision;
- **derived**: arithmetic on traced figures, with the inputs stated;
- **untraceable**: in no allowed input. **This is the finding**;
- **tool-only**: attributed to `./pb news read` or another call with no ledger row. Report separately: unverifiable by construction.

## 4. An empty field must have produced DATA UNAVAILABLE
For each block the agent's prompt tells it to use, check the data is actually there; where it is not, the output must say `DATA UNAVAILABLE` (or "no human view"), not carry a number:
```sh
python3 -I - data/stocks/$T/financial_data.yaml <<'EOF'
import sys, yaml
d = yaml.safe_load(open(sys.argv[1]))
def walk(o, p=''):
    if isinstance(o, dict):
        if not o: print('EMPTY', p)
        for k, v in o.items(): walk(v, f'{p}.{k}')
    elif isinstance(o, list):
        if not o: print('EMPTY', p)
    elif o in (None, ''): print('UNSET', p)
walk(d)
EOF
grep -c 'DATA UNAVAILABLE' "$F"
```
Known absences worth checking by name, because an agent is asked about each: **beta** (not in the data: a stated beta must be labelled a proxy), **earnings surprises** (no beat history), **Altman Z** (always 9999 from the vendor, so never carried) and **Piotroski** (not carried), **Form 4 transaction codes**, insider data outside the US, a **governance block** (none; a non-US board or pay figure needs a cited source), `human_view.yaml` missing (no human-view source), `broker.yaml` missing (no connector sync), and every line in each file's `data_gaps:` (frozen non-US estimates, no special-dividend flag, statements ending early).

## 5. Sources and dates for anything external
Every external claim in the body needs a matching entry in the file's *Sources* or *Data gaps & caveats* section with **both** a named source and a date. A source without a date is a finding: the date is how a reader tells last quarter's claim from today's.

## 6. Report
One table: file, line, ticker, figure, verdict, and the input it should have traced to. Then a verdict line per file: `CLEAN` / `N untraceable` / `N tool-only`. Write it to `$RUN/audit.md` (the Artifact's Watchlist page shows it; `pb report build` also reads an older `dossier_audit.md`).

**Do not repair the output by hand.** Re-run the agent with the missing input present (fetch the data with `./pb`, run the broker connector's sync or the human-view step), or accept the `DATA UNAVAILABLE` that should have been there. Editing a produced analysis to add a provenance it did not have makes the audit worthless: the file is evidence of what the model actually did.
