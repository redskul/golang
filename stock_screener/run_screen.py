#!/usr/bin/env python3
"""Runs the full screen: load universe -> fetch fundamentals/news -> score -> size -> synthesize
theses for the top/bottom names -> render a report.

Usage:
    python3 run_screen.py                  # full run, writes output/latest_report.md
    python3 run_screen.py --no-thesis       # skip the Claude synthesis step (faster, free)
    python3 run_screen.py --ticker HDFCBANK.NS   # screen a single ticker already in universe.yaml
"""
from __future__ import annotations

import argparse
import logging
from pathlib import Path

from fundamentals import fetch_fundamentals, fetch_recent_news
from macro import fetch_macro_snapshot
from report import render_report
from scoring import score_universe
from sizing import compute_sizing
from thesis import write_thesis
from universe import load_settings, load_universe, total_universe_value

logging.basicConfig(level=logging.INFO, format="%(levelname)s %(name)s: %(message)s")
logger = logging.getLogger(__name__)

OUTPUT_PATH = Path(__file__).parent / "output" / "latest_report.md"


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--no-thesis", action="store_true", help="Skip Claude thesis synthesis")
    parser.add_argument("--ticker", help="Only screen this single ticker")
    args = parser.parse_args()

    settings = load_settings()
    stocks = [s for s in load_universe() if s.is_screenable]
    if args.ticker:
        stocks = [s for s in stocks if s.ticker == args.ticker]
        if not stocks:
            raise SystemExit(f"No screenable entry with ticker {args.ticker} in universe.yaml")

    universe_total_value = total_universe_value(load_universe())

    logger.info("Fetching fundamentals for %d tickers...", len(stocks))
    fundamentals_by_ticker = {s.ticker: fetch_fundamentals(s.ticker) for s in stocks}

    scored_by_ticker = score_universe(fundamentals_by_ticker, settings)

    rows = []
    for stock in stocks:
        fund = fundamentals_by_ticker[stock.ticker]
        scored = scored_by_ticker[stock.ticker]
        sizing = compute_sizing(stock, scored, fund, universe_total_value, settings)
        rows.append([stock, fund, scored, sizing, None])

    # Rank for which names get a full LLM thesis (top buys + worst sells get priority).
    if not args.no_thesis:
        cfg = settings["synthesis"]
        ranked = sorted(
            [r for r in rows if not r[2].insufficient_data],
            key=lambda r: r[2].composite_score,
        )
        bottom = ranked[: cfg["full_thesis_count_bottom"]]
        top = ranked[-cfg["full_thesis_count_top"] :]
        thesis_targets = {id(r) for r in (bottom + top)}

        macro = fetch_macro_snapshot(settings["macro"]["series"])
        for row in rows:
            if id(row) in thesis_targets:
                stock, fund, scored, sizing, _ = row
                logger.info("Writing thesis for %s...", stock.ticker)
                news = fetch_recent_news(stock.ticker)
                row[4] = write_thesis(stock, fund, scored, sizing, news, macro, settings)
    else:
        macro = []

    OUTPUT_PATH.parent.mkdir(exist_ok=True)
    report_md = render_report([tuple(r) for r in rows], macro, currency="INR")
    OUTPUT_PATH.write_text(report_md)
    logger.info("Report written to %s", OUTPUT_PATH)
    print(report_md)


if __name__ == "__main__":
    main()
