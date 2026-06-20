"""Translates a tier + target weight into an actual buy/sell amount and share count."""
from __future__ import annotations

import dataclasses
from typing import Optional

from fundamentals import Fundamentals
from scoring import ScoredStock
from universe import Stock


@dataclasses.dataclass
class SizingDecision:
    action: str  # "Buy", "Sell", "Hold", "No Action"
    current_weight_pct: float
    target_weight_pct: Optional[float]
    trade_value: float  # positive = buy this much, negative = sell this much
    trade_shares: Optional[float]
    rationale: str


def compute_sizing(
    stock: Stock,
    scored: ScoredStock,
    fundamentals: Fundamentals,
    universe_total_value: float,
    settings: dict,
) -> SizingDecision:
    sizing_cfg = settings["sizing"]
    current_weight_pct = (
        100.0 * stock.market_value / universe_total_value if universe_total_value else 0.0
    )

    if scored.insufficient_data:
        return SizingDecision(
            action="No Action",
            current_weight_pct=current_weight_pct,
            target_weight_pct=None,
            trade_value=0.0,
            trade_shares=None,
            rationale="Not enough fundamental data to score; verify the ticker symbol.",
        )

    target_weight_pct = scored.target_weight_pct
    if target_weight_pct is None:  # Hold tier: keep current weight
        return SizingDecision(
            action="Hold",
            current_weight_pct=current_weight_pct,
            target_weight_pct=current_weight_pct,
            trade_value=0.0,
            trade_shares=None,
            rationale=f"Composite score {scored.composite_score:.0f} lands in the Hold band; "
            "maintain current position.",
        )

    target_value = (target_weight_pct / 100.0) * universe_total_value
    trade_value = target_value - stock.market_value

    min_rebalance_value = stock.market_value * (sizing_cfg["min_rebalance_pct_of_position"] / 100.0)
    max_trade_value = universe_total_value * (sizing_cfg["max_single_trade_pct_of_universe"] / 100.0)

    if abs(trade_value) < min_rebalance_value and stock.market_value > 0:
        return SizingDecision(
            action="Hold",
            current_weight_pct=current_weight_pct,
            target_weight_pct=target_weight_pct,
            trade_value=0.0,
            trade_shares=None,
            rationale=f"Already close to target weight ({current_weight_pct:.1f}% vs "
            f"{target_weight_pct:.1f}% target); rebalance too small to bother with.",
        )

    capped_trade_value = max(-max_trade_value, min(max_trade_value, trade_value))
    action = "Buy" if capped_trade_value > 0 else "Sell"
    shares = None
    if fundamentals.price:
        shares = abs(capped_trade_value) / fundamentals.price

    rationale = (
        f"Composite score {scored.composite_score:.0f} -> {scored.tier_label}. "
        f"Current weight {current_weight_pct:.1f}%, target {target_weight_pct:.1f}% of equity book."
    )

    return SizingDecision(
        action=action,
        current_weight_pct=current_weight_pct,
        target_weight_pct=target_weight_pct,
        trade_value=capped_trade_value,
        trade_shares=shares,
        rationale=rationale,
    )
