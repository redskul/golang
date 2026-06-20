# Long-Term Stock Screener

A research-and-screening assistant for long-term (1-3 year) holding decisions. It does **not**
predict short-term price moves -- no free (or paid) system reliably does that. What it does:

1. Loads your stock universe from `config/universe.yaml` (currently your real INDmoney
   holdings -- individual equities only, ETFs/gold/silver/liquid funds excluded).
2. Pulls fundamentals + price via `yfinance` (free, no API key) and recent headlines.
3. Scores each stock 0-100 on valuation, quality, growth, and financial health (percentile-ranked
   within your current universe -- see `scoring.py` for caveats on a small universe).
4. Maps the score to a tier (Strong Buy / Buy / Hold / Trim / Sell) and a target portfolio weight,
   then computes an actual buy/sell amount and share count from your current position size
   (`sizing.py`).
5. For the highest-conviction buys and worst-scoring holdings, calls the Claude API to write a
   bull case / bear case / key risks / thesis summary grounded in the fundamentals and news
   (`thesis.py`).
6. Renders everything into `output/latest_report.md`.

## Setup

```bash
cd stock_screener
pip install -r requirements.txt
export ANTHROPIC_API_KEY=sk-ant-...        # required for thesis synthesis
export FRED_API_KEY=...                     # optional, free key, enables macro snapshot
```

## Running

```bash
python3 run_screen.py                  # full run with theses
python3 run_screen.py --no-thesis      # quantitative scores only, no Claude calls
python3 run_screen.py --ticker AAPL    # screen a single name
```

## Keeping the universe in sync

`config/universe.yaml` was generated from a live snapshot of your INDmoney holdings. It will
drift as you buy/sell. To refresh it, ask Claude Code (which has IndMoney MCP access in this
session) to re-pull `networth_holdings` for `IND_STOCK` and `US_STOCK` and regenerate the file --
there's no standalone credential for the IndMoney API outside a Claude Code session, so this step
is manual/assisted rather than scripted.

Entries marked `verified: false` are best-effort ticker guesses (mostly recent IPOs/demergers) --
confirm the symbol on NSE/yfinance before trusting their fundamentals.

## Getting the report into your inbox

There's no Gmail send credential wired into this script (only Claude Code's session-scoped Gmail
MCP tool can create drafts, and it can only create drafts, not send). After running the script,
the easiest path is: open a Claude Code session and ask it to turn `output/latest_report.md` into
a Gmail draft. If you want fully unattended emailing, you'd need to set up your own Gmail API
OAuth credentials (or an SMTP app password) and add a small `notifier.py` that reads the rendered
report and sends it -- intentionally left out here since it requires secrets this environment
doesn't have.

## Note on this sandbox

This was built and smoke-tested inside a Claude Code remote execution environment whose network
egress policy doesn't allowlist `finance.yahoo.com`, so `yfinance` calls return nothing here and
every stock shows "insufficient data" -- that's the network policy, not a bug (the error handling
correctly degrades instead of crashing). Run it from your own machine, or an environment with
unrestricted/allowlisted network egress, to get real fundamentals.

## Honest limitations

- Percentile-rank scoring against a ~50-stock universe is noisy, especially across sectors
  (e.g. a bank's "good" ROE looks nothing like an industrial's). Treat scores as a ranking aid.
- `yfinance` data quality varies for small/mid-cap Indian names and recent IPOs; sanity-check
  before acting.
- ROIC is approximated (EBIT * (1 - tax rate) / invested capital) since yfinance doesn't expose
  it directly.
- This produces a thesis, not a prediction. The buy/sell decision and amount you actually execute
  is yours.
