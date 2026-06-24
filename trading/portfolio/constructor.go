package portfolio

import (
	"fmt"
	"math"
	"strings"
)

type RiskTolerance string
type Horizon string

const (
	RiskLow    RiskTolerance = "LOW"
	RiskMedium RiskTolerance = "MEDIUM"
	RiskHigh   RiskTolerance = "HIGH"

	Horizon1Y  Horizon = "1Y"
	Horizon3Y  Horizon = "3Y"
	Horizon5Y  Horizon = "5Y"
	Horizon10Y Horizon = "10Y"
)

type Asset struct {
	Name            string
	Class           string
	Geography       string
	Allocation      float64 // 0-1
	ExpectedReturn  float64 // annual, decimal
	ExpectedVol     float64 // annual, decimal
	Reason          string
}

type Portfolio struct {
	Name            string
	RiskTolerance   RiskTolerance
	Horizon         Horizon
	TotalCapital    float64
	Assets          []Asset
	ExpectedReturn  float64
	ExpectedVol     float64
	ExpectedDrawdown float64
	SharpeEstimate  float64
}

func Build(capital float64, risk RiskTolerance, horizon Horizon) Portfolio {
	assets := allocate(risk, horizon)
	er, ev := portfolioStats(assets)
	sharpe := (er - 0.07) / ev // 7% risk-free (India benchmark)
	dd := ev * 1.5             // rough max drawdown estimate

	return Portfolio{
		Name:             fmt.Sprintf("%s Risk | %s Horizon Portfolio", risk, horizon),
		RiskTolerance:    risk,
		Horizon:          horizon,
		TotalCapital:     capital,
		Assets:           assets,
		ExpectedReturn:   er,
		ExpectedVol:      ev,
		ExpectedDrawdown: dd,
		SharpeEstimate:   sharpe,
	}
}

func allocate(risk RiskTolerance, horizon Horizon) []Asset {
	switch risk {
	case RiskLow:
		return []Asset{
			{Name: "HDFC Short Term Debt MF", Class: "Debt MF", Geography: "India", Allocation: 0.30, ExpectedReturn: 0.07, ExpectedVol: 0.02, Reason: "Capital preservation; 7% stable return with minimal volatility"},
			{Name: "SBI Liquid Fund", Class: "Liquid MF", Geography: "India", Allocation: 0.20, ExpectedReturn: 0.065, ExpectedVol: 0.005, Reason: "Emergency buffer and parking; near-zero drawdown risk"},
			{Name: "NIFTY 50 Index Fund", Class: "Equity MF", Geography: "India", Allocation: 0.20, ExpectedReturn: 0.13, ExpectedVol: 0.18, Reason: "Equity growth exposure with diversification; lowest cost large-cap"},
			{Name: "Sovereign Gold Bond (SGB)", Class: "Gold", Geography: "India", Allocation: 0.15, ExpectedReturn: 0.09, ExpectedVol: 0.12, Reason: "Inflation hedge; negative correlation to equity in crises"},
			{Name: "US S&P 500 Index (via MF)", Class: "Global Equity", Geography: "US", Allocation: 0.10, ExpectedReturn: 0.12, ExpectedVol: 0.16, Reason: "USD diversification + US tech exposure; uncorrelated to INR risks"},
			{Name: "RBI Floating Rate Bonds", Class: "Bonds", Geography: "India", Allocation: 0.05, ExpectedReturn: 0.075, ExpectedVol: 0.01, Reason: "Inflation-linked returns; government safety"},
		}
	case RiskMedium:
		return []Asset{
			{Name: "Mirae Asset Large Cap MF", Class: "Equity MF", Geography: "India", Allocation: 0.25, ExpectedReturn: 0.14, ExpectedVol: 0.18, Reason: "Core large-cap India equity; consistent alpha over Nifty"},
			{Name: "PPFAS Flexi Cap MF", Class: "Equity MF", Geography: "IN+US", Allocation: 0.15, ExpectedReturn: 0.16, ExpectedVol: 0.19, Reason: "Value-oriented, global flexibility; best-in-class long-term track record"},
			{Name: "Nifty Midcap 150 Index Fund", Class: "Equity MF", Geography: "India", Allocation: 0.15, ExpectedReturn: 0.17, ExpectedVol: 0.22, Reason: "Mid-cap allocation for growth; index approach avoids active risk"},
			{Name: "Nippon India Small Cap MF", Class: "Equity MF", Geography: "India", Allocation: 0.10, ExpectedReturn: 0.20, ExpectedVol: 0.27, Reason: "High growth, higher risk; capped at 10% to control portfolio vol"},
			{Name: "US Nasdaq 100 (via MF/ETF)", Class: "Global Equity", Geography: "US", Allocation: 0.12, ExpectedReturn: 0.15, ExpectedVol: 0.20, Reason: "Tech/growth exposure in USD; diversifies away from India-specific risk"},
			{Name: "Sovereign Gold Bond", Class: "Gold", Geography: "India", Allocation: 0.10, ExpectedReturn: 0.09, ExpectedVol: 0.12, Reason: "Portfolio hedge; performs well in risk-off regimes"},
			{Name: "HDFC Corporate Bond MF", Class: "Debt MF", Geography: "India", Allocation: 0.08, ExpectedReturn: 0.08, ExpectedVol: 0.03, Reason: "Income component; reduces overall portfolio volatility"},
			{Name: "Cash / Overnight Fund", Class: "Cash", Geography: "India", Allocation: 0.05, ExpectedReturn: 0.065, ExpectedVol: 0.001, Reason: "Dry powder for opportunities; tactical rebalancing buffer"},
		}
	default: // HIGH
		return []Asset{
			{Name: "Direct Indian Stocks (Momentum Screen)", Class: "Stocks", Geography: "India", Allocation: 0.25, ExpectedReturn: 0.18, ExpectedVol: 0.25, Reason: "Highest alpha potential; active momentum picks from Nifty 500"},
			{Name: "Nippon Small Cap + Quant Small Cap MF", Class: "Equity MF", Geography: "India", Allocation: 0.20, ExpectedReturn: 0.22, ExpectedVol: 0.30, Reason: "Small-cap for high compounding; India's small-cap CAGR has been 18-22% over 10Y"},
			{Name: "US Tech Direct Stocks (FAANG+)", Class: "Stocks", Geography: "US", Allocation: 0.20, ExpectedReturn: 0.18, ExpectedVol: 0.25, Reason: "Direct stock alpha in world's most liquid market; USD appreciation bonus"},
			{Name: "Sectoral Funds (Infra / Pharma / IT)", Class: "Sector MF", Geography: "India", Allocation: 0.15, ExpectedReturn: 0.20, ExpectedVol: 0.28, Reason: "Concentrated sector bets on multi-year themes; high risk/high reward"},
			{Name: "PPFAS Flexi Cap MF", Class: "Equity MF", Geography: "IN+US", Allocation: 0.10, ExpectedReturn: 0.16, ExpectedVol: 0.19, Reason: "Quality anchor; prevents chasing momentum exclusively"},
			{Name: "Sovereign Gold Bond", Class: "Gold", Geography: "India", Allocation: 0.05, ExpectedReturn: 0.09, ExpectedVol: 0.12, Reason: "Minimal hedge; reduces tail risk without drag on returns"},
			{Name: "Cash / Liquid Fund", Class: "Cash", Geography: "India", Allocation: 0.05, ExpectedReturn: 0.065, ExpectedVol: 0.001, Reason: "Tactical reserve for adding to drawdowns in quality positions"},
		}
	}
}

