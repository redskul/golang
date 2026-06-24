package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"trading-strategy-framework/backtest"
	"trading-strategy-framework/drawdown"
	"trading-strategy-framework/montecarlo"
	"trading-strategy-framework/multifactor"
	"trading-strategy-framework/optimizer"
	"trading-strategy-framework/portfolio"
	"trading-strategy-framework/regime"
	"trading-strategy-framework/risk"
	"trading-strategy-framework/strategy"
)

func main() {
	var (
		module      = flag.String("module", "all", "Module to run: all|strategies|backtest|risk|regime|multifactor|optimizer|portfolio|montecarlo|drawdown|tradesetup")
		capital     = flag.Float64("capital", 10000, "Starting capital in USD")
		riskPct     = flag.Float64("risk", 1.0, "Risk per trade in percent (1 = 1%)")
		timeframe   = flag.String("tf", "1D", "Timeframe: 1D or 1H")
		years       = flag.Int("years", 7, "Backtest years (5-10)")
		riskProfile = flag.String("risk-profile", "MEDIUM", "Portfolio risk: LOW|MEDIUM|HIGH")
		horizon     = flag.String("horizon", "3Y", "Investment horizon: 1Y|3Y|5Y|10Y")
		mcRuns      = flag.Int("mc-runs", 1000, "Monte Carlo simulation runs")
	)
	flag.Parse()

	tf := strategy.Timeframe(*timeframe)
	riskFrac := *riskPct / 100.0

	sep := func() { fmt.Println(strings.Repeat("─", 60)) }

	run := func(name string) bool {
		return *module == "all" || *module == name
	}

	// ── 1. STRATEGY GENERATION ───────────────────────────────────
	var strategies []strategy.Strategy
	if run("strategies") || run("backtest") || run("risk") || run("optimizer") || run("montecarlo") || run("drawdown") || run("all") {
		strategies = strategy.GenerateStrategies(*capital, riskFrac, tf)
	}

	if run("strategies") {
		fmt.Println("\n╔══════════════════════════════════════════════╗")
		fmt.Println("║        1. STRATEGY GENERATION                ║")
		fmt.Println("╚══════════════════════════════════════════════╝")
		for _, s := range strategies {
			fmt.Print(strategy.PrintStrategy(s))
		}
		sep()
	}

	// ── 2. BACKTESTING ───────────────────────────────────────────
	var btResults []backtest.Result
	for _, s := range strategies {
		btResults = append(btResults, backtest.Backtest(s, *years))
	}

	if run("backtest") {
		fmt.Println("\n╔══════════════════════════════════════════════╗")
		fmt.Println("║        2. BACKTESTING RESULTS                ║")
		fmt.Println("╚══════════════════════════════════════════════╝")
		for _, r := range btResults {
			fmt.Print(backtest.PrintResult(r))
		}
		sep()
	}

	// ── 3. RISK-REWARD ANALYSIS ──────────────────────────────────
	if run("risk") {
		fmt.Println("\n╔══════════════════════════════════════════════╗")
		fmt.Println("║        3. RISK-REWARD ANALYSIS               ║")
		fmt.Println("╚══════════════════════════════════════════════╝")
		for i, s := range strategies {
			fmt.Printf("\n--- %s ---\n", s.Name)
			rp := risk.Analyze(s, btResults[i])
			fmt.Print(risk.PrintRRProfile(rp))
		}
		sep()
	}

	// ── 4. MARKET REGIME DETECTION ──────────────────────────────
	if run("regime") {
		fmt.Println("\n╔══════════════════════════════════════════════╗")
		fmt.Println("║        4. MARKET REGIME DETECTION            ║")
		fmt.Println("╚══════════════════════════════════════════════╝")
		assets := []struct {
			name string
			mkt  strategy.Market
		}{
			{"Nifty 50 (India)", strategy.MarketIndianStocks},
			{"Nifty 500 MF Universe", strategy.MarketMutualFunds},
			{"S&P 500 (US)", strategy.MarketUS},
		}
		for _, a := range assets {
			r := regime.Detect(a.name, a.mkt)
			fmt.Print(regime.PrintRegime(r))
		}
		sep()
	}

	// ── 5. MULTI-FACTOR STRATEGY ─────────────────────────────────
	if run("multifactor") {
		fmt.Println("\n╔══════════════════════════════════════════════╗")
		fmt.Println("║        5. MULTI-FACTOR STRATEGY              ║")
		fmt.Println("╚══════════════════════════════════════════════╝")
		mf := multifactor.BuildStrategy()
		fmt.Print(multifactor.PrintStrategy(mf))
		sep()
	}

	// ── 6. STRATEGY OPTIMIZATION ─────────────────────────────────
	if run("optimizer") {
		fmt.Println("\n╔══════════════════════════════════════════════╗")
		fmt.Println("║        6. STRATEGY OPTIMIZATION              ║")
		fmt.Println("╚══════════════════════════════════════════════╝")
		for i, s := range strategies {
			opt := optimizer.Optimize(s, btResults[i], *years)
			fmt.Print(optimizer.PrintOptimResult(opt))
		}
		sep()
	}

	// ── 7. PORTFOLIO CONSTRUCTION ────────────────────────────────
	if run("portfolio") {
		fmt.Println("\n╔══════════════════════════════════════════════╗")
		fmt.Println("║        7. PORTFOLIO CONSTRUCTION             ║")
		fmt.Println("╚══════════════════════════════════════════════╝")
		riskMap := map[string]portfolio.RiskTolerance{
			"LOW": portfolio.RiskLow, "MEDIUM": portfolio.RiskMedium, "HIGH": portfolio.RiskHigh,
		}
		horizonMap := map[string]portfolio.Horizon{
			"1Y": portfolio.Horizon1Y, "3Y": portfolio.Horizon3Y, "5Y": portfolio.Horizon5Y, "10Y": portfolio.Horizon10Y,
		}
		rt := riskMap[strings.ToUpper(*riskProfile)]
		if rt == "" {
			rt = portfolio.RiskMedium
		}
		h := horizonMap[strings.ToUpper(*horizon)]
		if h == "" {
			h = portfolio.Horizon3Y
		}
		p := portfolio.Build(*capital, rt, h)
		fmt.Print(portfolio.PrintPortfolio(p))
		sep()
	}

	// ── 8. TRADE SETUP GENERATION ────────────────────────────────
	if run("tradesetup") {
		fmt.Println("\n╔══════════════════════════════════════════════╗")
		fmt.Println("║        8. TRADE SETUP GENERATION             ║")
		fmt.Println("╚══════════════════════════════════════════════╝")
		setups := generateTradeSetups(*capital)
		fmt.Print(printTradeSetups(setups))
		sep()
	}

	// ── 9. MONTE CARLO SIMULATION ────────────────────────────────
	if run("montecarlo") {
		fmt.Println("\n╔══════════════════════════════════════════════╗")
		fmt.Println("║        9. MONTE CARLO SIMULATION             ║")
		fmt.Println("╚══════════════════════════════════════════════╝")
		for i, s := range strategies {
			mc := montecarlo.Run(s, btResults[i], *mcRuns, *years)
			fmt.Print(montecarlo.PrintResult(mc))
		}
		sep()
	}

	// ── 10. DRAWDOWN ANALYSIS ────────────────────────────────────
	if run("drawdown") {
		fmt.Println("\n╔══════════════════════════════════════════════╗")
		fmt.Println("║       10. DRAWDOWN ANALYSIS                  ║")
		fmt.Println("╚══════════════════════════════════════════════╝")
		for i, s := range strategies {
			da := drawdown.Analyze(s, btResults[i])
			fmt.Print(drawdown.PrintAnalysis(da))
		}
		sep()
	}

	if *module != "all" && !run("strategies") && !run("backtest") && !run("risk") &&
		!run("regime") && !run("multifactor") && !run("optimizer") && !run("portfolio") &&
		!run("tradesetup") && !run("montecarlo") && !run("drawdown") {
		fmt.Fprintf(os.Stderr, "Unknown module: %s\n", *module)
		flag.Usage()
		os.Exit(1)
	}
}
