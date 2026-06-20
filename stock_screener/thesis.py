"""Synthesizes a plain-English investment thesis per stock using the Claude API."""
from __future__ import annotations

import dataclasses
import json
import logging
import os

from anthropic import Anthropic

from fundamentals import Fundamentals
from scoring import ScoredStock
from sizing import SizingDecision
from universe import Stock

logger = logging.getLogger(__name__)

_SYSTEM_PROMPT = """You are a long-term equity research assistant. Given quantitative \
fundamentals, a computed score, recent headlines, and macro context for one stock, write a \
concise investment thesis for a long-term (1-3 year) holding decision. You are not predicting \
short-term price moves. Be specific about what would have to be true for the bull and bear cases, \
and name the 2-3 factors most likely to move the stock over the horizon. Output strict JSON with \
keys: bull_case, bear_case, key_risks (list of strings), thesis_summary (2-3 sentences)."""


@dataclasses.dataclass
class Thesis:
    bull_case: str
    bear_case: str
    key_risks: list[str]
    thesis_summary: str
    error: str | None = None


def _client() -> Anthropic:
    api_key = os.environ.get("ANTHROPIC_API_KEY")
    if not api_key:
        raise RuntimeError("ANTHROPIC_API_KEY is not set; cannot synthesize theses.")
    return Anthropic(api_key=api_key)


def write_thesis(
    stock: Stock,
    fundamentals: Fundamentals,
    scored: ScoredStock,
    sizing: SizingDecision,
    news: list[dict],
    macro: list[dict],
    settings: dict,
) -> Thesis:
    payload = {
        "name": stock.name,
        "ticker": stock.ticker,
        "market": stock.market,
        "fundamentals": dataclasses.asdict(fundamentals),
        "composite_score": scored.composite_score,
        "pillar_scores": scored.pillar_scores,
        "recommendation_tier": scored.tier_label,
        "sizing": dataclasses.asdict(sizing),
        "recent_news": news,
        "macro_context": macro,
    }

    cfg = settings["synthesis"]
    try:
        client = _client()
        response = client.messages.create(
            model=cfg["model"],
            max_tokens=cfg["max_tokens"],
            system=_SYSTEM_PROMPT,
            messages=[{"role": "user", "content": json.dumps(payload, default=str)}],
        )
        text = response.content[0].text
        # Models sometimes wrap JSON in a code fence; strip it defensively.
        text = text.strip().removeprefix("```json").removeprefix("```").removesuffix("```").strip()
        data = json.loads(text)
        return Thesis(
            bull_case=data["bull_case"],
            bear_case=data["bear_case"],
            key_risks=data["key_risks"],
            thesis_summary=data["thesis_summary"],
        )
    except Exception as exc:
        logger.warning("Thesis synthesis failed for %s: %s", stock.ticker, exc)
        return Thesis(bull_case="", bear_case="", key_risks=[], thesis_summary="", error=str(exc))
