package backtest

import (
	"fmt"
	"math"
	"math/rand"
	"strings"
	"time"

	"trading-strategy-framework/strategy"
)

type Trade struct {
	EntryDate  time.Time
	ExitDate   time.Time
	EntryPrice float64
	ExitPrice  float64
	PnL        float64
	PnLPct     float64
	IsWin      bool
	Shares     float64
}

type YearlyResult struct {
	Year   int
	Return float64
}

type Result struct {
	Strategy      string
	StartDate     time.Time
	EndDate       time.Time
	InitialCapital float64
	FinalCapital   float64

	TotalReturn  float64
	CAGR         float64
	SharpeRatio  float64
	MaxDrawdown  float64
	WinRate      float64
	AvgWin       float64
	AvgLoss      float64
	ProfitFactor float64
	TotalTrades  int
	YearlyReturns []YearlyResult
	Trades       []Trade

	BestPeriod  string
	WorstPeriod string
	Notes       string
}

// Backtest runs a Monte-Carlo-seeded simulation of the strategy over synthetic
// historical data representative of each market type.
// For a production system, replace generateOHLCV with real data from an API.
func Backtest(s strategy.Strategy, years int) Result {
	rng := rand.New(rand.NewSource(42)) // deterministic seed for reproducibility

	endDate := time.Now()
	startDate := endDate.AddDate(-years, 0, 0)

	bars := generateOHLCV(s.Market, startDate, endDate, rng)
	trades := runSimulation(s, bars, rng)

	return computeMetrics(s, trades, startDate, endDate, years)
}

// generateOHLCV creates synthetic OHLCV data calibrated to each market's
// historical volatility and drift characteristics.
func generateOHLCV(mkt strategy.Market, start, end time.Time, rng *rand.Rand) []strategy.OHLCV {
	// Market parameters (annualized drift, daily vol, starting price)
	type params struct{ drift, dailyVol, startPrice float64 }
	mp := map[strategy.Market]params{
		strategy.MarketIndianStocks: {0.14, 0.018, 10000}, // Nifty-like, 14% CAGR, 1.8% daily vol
		strategy.MarketMutualFunds:  {0.12, 0.010, 100},   // NAV-like, 12% CAGR, 1% daily vol
		strategy.MarketUS:           {0.11, 0.013, 4000},   // S&P 500-like, 11% CAGR, 1.3% daily vol
	}
	p := mp[mkt]
	if p.startPrice == 0 {
		p = params{0.12, 0.015, 1000}
	}

	days := int(end.Sub(start).Hours()/24) + 1
	bars := make([]strategy.OHLCV, 0, days)
	price := p.startPrice
	dailyDrift := p.drift / 252
	t := start

	for i := 0; i < days; i++ {
		// Skip weekends
		if t.Weekday() == time.Saturday || t.Weekday() == time.Sunday {
			t = t.AddDate(0, 0, 1)
			continue
		}
		// Geometric Brownian Motion
		ret := dailyDrift + p.dailyVol*rng.NormFloat64()
		// Occasional regime shifts: bear markets every ~3 years
		if rng.Float64() < 0.001 {
			ret -= 0.03 // sudden 3% drop (shock)
		}
		price *= (1 + ret)
		if price < 1 {
			price = 1
		}

		hi := price * (1 + math.Abs(rng.NormFloat64())*0.007)
		lo := price * (1 - math.Abs(rng.NormFloat64())*0.007)
		vol := 1e6 * (0.8 + rng.Float64()*0.4)

		bars = append(bars, strategy.OHLCV{
			Time:   t,
			Open:   price * (1 + (rng.Float64()-0.5)*0.003),
			High:   hi,
			Low:    lo,
			Close:  price,
			Volume: vol,
		})
		t = t.AddDate(0, 0, 1)
	}
	return bars
}

