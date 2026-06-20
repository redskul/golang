"""Turns raw fundamentals into pillar scores and a composite 0-100 score.

Metrics are scored by percentile rank *within the current universe* rather
than against fixed thresholds, since "good" P/E or margins differ wildly
across sectors and between Indian small-caps and US mega-caps. With a small
universe this is noisy at the edges -- treat scores as a ranking aid, not a
precise number.
"""
from __future__ import annotations

import dataclasses
from typing import Optional

from fundamentals import Fundamentals

# Metrics where a *lower* raw value is better; percentile rank gets inverted.
LOWER_IS_BETTER = {"pe_ratio", "price_to_fcf", "peg_ratio", "ev_to_ebitda", "debt_to_equity"}


@dataclasses.dataclass
class ScoredStock:
    ticker: str
    composite_score: Optional[float]
    pillar_scores: dict[str, Optional[float]]
    tier_label: str
    target_weight_pct: Optional[float]
    insufficient_data: bool


def _percentile_rank(value: float, all_values: list[float]) -> float:
    if not all_values:
        return 50.0
    n = len(all_values)
    below_or_equal = sum(1 for v in all_values if v <= value)
    return 100.0 * below_or_equal / n


def _metric_score(metric: str, value: Optional[float], universe_values: list[float]) -> Optional[float]:
    if value is None:
        return None
    if metric == "fcf_trend":
        return {1.0: 100.0, 0.0: 50.0, -1.0: 0.0}.get(value, 50.0)
    rank = _percentile_rank(value, universe_values)
    return (100.0 - rank) if metric in LOWER_IS_BETTER else rank


def _pillar_score(
    metric_weights: dict[str, float],
    fundamentals: Fundamentals,
    universe_by_metric: dict[str, list[float]],
) -> Optional[float]:
    weighted_sum = 0.0
    weight_used = 0.0
    for metric, weight in metric_weights.items():
        value = getattr(fundamentals, metric, None)
        score = _metric_score(metric, value, universe_by_metric.get(metric, []))
        if score is None:
            continue
        weighted_sum += score * weight
        weight_used += weight
    if weight_used == 0:
        return None
    return weighted_sum / weight_used


def score_universe(
    fundamentals_by_ticker: dict[str, Fundamentals], settings: dict
) -> dict[str, ScoredStock]:
    scoring_cfg = settings["scoring"]
    pillar_weights = scoring_cfg["pillar_weights"]
    metric_groups = {
        "valuation": scoring_cfg["valuation_metrics"],
        "quality": scoring_cfg["quality_metrics"],
        "growth": scoring_cfg["growth_metrics"],
        "financial_health": scoring_cfg["financial_health_metrics"],
    }

    all_metrics = {m for group in metric_groups.values() for m in group}
    universe_by_metric: dict[str, list[float]] = {m: [] for m in all_metrics}
    for fund in fundamentals_by_ticker.values():
        for m in all_metrics:
            v = getattr(fund, m, None)
            if v is not None:
                universe_by_metric[m].append(v)

    tiers = sorted(settings["recommendation_tiers"], key=lambda t: -t["min_score"])

    results: dict[str, ScoredStock] = {}
    for ticker, fund in fundamentals_by_ticker.items():
        pillar_scores = {
            pillar: _pillar_score(metrics, fund, universe_by_metric)
            for pillar, metrics in metric_groups.items()
        }

        weighted_sum = 0.0
        weight_used = 0.0
        for pillar, score in pillar_scores.items():
            if score is None:
                continue
            w = pillar_weights[pillar]
            weighted_sum += score * w
            weight_used += w

        composite = weighted_sum / weight_used if weight_used > 0 else None
        insufficient = composite is None

        tier_label, target_weight = "Insufficient Data", None
        if composite is not None:
            for tier in tiers:
                if composite >= tier["min_score"]:
                    tier_label = tier["label"]
                    target_weight = tier["target_weight_pct"]
                    break

        results[ticker] = ScoredStock(
            ticker=ticker,
            composite_score=composite,
            pillar_scores=pillar_scores,
            tier_label=tier_label,
            target_weight_pct=target_weight,
            insufficient_data=insufficient,
        )

    return results
