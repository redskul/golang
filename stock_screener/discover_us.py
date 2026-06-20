#!/usr/bin/env python3
"""Screens US stocks you DON'T currently own and surfaces only Buy-rated new ideas.

Unlike run_screen.py (which reviews your existing holdings), this scans the candidate
list in config/candidates_us.yaml, drops anything already in config/universe.yaml
(your real holdings), scores the rest, and sizes a starter position as a % of your
current equity book for every name that clears the Buy bar.

Usage:
    python3 discover_us.py                  # full run, writes output/us_buy_ideas.md
    python3 discover_us.py --no-thesis       # quantitative scores only, no Claude calls
    python3 discover_us.py --top 5           # limit how many buy ideas to report
"""
from __future__ import annotations

import argparse
import dataclasses
import datetime as dt
import logging
from pathlib import Path

from fundamentals import fetch_fundamentals, fetch_recent_news
from macro import fetch_macro_snapshot
from scoring import score_universe
from sizing import compute_sizing
from thesis import write_thesis
from universe import CONFIG_DIR, load_settings, load_universe, total_universe_value

logging.basicConfig(level=logging.INFO, format="%(levelname)s %(name)s: %(message)s")
logger = logging.getLogger(__name__)

OUTPUT_PATH = Path(__file__).parent / "output" / "us_buy_ideas.md"


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--no-thesis", action="store_true", help="Skip Claude thesis synthesis")
    parser.add_argument("--top", type=int, default=10, help="Max number of buy ideas to report")
    args = parser.parse_args()

    settings = load_settings()
    holdings = load_universe()
    held_tickers = {s.ticker for s in holdings if s.ticker}

    candidates = load_universe(CONFIG_DIR / "candidates_us.yaml")
    candidates = [c for c in candidates if c.is_screenable and c.ticker not in held_tickers]
    logger.info("Screening %d US candidates not currently held...", len(candidates))

    # Size new positions as a % of your existing equity book, so a "Buy" amount is
    # comparable to the rest of your portfolio rather than some abstract %.
    portfolio_value = total_universe_value(holdings)

    fundamentals_by_ticker = {c.ticker: fetch_fundamentals(c.ticker) for c in candidates}
    scored_by_ticker = score_universe(fundamentals_by_ticker, settings)

    rows = []
    for stock in candidates:
        fund = fundamentals_by_ticker[stock.ticker]
        scored = scored_by_ticker[stock.ticker]
        sizing = compute_sizing(stock, scored, fund, portfolio_value, settings)
        if sizing.action == "Buy" and sizing.trade_value > 0:
            rows.append([stock, fund, scored, sizing, None])

    rows.sort(key=lambda r: -(r[2].composite_score or 0))
    rows = rows[: args.top]

    macro = []
    if not args.no_thesis and rows:
        macro = fetch_macro_snapshot(settings["macro"]["series"])
        for row in rows:
            stock, fund, scored, sizing, _ = row
            logger.info("Writing thesis for %s...", stock.ticker)
            news = fetch_recent_news(stock.ticker)
            row[4] = write_thesis(stock, fund, scored, sizing, news, macro, settings)

    OUTPUT_PATH.parent.mkdir(exist_ok=True)
    report_md = render_buy_ideas(rows, macro, len(candidates))
    OUTPUT_PATH.write_text(report_md)
    logger.info("Report written to %s", OUTPUT_PATH)
    print(report_md)


def render_buy_ideas(rows, macro, candidates_screened: int) -> str:
    today = dt.date.today().isoformat()
    lines = [f"# New US Stock Ideas (not currently held) -- {today}", ""]

    if macro:
        lines.append("## Macro Snapshot")
        for m in macro:
            lines.append(f"- **{m['label']}**: {m['value']} (as of {m['date']})")
        lines.append("")

    lines.append(f"Screened {candidates_screened} candidates from config/candidates_us.yaml. ")
    lines.append(f"{len(rows)} cleared the Buy bar.")
    lines.append("")
    lines.append(
        "*Reminder: this organizes evidence into a thesis, it doesn't predict price moves. "
        "The buy decision and amount you actually execute is yours.*"
    )
    lines.append("")

    if not rows:
        lines.append("No candidates scored high enough to recommend a new position right now.")
        return "\n".join(lines)

    for stock, fund, scored, sizing, thesis in rows:
        score_txt = f"{scored.composite_score:.0f}/100" if scored.composite_score else "n/a"
        shares_txt = f" (~{sizing.trade_shares:,.2f} shares)" if sizing.trade_shares else ""
        lines.append(f"## {stock.name} ({stock.ticker})")
        lines.append(f"Score: **{score_txt}** ({scored.tier_label})")
        lines.append(f"**Buy ${sizing.trade_value:,.0f}**{shares_txt} -- target {sizing.target_weight_pct:.1f}% of your equity book")
        lines.append(f"_{sizing.rationale}_")
        if thesis and not thesis.error:
            lines.append("")
            lines.append(f"**Thesis:** {thesis.thesis_summary}")
            lines.append(f"- **Bull case:** {thesis.bull_case}")
            lines.append(f"- **Bear case:** {thesis.bear_case}")
            if thesis.key_risks:
                lines.append(f"- **Key risks:** {'; '.join(thesis.key_risks)}")
        elif thesis and thesis.error:
            lines.append(f"_(thesis synthesis unavailable: {thesis.error})_")
        lines.append("")

    return "\n".join(lines)


if __name__ == "__main__":
    main()