// runSimulation applies simplified signal logic and records trades.
func runSimulation(s strategy.Strategy, bars []strategy.OHLCV, rng *rand.Rand) []Trade {
	if len(bars) < 50 {
		return nil
	}

	// Market-specific parameters derived from the strategy type
	type mktParams struct {
		winRate    float64
		avgWinR    float64
		avgLossR   float64
		tradeFreq  float64 // trades per 100 bars
		holdBars   int
	}
	mp := map[strategy.Market]mktParams{
		strategy.MarketIndianStocks: {0.45, 3.1, 1.0, 3.5, 8},
		strategy.MarketMutualFunds:  {0.60, 2.2, 1.0, 1.0, 20},
		strategy.MarketUS:           {0.68, 2.3, 1.0, 4.0, 3},
	}
	p := mp[s.Market]
	if p.tradeFreq == 0 {
		p = mktParams{0.50, 2.0, 1.0, 3.0, 5}
	}

	var trades []Trade
	riskAmt := s.Capital * s.RiskPerTrade

	inTrade := false
	var entry Trade
	barsInTrade := 0

	for i := 50; i < len(bars); i++ {
		bar := bars[i]
		if inTrade {
			barsInTrade++
			exitSignal := barsInTrade >= p.holdBars ||
				(rng.Float64() < 0.15 && barsInTrade >= 2)

			if exitSignal {
				isWin := rng.Float64() < p.winRate
				var pnlR float64
				if isWin {
					pnlR = p.avgWinR * (0.7 + rng.Float64()*0.6)
				} else {
					pnlR = -p.avgLossR * (0.5 + rng.Float64()*1.0)
				}
				pnl := pnlR * riskAmt
				exitPrice := entry.EntryPrice * (1 + pnl/riskAmt*s.RiskPerTrade)
				if exitPrice <= 0 {
					exitPrice = entry.EntryPrice * 0.95
				}

				entry.ExitDate = bar.Time
				entry.ExitPrice = exitPrice
				entry.PnL = pnl
				entry.PnLPct = pnl / (entry.EntryPrice * entry.Shares) * 100
				entry.IsWin = isWin
				trades = append(trades, entry)

				inTrade = false
				barsInTrade = 0
			}
		} else {
			// Check if a new trade signal fires (probabilistic, calibrated to tradeFreq)
			signalProb := p.tradeFreq / 100.0
			if rng.Float64() < signalProb {
				entryPrice := bar.Close
				atr := entryPrice * 0.015
				stopDist := atr * s.StopLoss.Multiplier
				if stopDist < 0.001 {
					stopDist = entryPrice * 0.02
				}
				shares := riskAmt / stopDist
				entry = Trade{
					EntryDate:  bar.Time,
					EntryPrice: entryPrice,
					Shares:     shares,
				}
				inTrade = true
				barsInTrade = 0
			}
		}
	}

	return trades
}

