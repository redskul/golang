"""Renders the screen results into a Markdown digest ready to email or paste into a Gmail draft."""
from __future__ import annotations

import datetime as dt

from fundamentals import Fundamentals
from scoring import ScoredStock
from sizing import SizingDecision
from thesis import Thesis
from universe import Stock

ACTION_EMOJI = {"Buy": "\U0001F7E2", "Sell": "\U0001F534", "Hold": "\U0001F7E1", "No Action": "⚪"}


def _fmt_money(value: float, currency_note: str) -> str:
    sign = "+" if value > 0 else ""
    return f"{sign}{value:,.0f} {currency_note}"


def render_report(
    rows: list[tuple[Stock, Fundamentals, ScoredStock, SizingDecision, Thesis | None]],
    macro: list[dict],
    currency: str,
) -> str:
    today = dt.date.today().isoformat()
    lines = [f"# Long-Term Portfolio Screen -- {today}", ""]

    if macro:
        lines.append("## Macro Snapshot")
        for m in macro:
            lines.append(f"- **{m['label']}**: {m['value']} (as of {m['date']})")
        lines.append("")

    actionable = [r for r in rows if r[3].action in ("Buy", "Sell")]
    holds = [r for r in rows if r[3].action == "Hold"]
    no_action = [r for r in rows if r[3].action == "No Action"]

    lines.append("## Summary")
    lines.append(f"- {len(actionable)} actionable calls (Buy/Sell)")
    lines.append(f"- {len(holds)} Hold")
    lines.append(f"- {len(no_action)} skipped (insufficient data -- check ticker symbols)")
    lines.append("")
    lines.append(
        "*Reminder: this is a quantitative screen + LLM-written research summary, not a "
        "prediction. It organizes evidence; the buy/sell decision is yours.*"
    )
    lines.append("")

    def render_section(title: str, items):
        if not items:
            return
        lines.append(f"## {title}")
        for stock, fund, scored, sizing, thesis in sorted(
            items, key=lambda r: -(r[2].composite_score or 0)
        ):
            emoji = ACTION_EMOJI.get(sizing.action, "")
            score_txt = f"{scored.composite_score:.0f}/100" if scored.composite_score else "n/a"
            lines.append(f"### {emoji} {stock.name} ({stock.ticker}) -- {sizing.action}")
            lines.append(
                f"Score: **{score_txt}** ({scored.tier_label}) | "
                f"Current weight: {sizing.current_weight_pct:.1f}% | "
                f"Target: {sizing.target_weight_pct:.1f}%"
                if sizing.target_weight_pct is not None
                else f"Score: **{score_txt}** ({scored.tier_label})"
            )
            if sizing.action in ("Buy", "Sell"):
                shares_txt = f" (~{sizing.trade_shares:,.2f} shares)" if sizing.trade_shares else ""
                lines.append(f"**{sizing.action} {_fmt_money(abs(sizing.trade_value), currency)}**{shares_txt}")
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

    render_section("Actionable: Buy / Sell", actionable)
    render_section("Hold", holds)
    render_section("Skipped (insufficient data)", no_action)

    return "\n".join(lines)
