package multifactor

import (
	"fmt"
	"strings"
)

type Factor struct {
	Name        string
	Weight      float64 // 0-1
	Formula     string
	Rationale   string
	Rebalance   string
}

type PortfolioAlloc struct {
	Asset   string
	Score   float64
	Weight  float64
	Sector  string
	Reason  string
}

type MultiFactorStrategy struct {
	Name              string
	Factors           []Factor
	RebalanceFrequency string
	ExamplePortfolio  []PortfolioAlloc
	ConstructionLogic string
}

func BuildStrategy() MultiFactorStrategy {
	factors := []Factor{
		{
			Name:      "Momentum",
			Weight:    0.30,
			Formula:   "12-month total return minus 1-month return (to skip reversal). Rank percentile within universe. Score = 0 to 100.",
			Rationale: "Academically documented (Jegadeesh & Titman 1993). Stocks that have outperformed continue to do so over 3-12 months. Works in both India and US markets.",
			Rebalance: "Monthly — momentum is a short-lived signal",
		},
		{
			Name:      "Value",
			Weight:    0.25,
			Formula:   "Composite: equal weight of (1/P/E rank) + (P/B rank) + (EV/EBITDA rank). Lower multiple = higher score. Normalize to 0-100.",
			Rationale: "Value premium documented by Fama & French (1992). Cheap stocks outperform over 3-5 year horizons, especially in Indian markets where value dispersion is high.",
			Rebalance: "Quarterly — value signals are slower-moving",
		},
		{
			Name:      "Volatility (Low-Vol)",
			Weight:    0.20,
			Formula:   "Inverse of 12-month realized volatility. Score = 100 − (percentile rank of annualized vol). Low-vol stocks score high.",
			Rationale: "Low-volatility anomaly (Baker et al. 2011): low-vol stocks outperform on a risk-adjusted basis globally. Reduces portfolio drawdown significantly.",
			Rebalance: "Monthly",
		},
		{
			Name:      "Trend",
			Weight:    0.25,
			Formula:   "Price vs 200-day SMA: score 100 if price > 200SMA by >10%, scale linearly. Negative score if price below 200SMA.",
			Rationale: "Trend-following cuts exposure to deteriorating stocks. Acts as a regime filter — removes value traps and falling knives from the long book.",
			Rebalance: "Weekly — trend signals can shift quickly",
		},
	}

	example := []PortfolioAlloc{
		{Asset: "Reliance Industries (NSE)", Score: 82, Weight: 0.12, Sector: "Energy/Conglomerate", Reason: "High momentum rank, above 200SMA, moderate valuation"},
		{Asset: "HDFC Bank (NSE)", Score: 78, Weight: 0.10, Sector: "Banking", Reason: "Strong trend, low volatility, reasonable P/B vs peers"},
		{Asset: "Infosys (NSE)", Score: 75, Weight: 0.09, Sector: "IT", Reason: "Positive momentum, low vol, improving EV/EBITDA"},
		{Asset: "Tata Motors (NSE)", Score: 71, Weight: 0.08, Sector: "Auto", Reason: "Value factor leader (low P/E), recovering trend"},
		{Asset: "Bajaj Finance (NSE)", Score: 70, Weight: 0.08, Sector: "NBFC", Reason: "Momentum leader in BFSI, above 200SMA"},
		{Asset: "Apple Inc (NASDAQ)", Score: 85, Weight: 0.12, Sector: "Tech", Reason: "High momentum, low realized vol, strong trend"},
		{Asset: "Microsoft (NASDAQ)", Score: 83, Weight: 0.11, Sector: "Tech", Reason: "Top momentum and trend score, moderate valuation for growth"},
		{Asset: "Mirae Asset Large Cap MF", Score: 74, Weight: 0.10, Sector: "MF", Reason: "Consistent outperformance, lower volatility than pure small-cap"},
		{Asset: "PPFAS Flexi Cap MF", Score: 72, Weight: 0.10, Sector: "MF", Reason: "Value + global diversification, steady NAV trend"},
		{Asset: "Cash / Liquid Fund", Score: 0, Weight: 0.10, Sector: "Cash", Reason: "Regime buffer — reduces exposure when trend score is mixed"},
	}

	return MultiFactorStrategy{
		Name:    "Momentum-Value-VolTrend (MVVT) Multi-Factor",
		Factors: factors,
		RebalanceFrequency: "Monthly for Momentum+LowVol; Quarterly for Value; Weekly check for Trend filter",
		ExamplePortfolio:   example,
		ConstructionLogic: `
CONSTRUCTION STEPS:
1. Define universe: Nifty 500 (India) + S&P 500 (US) + top 200 MFs
2. For each asset, compute all 4 factor scores (0-100)
3. Composite score = 0.30×Momentum + 0.25×Value + 0.20×LowVol + 0.25×Trend
4. Rank all assets by composite score
5. Long top 20% of ranked assets (top quintile)
6. Weight by composite score (score-proportional weighting), cap at 15% per asset
7. Apply sector diversification: max 25% in any single sector
8. Rebalance monthly; transaction cost filter — only replace if new asset score > old by 5 points`,
	}
}

func PrintStrategy(s MultiFactorStrategy) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "═══════════════════════════════════════════════════════\n")
	fmt.Fprintf(&sb, "MULTI-FACTOR STRATEGY: %s\n", s.Name)
	fmt.Fprintf(&sb, "Rebalance: %s\n", s.RebalanceFrequency)
	fmt.Fprintf(&sb, "───────────────────────────────────────────────────────\n")

	totalWeight := 0.0
	for _, f := range s.Factors {
		totalWeight += f.Weight
	}
	fmt.Fprintf(&sb, "\nFACTORS  (Total Weight: %.0f%%)\n", totalWeight*100)
	for _, f := range s.Factors {
		fmt.Fprintf(&sb, "\n  [%.0f%%] %s\n", f.Weight*100, f.Name)
		fmt.Fprintf(&sb, "    Formula:   %s\n", f.Formula)
		fmt.Fprintf(&sb, "    Rationale: %s\n", f.Rationale)
		fmt.Fprintf(&sb, "    Rebalance: %s\n", f.Rebalance)
	}

	fmt.Fprintf(&sb, "\nEXAMPLE PORTFOLIO (Top 10 Holdings)\n")
	fmt.Fprintf(&sb, "  %-35s %6s %8s  %s\n", "Asset", "Score", "Weight", "Reason")
	fmt.Fprintf(&sb, "  %s\n", strings.Repeat("-", 85))
	for _, a := range s.ExamplePortfolio {
		fmt.Fprintf(&sb, "  %-35s %6.0f %7.0f%%  %s\n", a.Asset, a.Score, a.Weight*100, a.Reason)
	}

	fmt.Fprintf(&sb, "\nCONSTRUCTION LOGIC%s\n", s.ConstructionLogic)
	fmt.Fprintf(&sb, "═══════════════════════════════════════════════════════\n")
	return sb.String()
}
