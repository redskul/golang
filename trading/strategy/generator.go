package strategy

import (
	"fmt"
	"strings"
	"time"
)

// GenerateStrategies returns 3 strategies: Indian stocks, Indian MFs, US stocks.
func GenerateStrategies(capital float64, riskPct float64, tf Timeframe) []Strategy {
	return []Strategy{
		indianMomentumStrategy(capital, riskPct, tf),
		indianMFSIPStrategy(capital, riskPct),
		usMeanReversionStrategy(capital, riskPct, tf),
	}
}

func indianMomentumStrategy(capital, risk float64, tf Timeframe) Strategy {
	return Strategy{
		Name:         "India Breakout Momentum (EMA + RSI + Volume)",
		Market:       MarketIndianStocks,
		Timeframe:    tf,
		Capital:      capital,
		RiskPerTrade: risk,
		Direction:    Long,
		Indicators: []Indicator{
			{Name: "EMA", Settings: map[string]float64{"fast": 20, "slow": 50}, Purpose: "Trend direction filter"},
			{Name: "RSI", Settings: map[string]float64{"period": 14, "oversold": 40, "overbought": 70}, Purpose: "Momentum confirmation"},
			{Name: "Volume", Settings: map[string]float64{"ma_period": 20, "surge_multiplier": 1.5}, Purpose: "Breakout strength"},
			{Name: "ATR", Settings: map[string]float64{"period": 14}, Purpose: "Stop-loss sizing"},
		},
		EntryRules: []EntryRule{
			{Step: 1, Condition: "Trend Filter", Logic: "EMA20 > EMA50 (price is in uptrend)"},
			{Step: 2, Condition: "Momentum", Logic: "RSI(14) between 50-70 (strong but not overbought)"},
			{Step: 3, Condition: "Breakout", Logic: "Price closes above prior 20-bar high on candle body"},
			{Step: 4, Condition: "Volume Surge", Logic: "Volume >= 1.5x the 20-period average volume"},
			{Step: 5, Condition: "Entry", Logic: "Buy on next bar open after all 4 conditions are met"},
		},
		ExitRules: []ExitRule{
			{Step: 1, Condition: "Trend Break", Logic: "Close below EMA20 — exit immediately"},
			{Step: 2, Condition: "RSI Overbought", Logic: "RSI > 75 for two consecutive bars — scale out 50%"},
			{Step: 3, Condition: "Take Profit Hit", Logic: "Price reaches 3× ATR above entry — exit remaining position"},
		},
		StopLoss: StopLossLogic{
			Type:        "ATR",
			Multiplier:  1.5,
			Description: "Place stop at Entry − 1.5×ATR(14). Adjust position size so (1.5×ATR / Entry Price) × Shares = Capital × RiskPct",
		},
		TakeProfit: TakeProfitLogic{
			Type:        "RR",
			Ratio:       3.0,
			Description: "Target 3R (3× the initial risk distance). Partial exit at 2R, trail stop at breakeven after 2R hit.",
		},
		BestConditions:  "Strong bull market (Nifty > 200DMA), high FII inflows, sector rotation into momentum names. Works best in mid/small cap space during earnings season.",
		WorstConditions: "Sideways chop, broad market corrections >5%, low-volume holiday periods, pre-budget uncertainty.",
		Edge:            "Volume-confirmed breakouts above 20-bar highs in trending markets have high follow-through probability. The RSI 50-70 band filters out false breakouts at overbought extremes. India's retail participation drives momentum persistence.",
		GeneratedAt:     time.Now(),
	}
}

