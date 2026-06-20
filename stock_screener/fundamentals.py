"""Fetches fundamentals and price history per ticker via yfinance (free, no API key)."""
from __future__ import annotations

import dataclasses
import logging
from typing import Optional

import yfinance as yf

logger = logging.getLogger(__name__)


@dataclasses.dataclass
class Fundamentals:
    ticker: str
    price: Optional[float] = None
    pe_ratio: Optional[float] = None
    price_to_fcf: Optional[float] = None
    peg_ratio: Optional[float] = None
    ev_to_ebitda: Optional[float] = None
    roe: Optional[float] = None
    roic: Optional[float] = None
    operating_margin: Optional[float] = None
    revenue_cagr_5y: Optional[float] = None
    earnings_cagr_5y: Optional[float] = None
    debt_to_equity: Optional[float] = None
    fcf_trend: Optional[float] = None  # +1 rising, 0 flat, -1 falling
    fetch_error: Optional[str] = None


def _cagr(first: float, last: float, years: int) -> Optional[float]:
    if first is None or last is None or first <= 0 or years <= 0:
        return None
    return (last / first) ** (1 / years) - 1


def _fcf_trend(cashflow) -> Optional[float]:
    try:
        ocf = cashflow.loc["Total Cash From Operating Activities"]
        capex = cashflow.loc["Capital Expenditures"]
        fcf = (ocf + capex).dropna()
        if len(fcf) < 2:
            return None
        fcf = fcf.iloc[::-1]  # oldest first
        diffs = fcf.diff().dropna()
        return 1.0 if diffs.mean() > 0 else (-1.0 if diffs.mean() < 0 else 0.0)
    except Exception:
        return None


def fetch_fundamentals(ticker: str) -> Fundamentals:
    f = Fundamentals(ticker=ticker)
    try:
        t = yf.Ticker(ticker)
        info = t.info or {}

        f.price = info.get("currentPrice") or info.get("regularMarketPrice")
        f.pe_ratio = info.get("trailingPE")
        f.peg_ratio = info.get("pegRatio")
        f.ev_to_ebitda = info.get("enterpriseToEbitda")
        f.roe = info.get("returnOnEquity")
        f.operating_margin = info.get("operatingMargins")
        f.debt_to_equity = info.get("debtToEquity")

        market_cap = info.get("marketCap")
        fcf = info.get("freeCashflow")
        if market_cap and fcf and fcf != 0:
            f.price_to_fcf = market_cap / fcf

        # ROIC isn't directly exposed by yfinance; approximate with
        # EBIT * (1 - tax rate) / (debt + equity) when the pieces are available.
        try:
            financials = t.financials
            balance = t.balance_sheet
            ebit = financials.loc["Ebit"].iloc[0]
            tax_rate = info.get("effectiveTaxRate") or 0.25
            invested_capital = (
                balance.loc["Total Debt"].iloc[0] if "Total Debt" in balance.index else 0
            ) + (balance.loc["Total Stockholder Equity"].iloc[0])
            if invested_capital:
                f.roic = (ebit * (1 - tax_rate)) / invested_capital
        except Exception:
            pass

        try:
            financials = t.financials
            revenue = financials.loc["Total Revenue"].dropna()
            if len(revenue) >= 2:
                years = len(revenue) - 1
                f.revenue_cagr_5y = _cagr(revenue.iloc[-1], revenue.iloc[0], years)
            earnings = financials.loc["Net Income"].dropna()
            if len(earnings) >= 2:
                years = len(earnings) - 1
                f.earnings_cagr_5y = _cagr(earnings.iloc[-1], earnings.iloc[0], years)
        except Exception:
            pass

        try:
            f.fcf_trend = _fcf_trend(t.cashflow)
        except Exception:
            pass

    except Exception as exc:  # network errors, delisted tickers, bad symbols, etc.
        f.fetch_error = str(exc)
        logger.warning("Failed to fetch fundamentals for %s: %s", ticker, exc)

    return f


def fetch_recent_news(ticker: str, limit: int = 5) -> list[dict]:
    try:
        t = yf.Ticker(ticker)
        items = t.news or []
        out = []
        for item in items[:limit]:
            content = item.get("content", item)
            out.append(
                {
                    "title": content.get("title", item.get("title", "")),
                    "publisher": (content.get("provider") or {}).get("displayName", ""),
                    "link": (content.get("canonicalUrl") or {}).get("url", item.get("link", "")),
                }
            )
        return out
    except Exception as exc:
        logger.warning("Failed to fetch news for %s: %s", ticker, exc)
        return []
