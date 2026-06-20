"""Loads the stock universe and scoring/sizing settings from config/."""
from __future__ import annotations

import dataclasses
from pathlib import Path
from typing import Optional

import yaml

CONFIG_DIR = Path(__file__).parent / "config"


@dataclasses.dataclass
class Stock:
    name: str
    ticker: Optional[str]
    market: str  # "IN" or "US"
    market_cap_tier: str
    market_value: float
    verified: bool = True
    unlisted: bool = False

    @property
    def is_screenable(self) -> bool:
        return bool(self.ticker) and not self.unlisted


def load_universe(path: Path = CONFIG_DIR / "universe.yaml") -> list[Stock]:
    raw = yaml.safe_load(path.read_text())
    return [
        Stock(
            name=item["name"],
            ticker=item.get("ticker"),
            market=item["market"],
            market_cap_tier=item.get("market_cap_tier", "Unknown"),
            market_value=float(item["market_value"]),
            verified=item.get("verified", True),
            unlisted=item.get("unlisted", False),
        )
        for item in raw["stocks"]
    ]


def load_settings(path: Path = CONFIG_DIR / "settings.yaml") -> dict:
    return yaml.safe_load(path.read_text())


def total_universe_value(stocks: list[Stock]) -> float:
    return sum(s.market_value for s in stocks if not s.unlisted)
