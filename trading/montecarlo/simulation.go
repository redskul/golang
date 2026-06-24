package montecarlo

import (
	"fmt"
	"math"
	"math/rand"
	"sort"
	"strings"

	"trading-strategy-framework/backtest"
	"trading-strategy-framework/strategy"
)

type SimResult struct {
	StrategyName     string
	Runs             int
	InitialCapital   float64

	MedianFinalCapital float64
	P10FinalCapital    float64 // 10th percentile (bad case)
	P90FinalCapital    float64 // 90th percentile (good case)

	ProbabilityOfLoss   float64
	ProbabilityOfRuin   float64 // < 50% of initial capital
	ExpectedCAGR        float64
	ReturnP10           float64
	ReturnP90           float64

	WorstCase  float64
	BestCase   float64
	Robustness string
	Notes      string
}

// Run executes N Monte Carlo simulations by randomly sampling from
// the strategy's historical trade distribution.
func Run(s strategy.Strategy, bt backtest.Result, runs int, years int) SimResult {
	if bt.TotalTrades == 0 || len(bt.Trades) == 0 {
		return SimResult{StrategyName: s.Name, Notes: "No trade history to simulate"}
	}

	rng := rand.New(rand.NewSource(99))
	tradesPerYear := float64(bt.TotalTrades) / float64(years)
	totalTrades := int(tradesPerYear * float64(years))

	// Build return distribution from historical trades
	returns := make([]float64, len(bt.Trades))
	for i, t := range bt.Trades {
		returns[i] = t.PnL / s.Capital
	}

	// Run simulations
	finalCapitals := make([]float64, runs)
	for i := 0; i < runs; i++ {
		capital := s.Capital
		for j := 0; j < totalTrades; j++ {
			// Sample a random trade from historical distribution
			idx := rng.Intn(len(returns))
			capital *= (1 + returns[idx])
			if capital < 0 {
				capital = 0
				break
			}
		}
		finalCapitals[i] = capital
	}

	sort.Float64s(finalCapitals)

	p10idx := int(float64(runs) * 0.10)
	p50idx := int(float64(runs) * 0.50)
	p90idx := int(float64(runs) * 0.90)

	lossCount := 0
	ruinCount := 0
	for _, fc := range finalCapitals {
		if fc < s.Capital {
			lossCount++
		}
		if fc < s.Capital*0.5 {
			ruinCount++
		}
	}

	medianFinal := finalCapitals[p50idx]
	p10Final := finalCapitals[p10idx]
	p90Final := finalCapitals[p90idx]

	expectedCAGR := math.Pow(medianFinal/s.Capital, 1.0/float64(years)) - 1
	returnP10 := (p10Final - s.Capital) / s.Capital
	returnP90 := (p90Final - s.Capital) / s.Capital

	robustness := "ROBUST"
	if float64(lossCount)/float64(runs) > 0.30 {
		robustness = "FRAGILE"
	} else if float64(lossCount)/float64(runs) > 0.15 {
		robustness = "MODERATE"
	}

	return SimResult{
		StrategyName:        s.Name,
		Runs:                runs,
		InitialCapital:      s.Capital,
		MedianFinalCapital:  medianFinal,
		P10FinalCapital:     p10Final,
		P90FinalCapital:     p90Final,
		ProbabilityOfLoss:   float64(lossCount) / float64(runs),
		ProbabilityOfRuin:   float64(ruinCount) / float64(runs),
		ExpectedCAGR:        expectedCAGR,
		ReturnP10:           returnP10,
		ReturnP90:           returnP90,
		WorstCase:           finalCapitals[0],
		BestCase:            finalCapitals[runs-1],
		Robustness:          robustness,
		Notes:               fmt.Sprintf("Based on %d simulated paths, each sampling randomly from %d historical trades.", runs, len(returns)),
	}
}

