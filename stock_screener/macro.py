"""Optional macro context via the free FRED API. Skipped silently if FRED_API_KEY is unset."""
from __future__ import annotations

import logging
import os

import requests

logger = logging.getLogger(__name__)

FRED_BASE = "https://api.stlouisfed.org/fred/series/observations"


def fetch_macro_snapshot(series: list[dict]) -> list[dict]:
    api_key = os.environ.get("FRED_API_KEY")
    if not api_key:
        logger.info("FRED_API_KEY not set; skipping macro snapshot.")
        return []

    results = []
    for s in series:
        try:
            resp = requests.get(
                FRED_BASE,
                params={
                    "series_id": s["id"],
                    "api_key": api_key,
                    "file_type": "json",
                    "sort_order": "desc",
                    "limit": 1,
                },
                timeout=10,
            )
            resp.raise_for_status()
            obs = resp.json()["observations"][0]
            results.append({"label": s["label"], "date": obs["date"], "value": obs["value"]})
        except Exception as exc:
            logger.warning("Failed to fetch FRED series %s: %s", s["id"], exc)
    return results