func portfolioStats(assets []Asset) (float64, float64) {
	er, ev := 0.0, 0.0
	for _, a := range assets {
		er += a.Allocation * a.ExpectedReturn
		ev += a.Allocation * a.ExpectedVol * a.Allocation * a.ExpectedVol
	}
	// Add diversification benefit (off-diagonal correlation terms ≈ 0.3 avg)
	totalVol := 0.0
	for _, a := range assets {
		totalVol += a.Allocation * a.ExpectedVol
	}
	ev = math.Sqrt(ev + 0.3*totalVol*totalVol*0.5) // simplified
	return er, ev
}

func PrintPortfolio(p Portfolio) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "═══════════════════════════════════════════════════════\n")
	fmt.Fprintf(&sb, "PORTFOLIO: %s\n", p.Name)
	fmt.Fprintf(&sb, "Capital: $%.0f  |  Risk: %s  |  Horizon: %s\n", p.TotalCapital, p.RiskTolerance, p.Horizon)
	fmt.Fprintf(&sb, "───────────────────────────────────────────────────────\n")

	fmt.Fprintf(&sb, "\n%-38s %8s %10s %10s   %s\n", "Asset", "Alloc%", "Exp.Ret", "Exp.Vol", "Reason")
	fmt.Fprintf(&sb, "%s\n", strings.Repeat("-", 110))
	for _, a := range p.Assets {
		capitalAmt := p.TotalCapital * a.Allocation
		fmt.Fprintf(&sb, "%-38s %7.0f%% %9.1f%% %9.1f%%   %s\n",
			a.Name, a.Allocation*100, a.ExpectedReturn*100, a.ExpectedVol*100, a.Reason)
		fmt.Fprintf(&sb, "   [%s | %s | $%.0f]\n", a.Class, a.Geography, capitalAmt)
	}

	fmt.Fprintf(&sb, "\nPORTFOLIO SUMMARY\n")
	fmt.Fprintf(&sb, "  Expected Annual Return:  %.1f%%\n", p.ExpectedReturn*100)
	fmt.Fprintf(&sb, "  Expected Volatility:     %.1f%%\n", p.ExpectedVol*100)
	fmt.Fprintf(&sb, "  Expected Max Drawdown:   ~%.1f%%\n", p.ExpectedDrawdown*100)
	fmt.Fprintf(&sb, "  Estimated Sharpe Ratio:  %.2f\n", p.SharpeEstimate)
	fmt.Fprintf(&sb, "═══════════════════════════════════════════════════════\n")
	return sb.String()
}