func indianMFSIPStrategy(capital, risk float64) Strategy {
	return Strategy{
		Name:         "India Mutual Fund Momentum-Value SIP Rotation",
		Market:       MarketMutualFunds,
		Timeframe:    "1M", // Monthly rebalance
		Capital:      capital,
		RiskPerTrade: risk,
		Direction:    Long,
		Indicators: []Indicator{
			{Name: "12-1 Momentum", Settings: map[string]float64{"lookback_months": 12, "skip_months": 1}, Purpose: "Select top-performing fund categories"},
			{Name: "Relative Strength vs Index", Settings: map[string]float64{"period_days": 63}, Purpose: "Fund vs Nifty 50 outperformance"},
			{Name: "Rolling Sharpe", Settings: map[string]float64{"window_months": 6}, Purpose: "Risk-adjusted ranking"},
			{Name: "Max Drawdown Filter", Settings: map[string]float64{"max_dd_pct": 20}, Purpose: "Eliminate high-volatility funds"},
		},
		EntryRules: []EntryRule{
			{Step: 1, Condition: "Category Screening", Logic: "Rank all fund categories by 12-month return (skip last month to avoid reversal)"},
			{Step: 2, Condition: "Top Quartile", Logic: "Select top 25% categories: typically Midcap, Smallcap, Flexi-cap in bull; Debt/Gilt in bear"},
			{Step: 3, Condition: "Fund Selection", Logic: "Within category, pick funds ranked top 2 by 6-month rolling Sharpe AND max drawdown < 20%"},
			{Step: 4, Condition: "SIP Entry", Logic: "Deploy 1/3 of allocation on 1st, 8th, 15th of month to average entry price"},
		},
		ExitRules: []ExitRule{
			{Step: 1, Condition: "Monthly Rebalance", Logic: "On the 1st of each month, re-rank. Exit fund if it drops out of top 40% of category"},
			{Step: 2, Condition: "Regime Switch", Logic: "If Nifty drops below 10-month SMA, rotate 70% of equity allocation to liquid/overnight funds"},
			{Step: 3, Condition: "Category Rotation", Logic: "Shift to large-cap/index funds when midcap/smallcap P/E > 1.5× 5-year average"},
		},
		StopLoss: StopLossLogic{
			Type:        "Regime",
			Multiplier:  0.0,
			Description: "No hard stop on individual funds. Portfolio-level stop: if portfolio NAV falls 15% from peak, halt new SIPs and hold until recovery above 10-month SMA.",
		},
		TakeProfit: TakeProfitLogic{
			Type:        "Rebalance",
			Ratio:       0.0,
			Description: "Annual rebalancing: if any fund exceeds 30% of portfolio, trim back to 20% and redeploy into laggard category.",
		},
		BestConditions:  "Bull markets with clear sectoral rotation (IT → Infra → FMCG cycle), falling interest rate environment, post-correction recovery phases.",
		WorstConditions: "Sudden liquidity crises (IL&FS, NBFC crisis), regulatory changes to mutual fund taxation, prolonged flat markets where all categories underperform.",
		Edge:            "12-1 momentum in mutual funds is academically documented to persist 3-12 months. SIP averaging removes timing risk. Regime filter (10-month SMA) historically reduces drawdown by 40% by avoiding the worst bear legs.",
		GeneratedAt:     time.Now(),
	}
}