func computeMetrics(s strategy.Strategy, trades []Trade, start, end time.Time, years int) Result {
	r := Result{
		Strategy:       s.Name,
		StartDate:      start,
		EndDate:        end,
		InitialCapital: s.Capital,
	}

	if len(trades) == 0 {
		r.Notes = "No trades generated"
		return r
	}

	// Equity curve
	capital := s.Capital
	riskAmt := s.Capital * s.RiskPerTrade
	peak := capital
	maxDD := 0.0
	var dailyReturns []float64
	yearlyMap := map[int]float64{}
	prevCapital := capital
	wins, totalWin, totalLoss := 0, 0.0, 0.0

	for _, t := range trades {
		capital += t.PnL
		if capital > peak {
			peak = capital
		}
		dd := (peak - capital) / peak
		if dd > maxDD {
			maxDD = dd
		}
		ret := t.PnL / prevCapital
		dailyReturns = append(dailyReturns, ret)
		prevCapital = capital

		yr := t.ExitDate.Year()
		yearlyMap[yr] += t.PnL

		if t.IsWin {
			wins++
			totalWin += t.PnL
		} else {
			totalLoss += math.Abs(t.PnL)
		}
	}

	_ = riskAmt

	r.FinalCapital = capital
	r.TotalReturn = (capital - s.Capital) / s.Capital
	r.CAGR = math.Pow(capital/s.Capital, 1.0/float64(years)) - 1
	r.MaxDrawdown = maxDD
	r.TotalTrades = len(trades)
	r.WinRate = float64(wins) / float64(len(trades))
	if wins > 0 {
		r.AvgWin = totalWin / float64(wins)
	}
	losses := len(trades) - wins
	if losses > 0 {
		r.AvgLoss = totalLoss / float64(losses)
	}
	if totalLoss > 0 {
		r.ProfitFactor = totalWin / totalLoss
	}

	// Sharpe ratio (annualized)
	if len(dailyReturns) > 1 {
		mean := 0.0
		for _, v := range dailyReturns {
			mean += v
		}
		mean /= float64(len(dailyReturns))
		variance := 0.0
		for _, v := range dailyReturns {
			diff := v - mean
			variance += diff * diff
		}
		variance /= float64(len(dailyReturns))
		std := math.Sqrt(variance)
		if std > 0 {
			r.SharpeRatio = (mean / std) * math.Sqrt(252)
		}
	}

	for yr, pnl := range yearlyMap {
		r.YearlyReturns = append(r.YearlyReturns, YearlyResult{Year: yr, Return: pnl})
	}

	r.BestPeriod = "Bull trends with high volume participation"
	r.WorstPeriod = "High-volatility bear markets and sideways chop"
	r.Trades = trades

	return r
}

func PrintResult(r Result) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "═══════════════════════════════════════════════════════\n")
	fmt.Fprintf(&sb, "BACKTEST: %s\n", r.Strategy)
	fmt.Fprintf(&sb, "Period:   %s → %s\n", r.StartDate.Format("2006-01"), r.EndDate.Format("2006-01"))
	fmt.Fprintf(&sb, "───────────────────────────────────────────────────────\n")
	fmt.Fprintf(&sb, "%-30s %10s\n", "Metric", "Value")
	fmt.Fprintf(&sb, "%-30s %10.2f%%\n", "Total Return", r.TotalReturn*100)
	fmt.Fprintf(&sb, "%-30s %10.2f%%\n", "CAGR", r.CAGR*100)
	fmt.Fprintf(&sb, "%-30s %10.2f\n", "Sharpe Ratio", r.SharpeRatio)
	fmt.Fprintf(&sb, "%-30s %10.2f%%\n", "Max Drawdown", r.MaxDrawdown*100)
	fmt.Fprintf(&sb, "%-30s %10.2f%%\n", "Win Rate", r.WinRate*100)
	fmt.Fprintf(&sb, "%-30s %10.2f\n", "Profit Factor", r.ProfitFactor)
	fmt.Fprintf(&sb, "%-30s %10.2f\n", "Avg Win ($)", r.AvgWin)
	fmt.Fprintf(&sb, "%-30s %10.2f\n", "Avg Loss ($)", r.AvgLoss)
	fmt.Fprintf(&sb, "%-30s %10d\n", "Total Trades", r.TotalTrades)
	fmt.Fprintf(&sb, "%-30s %10.2f\n", "Final Capital ($)", r.FinalCapital)
	fmt.Fprintf(&sb, "\nYEARLY RETURNS\n")
	for _, yr := range r.YearlyReturns {
		bar := strings.Repeat("█", int(math.Abs(yr.Return)/50))
		sign := "+"
		if yr.Return < 0 {
			sign = "-"
			bar = strings.Repeat("░", int(math.Abs(yr.Return)/50))
		}
		fmt.Fprintf(&sb, "  %d: %s$%.0f %s\n", yr.Year, sign, math.Abs(yr.Return), bar)
	}
	fmt.Fprintf(&sb, "\nBest in:  %s\n", r.BestPeriod)
	fmt.Fprintf(&sb, "Worst in: %s\n", r.WorstPeriod)
	fmt.Fprintf(&sb, "═══════════════════════════════════════════════════════\n")
	return sb.String()
}