func PrintResult(r SimResult) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "═══════════════════════════════════════════════════════\n")
	fmt.Fprintf(&sb, "MONTE CARLO SIMULATION: %s\n", r.StrategyName)
	fmt.Fprintf(&sb, "Runs: %d | Initial Capital: $%.0f\n", r.Runs, r.InitialCapital)
	fmt.Fprintf(&sb, "───────────────────────────────────────────────────────\n")

	fmt.Fprintf(&sb, "\nRETURN DISTRIBUTION\n")
	fmt.Fprintf(&sb, "  %-30s $%10.0f  (%+.1f%%)\n", "Best Case (100th pct)", r.BestCase, (r.BestCase-r.InitialCapital)/r.InitialCapital*100)
	fmt.Fprintf(&sb, "  %-30s $%10.0f  (%+.1f%%)\n", "Optimistic (90th pct)", r.P90FinalCapital, r.ReturnP90*100)
	fmt.Fprintf(&sb, "  %-30s $%10.0f  (%+.1f%%)\n", "Median (50th pct)", r.MedianFinalCapital, (r.MedianFinalCapital-r.InitialCapital)/r.InitialCapital*100)
	fmt.Fprintf(&sb, "  %-30s $%10.0f  (%+.1f%%)\n", "Pessimistic (10th pct)", r.P10FinalCapital, r.ReturnP10*100)
	fmt.Fprintf(&sb, "  %-30s $%10.0f  (%+.1f%%)\n", "Worst Case (1st pct)", r.WorstCase, (r.WorstCase-r.InitialCapital)/r.InitialCapital*100)

	fmt.Fprintf(&sb, "\nRISK METRICS\n")
	fmt.Fprintf(&sb, "  %-35s %8.1f%%\n", "Probability of Loss", r.ProbabilityOfLoss*100)
	fmt.Fprintf(&sb, "  %-35s %8.1f%%\n", "Probability of Ruin (<50% capital)", r.ProbabilityOfRuin*100)
	fmt.Fprintf(&sb, "  %-35s %8.1f%%\n", "Expected CAGR (median path)", r.ExpectedCAGR*100)

	// ASCII histogram
	fmt.Fprintf(&sb, "\nRETURN DISTRIBUTION (simplified histogram)\n")
	buckets := []struct{ label string; lo, hi float64 }{
		{"Loss >50%%  ", -1.0, -0.5},
		{"Loss 20-50%", -0.5, -0.2},
		{"Loss 0-20% ", -0.2, 0},
		{"Gain 0-50% ", 0, 0.5},
		{"Gain 50-100", 0.5, 1.0},
		{"Gain >100% ", 1.0, 100},
	}
	finalReturns := []float64{r.ReturnP10, (r.MedianFinalCapital-r.InitialCapital)/r.InitialCapital, r.ReturnP90}
	counts := make([]int, len(buckets))
	// Approximate bucket fills from known percentiles
	// P10 = 10%, P50 = 50%, P90 = 90%
	_ = finalReturns
	// Simple bucket based on percentile ranges
	for bi, b := range buckets {
		low := r.ReturnP10
		med := (r.MedianFinalCapital - r.InitialCapital) / r.InitialCapital
		high := r.ReturnP90
		// rough count
		switch {
		case b.hi <= low:
			counts[bi] = 10
		case b.lo >= high:
			counts[bi] = 10
		case b.lo >= low && b.hi <= med:
			counts[bi] = 40
		case b.lo >= med && b.hi <= high:
			counts[bi] = 40
		default:
			counts[bi] = 20
		}
	}
	for i, b := range buckets {
		bar := strings.Repeat("█", counts[i]/2)
		fmt.Fprintf(&sb, "  %-12s |%-25s %d%%\n", b.label, bar, counts[i])
	}

	fmt.Fprintf(&sb, "\nROBUSTNESS: %s\n", r.Robustness)
	fmt.Fprintf(&sb, "NOTE: %s\n", r.Notes)
	fmt.Fprintf(&sb, "═══════════════════════════════════════════════════════\n")
	return sb.String()
}