func usMeanReversionStrategy(capital, risk float64, tf Timeframe) Strategy {
	return Strategy{
		Name:         "US Stocks Overnight Mean Reversion (RSI2 + Bollinger Bands)",
		Market:       MarketUS,
		Timeframe:    tf,
		Capital:      capital,
		RiskPerTrade: risk,
		Direction:    Long,
		Indicators: []Indicator{
			{Name: "RSI", Settings: map[string]float64{"period": 2, "oversold": 10, "overbought": 90}, Purpose: "Extreme short-term oversold detection"},
			{Name: "Bollinger Bands", Settings: map[string]float64{"period": 20, "std_dev": 2.0}, Purpose: "Price deviation from mean"},
			{Name: "SMA", Settings: map[string]float64{"period": 200}, Purpose: "Long-term trend filter — only trade LONG above 200SMA"},
			{Name: "ATR", Settings: map[string]float64{"period": 10}, Purpose: "Volatility-adjusted stop sizing"},
			{Name: "S&P500 Filter", Settings: map[string]float64{"sma_period": 5}, Purpose: "Market regime — trade only when SPY > 5DMA"},
		},
		EntryRules: []EntryRule{
			{Step: 1, Condition: "Trend Filter", Logic: "Stock price > 200-day SMA AND SPY (S&P 500 ETF) > 5-day SMA"},
			{Step: 2, Condition: "Oversold Signal", Logic: "RSI(2) closes below 10 (extreme short-term oversold)"},
			{Step: 3, Condition: "Bollinger Confirmation", Logic: "Price closes below or at lower Bollinger Band (20,2)"},
			{Step: 4, Condition: "Entry", Logic: "Buy at market open next day (overnight hold strategy)"},
		},
		ExitRules: []ExitRule{
			{Step: 1, Condition: "RSI Recovery", Logic: "Exit when RSI(2) rises above 65 — mean reversion complete"},
			{Step: 2, Condition: "Time Stop", Logic: "If not profitable after 3 trading days, exit at close on day 3"},
			{Step: 3, Condition: "Hard Stop", Logic: "Exit if price drops 2× ATR(10) below entry"},
		},
		StopLoss: StopLossLogic{
			Type:        "ATR",
			Multiplier:  2.0,
			Description: "Stop at Entry − 2×ATR(10). Position size = (Capital × RiskPct) / (2×ATR)",
		},
		TakeProfit: TakeProfitLogic{
			Type:        "RSI",
			Ratio:       2.5,
			Description: "Exit when RSI(2) > 65. Average winner holds 1-2 days. Average R:R ~2.5:1 due to high win rate (65-70%).",
		},
		BestConditions:  "Low-volatility bull market with occasional 2-5% pullbacks in strong stocks. Works excellently in S&P 500 and Nasdaq 100 component stocks with high liquidity.",
		WorstConditions: "Trending bear markets, high VIX environments (>30), earnings week (skip stocks reporting), macro shocks (Fed announcements, geopolitical events).",
		Edge:            "RSI(2) mean reversion was documented by Connors & Alvarez (2009) and remains effective. Short holding period limits exposure to overnight gap risk. The 200-SMA filter ensures trading with the primary trend, converting losers into smaller losses. Win rate typically 65-72% in historical backtests.",
		GeneratedAt:     time.Now(),
	}
}

// PrintStrategy renders a strategy to a readable string.
func PrintStrategy(s Strategy) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "═══════════════════════════════════════════════════════\n")
	fmt.Fprintf(&sb, "STRATEGY: %s\n", s.Name)
	fmt.Fprintf(&sb, "Market: %-15s  Timeframe: %s\n", s.Market, s.Timeframe)
	fmt.Fprintf(&sb, "Capital: $%.0f   Risk/Trade: %.0f%%\n", s.Capital, s.RiskPerTrade*100)
	fmt.Fprintf(&sb, "───────────────────────────────────────────────────────\n")

	fmt.Fprintf(&sb, "\n[1] INDICATORS\n")
	for _, ind := range s.Indicators {
		settings := ""
		for k, v := range ind.Settings {
			settings += fmt.Sprintf("%s=%.0f ", k, v)
		}
		fmt.Fprintf(&sb, "  %-20s  %-35s  %s\n", ind.Name, strings.TrimSpace(settings), ind.Purpose)
	}

	fmt.Fprintf(&sb, "\n[2] ENTRY RULES\n")
	for _, r := range s.EntryRules {
		fmt.Fprintf(&sb, "  Step %d [%s]: %s\n", r.Step, r.Condition, r.Logic)
	}

	fmt.Fprintf(&sb, "\n[3] EXIT RULES\n")
	for _, r := range s.ExitRules {
		fmt.Fprintf(&sb, "  Step %d [%s]: %s\n", r.Step, r.Condition, r.Logic)
	}

	fmt.Fprintf(&sb, "\n[4] STOP-LOSS\n  Type: %s | Multiplier: %.1f\n  %s\n",
		s.StopLoss.Type, s.StopLoss.Multiplier, s.StopLoss.Description)

	fmt.Fprintf(&sb, "\n[5] TAKE-PROFIT\n  Type: %s | R:R = %.1f:1\n  %s\n",
		s.TakeProfit.Type, s.TakeProfit.Ratio, s.TakeProfit.Description)

	fmt.Fprintf(&sb, "\n[6] MARKET CONDITIONS\n")
	fmt.Fprintf(&sb, "  Best:  %s\n", s.BestConditions)
	fmt.Fprintf(&sb, "  Worst: %s\n", s.WorstConditions)

	fmt.Fprintf(&sb, "\n[7] EDGE\n  %s\n", s.Edge)
	fmt.Fprintf(&sb, "═══════════════════════════════════════════════════════\n")
	return sb.String()
}
