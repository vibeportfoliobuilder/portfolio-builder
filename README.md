# Vibe Portfolio Builder

Build a long-term dividend portfolio with an AI assistant. A team of AI analysts screens the market,
researches the companies you pick, and suggests three portfolio mixes with a stress test. You make
every decision on one page in your browser.

You do not need to be a programmer. You need a computer, an AI coding assistant and two free keys.

> **Not investment advice.** This is research software. Every verdict, mix and figure it shows is
> input for your own decision. Check every tax figure with a tax adviser.

> **What has been tested:** macOS with Claude Code. Windows, Linux, **Codex** and **Grok Build**
> should work (the files follow their formats) but are **untested**. Tell us what you find.

## What you get

A run has three moments where you decide:
1. **Screen.** About 100 stocks and 20–30 funds. You tick 25 to 35 stocks and the funds you like.
2. **Watchlist.** Six AI analysts (moat, CEO, scenarios, news, committee, fit) judge every name you
   picked. You choose 10 to 15 names the portfolio may use.
3. **Portfolio.** Three mixes (balanced, income, growth) with a stress test. You choose one and edit it.

Three rules hold everywhere: figures come from code, never from the AI's memory (a missing figure is
written as `DATA UNAVAILABLE`); a company's moat and fair value come from human analysts, never from
the AI; and every suggestion is judged against *your* profile and strategy, which you set at the start.

## Set up, step by step (about 20 minutes)

**1. Install an AI coding assistant.** Claude Code is the tested one:
https://claude.com/claude-code. (Codex or Grok Build: untested.)

**2. Download Vibe Portfolio Builder.** Open the project's **Releases** page on GitHub and download
the zip for your computer:
- Mac with Apple chip (M1 or newer): `…-darwin-arm64.zip`
- Mac with Intel chip: `…-darwin-amd64.zip`
- Windows: `…-windows-amd64.zip`
- Linux: `…-linux-amd64.zip`

Unzip it somewhere you will find again, for example `Documents/VibePortfolio`.

*Mac only:* macOS blocks programs downloaded from the internet that are not from the App Store. Open
the **Terminal** app and run this once (change the path if you unzipped elsewhere):
```sh
xattr -dr com.apple.quarantine ~/Documents/VibePortfolio
```

**3. Get two free keys.**
- **Eulerpool** (financial data): sign up at eulerpool.com and copy your API key.
- **Tavily** (web search): sign up at tavily.com and copy your API key.

In the folder, make a copy of the file `.env.example` named `.env` (files starting with a dot are
hidden: on a Mac press Cmd+Shift+. in Finder to see them), open it in a text editor and paste
each key after its `=`. Never paste a key into a chat with the AI.

**4. Start the assistant in the folder.** In Terminal (Mac/Linux) or PowerShell (Windows):
```sh
cd ~/Documents/VibePortfolio
claude
```
Then type: **"Set me up."** The assistant will:
- ask which **language** you want (it talks to you in it);
- interview you with **the questionnaire**: your situation, goals, risk, how you want to invest.
  Your answers become your **profile** and your **strategy**: the limits every check uses (how
  much in funds, the largest single stock, the yield you aim for, and so on). Nothing is pre-filled:
  a limit you do not set is simply not checked;
- research your **country's** dividend taxes and tax-free account, and save them with sources;
- set up your **broker**: its fees and the costs of changing currency, from its published price
  list. (Importing your holdings from your broker's CSV statement is planned for a later version.)

**5. Run.** Type **"Do a portfolio run."** At each of the three moments the assistant tells you to
open the page `data/report/index.html` (double-click it), make your choices, press the download
button, and tell it where the file went.

Until your own country and broker are set up, the tool uses made-up examples called Exampleland and
Example Broker, and says so. Every tax and fee figure is then an example.

## Your data stays yours

Everything about you is in the `data` folder: your profile, strategy, runs and notes. It never
leaves your computer unless you send it somewhere. Keep a backup of that folder.

## For the curious

- `AGENTS.md`: the instructions every assistant reads (Claude Code through `CLAUDE.md`).
- `.agents/skills/`: the step-by-step skills; `.claude/agents/`: the analysts.
- Building from source, the code, and adding a country or broker: `CONTRIBUTING.md`.

## Claude Code's code mode: tested, not recommended today

Claude Code has an experimental "code mode" (`CLAUDE_CODE_EXPERIMENTAL_CODE_MODE=1`): the assistant
writes small scripts to read files instead of reading them one by one. We tested whether it makes a
run use fewer tokens (November 2026, Sonnet, a watchlist of 8 stocks, every task run twice
in each mode, a fresh session each time). Tokens are all tokens, cache reads included; each row
adds both runs together:

| Task | Normal mode | Code mode | Change |
|---|---|---|---|
| Small test (a table + one analyst on 2 names, one run each) | 1.12M | 0.76M | −33% |
| B1: the data-gaps report | 1.20M | 1.19M | −1% |
| B2: one heavy analyst (scenarios) on 8 names | 6.70M | 21.03M | +214% |
| B3: three analysts in a row on 5 names | 15.11M | 11.55M | −24% |
| B4: the portfolio builder's check loop | 3.05M | 3.62M | +19% |
| **B1–B4 total** | **26.06M** | **37.38M** | **+43%** |

What we found:
- **More tokens, not fewer.** Inside an analyst, each script is followed by an extra turn that
  waits for its result, and every turn re-reads the whole conversation. A long analyst (B2) made
  dozens of these turns.
- **Same verdicts, one weaker report.** The analysts' verdicts agreed as often across modes as
  between two runs of the same mode. But code mode's data-gaps report stated two things the files
  do not say.
- **No browser.** Code mode cannot drive Chrome, which a broker or research connector that reads
  your logged-in browser needs.

So leave code mode off. It may be worth a new test when Claude Code changes it. Run each task at
least twice per mode: the same task in the same mode varied by up to 3x in tokens between runs.

## Licence

Apache License 2.0 (see `LICENSE` and `NOTICE`).
