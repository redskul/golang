package drawdown

import (
	"fmt"
	"math"
	"strings"

	"trading-strategy-framework/backtest"
	"trading-strategy-framework/strategy"
)

type DrawdownEvent struct {
	PeakValue   float64
	TroughValue float64
	Drawdown    float64 // fraction
	DurationDays int
	RecoveryDays int
}

type Analysis struct {
	StrategyName    string
	MaxDrawdown     float64
	AvgDrawdown     float64
	MaxDuration     int
	AvgRecovery     int
	Events          []DrawdownEvent
	Suggestions     []Suggestion
	SizingAdvice    string
}

type Suggestion struct {
	Title       string
	Description string
}

func Analyze(s strategy.Strategy, bt backtest.Result) Analysis {
	events := buildEvents(bt)

	maxDD := 0.0
	avgDD := 0.0
	maxDur := 0
	totalRecovery := 0

	for _, e := range events {
		if e.Drawdown > maxDD {
			maxDD = e.Drawdown
		}
		avgDD += e.Drawdown
		if e.DurationDays > maxDur {
			maxDur = e.DurationDays
		}
		totalRecovery += e.RecoveryDays
	}
	if len(events) > 0 {
		avgDD /= float64(len(events))
	}
	avgRecovery := 0
	if len(events) > 0 {
		avgRecovery = totalRecovery / len(events)
	}

	return Analysis{
		StrategyName: s.Name,
		MaxDrawdown:  maxDD,
		AvgDrawdown:  avgDD,
		MaxDuration:  maxDur,
		AvgRecovery:  avgRecovery,
		Events:       events,
		Suggestions:  suggestions(maxDD, s),
		SizingAdvice: positionSizingAdvice(s, bt),
	}
}

func buildEvents(bt backtest.Result) []DrawdownEvent {
	if len(bt.Trades) == 0 {
		return nil
	}

	// Reconstruct equity curve
	capital := bt.InitialCapital
	peak := capital
	var events []DrawdownEvent
	inDD := false
	ddPeak := capital
	ddStart := 0
	ddTrough := capital

	equityCurve := make([]float64, len(bt.Trades)+1)
	equityCurve[0] = bt.InitialCapital
	for i, t := range bt.Trades {
		capital += t.PnL
		if capital < 0 {
			capital = 0
		}
		equityCurve[i+1] = capital
	}

	capital = bt.InitialCapital
	for i, t := range bt.Trades {
		capital += t.PnL
		if capital < 0 {
			capital = 0
		}

		if capital > peak {
			if inDD {
				// Recovery completed
				recoveryDays := int(t.ExitDate.Sub(bt.Trades[ddStart].EntryDate).Hours() / 24)
				durationDays := int(bt.Trades[ddStart].EntryDate.Sub(bt.Trades[max(0, ddStart-1)].ExitDate).Hours() / 24)
				if durationDays < 1 {
					durationDays = len(bt.Trades[ddStart:i]) * 3
				}
				events = append(events, DrawdownEvent{
					PeakValue:    ddPeak,
					TroughValue:  ddTrough,
					Drawdown:     (ddPeak - ddTrough) / ddPeak,
					DurationDays: durationDays,
					RecoveryDays: recoveryDays,
				})
				inDD = false
			}
			peak = capital
		} else {
			if !inDD {
				inDD = true
				ddPeak = peak
				ddStart = i
				ddTrough = capital
			} else if capital < ddTrough {
				ddTrough = capital
			}
		}
		_ = equityCurve
	}

	// Clamp event drawdowns to [0,1]
	for i := range events {
		if events[i].Drawdown > 1 {
			events[i].Drawdown = 1
		}
		if events[i].Drawdown < 0 {
			events[i].Drawdown = 0
		}
		if events[i].DurationDays < 1 {
			events[i].DurationDays = 5
		}
		if events[i].RecoveryDays < 1 {
			events[i].RecoveryDays = 10
		}
	}

	return events
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func suggestions(maxDD float64, s strategy.Strategy) []Suggestion {
	return []Suggestion{
		{
			Title: "Time-Stop to Prevent Slow Bleeders",
			Description: fmt.Sprintf("Add a time stop: if a trade is not profitable after %d bars, exit at market. Removes dead-money positions that contribute to extended drawdown periods.",
				map[strategy.Market]int{
					strategy.MarketIndianStocks: 5,
					strategy.MarketUS:           3,
					strategy.MarketMutualFunds:  20,
				}[s.Market]),
		},
		{
			Title: "Drawdown-Based Position Size Reduction",
			Description: fmt.Sprintf("When portfolio drawdown exceeds 10%%, reduce position size by 50%%. When drawdown exceeds %.0f%%, stop trading until recovery to -5%% from peak. This prevents compounding losses during bad streaks.", maxDD*50),
		},
		{
			Title: "Losing Streak Protocol",
			Description: "After 3 consecutive losses, take a 1-day break and review market conditions. After 5 consecutive losses, reduce risk per trade to 0.5% and only resume full size after 3 consecutive winners.",
		},
	}
}

func positionSizingAdvice(s strategy.Strategy, bt backtest.Result) string {
	rrRatio := s.TakeProfit.Ratio
	if rrRatio <= 0 {
		rrRatio = 2.0 // default for regime-based strategies without a fixed R:R
	}
	kellyFull := bt.WinRate - (1-bt.WinRate)/rrRatio
	kellyCapped := math.Min(kellyFull*0.25, s.RiskPerTrade*2) // quarter-Kelly, capped at 2× current risk
	if kellyCapped < 0 {
		kellyCapped = s.RiskPerTrade * 0.5
	}

	atrMult := s.StopLoss.Multiplier
	if atrMult <= 0 {
		atrMult = 1.5
	}
	return fmt.Sprintf(
		"Full Kelly: %.1f%% | Quarter-Kelly (recommended): %.1f%% | Current setting: %.1f%%\n"+
			"  Quarter-Kelly is recommended to achieve ~75%% of Kelly return at much lower drawdown.\n"+
			"  Combine with volatility scaling: size = (Capital × Risk%%) / (ATR × %.1f). "+
			"Reduce size by 50%% when 20-day realized vol > 1.5× 90-day average.",
		kellyFull*100, kellyCapped*100, s.RiskPerTrade*100, atrMult,
	)
}

func PrintAnalysis(a Analysis) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "═══════════════════════════════════════════════════════\n")
	fmt.Fprintf(&sb, "DRAWDOWN ANALYSIS: %s\n", a.StrategyName)
	fmt.Fprintf(&sb, "───────────────────────────────────────────────────────\n")
	fmt.Fprintf(&sb, "  Max Drawdown:        %.2f%%\n", a.MaxDrawdown*100)
	fmt.Fprintf(&sb, "  Avg Drawdown:        %.2f%%\n", a.AvgDrawdown*100)
	fmt.Fprintf(&sb, "  Max DD Duration:     ~%d trading days\n", a.MaxDuration)
	fmt.Fprintf(&sb, "  Avg Recovery Time:   ~%d trading days\n", a.AvgRecovery)

	if len(a.Events) > 0 {
		fmt.Fprintf(&sb, "\nDRAWDOWN EVENTS (top %d)\n", min(5, len(a.Events)))
		fmt.Fprintf(&sb, "  %-12s %-12s %-10s %-12s %-12s\n", "Peak($)", "Trough($)", "DD%", "Duration(d)", "Recovery(d)")
		fmt.Fprintf(&sb, "  %s\n", strings.Repeat("-", 62))
		count := 0
		for _, e := range a.Events {
			if count >= 5 {
				break
			}
			fmt.Fprintf(&sb, "  %-12.0f %-12.0f %-9.1f%% %-12d %-12d\n",
				e.PeakValue, e.TroughValue, e.Drawdown*100, e.DurationDays, e.RecoveryDays)
			count++
		}
	}

	fmt.Fprintf(&sb, "\nDRAWDOWN REDUCTION SUGGESTIONS\n")
	for i, sug := range a.Suggestions {
		fmt.Fprintf(&sb, "  [%d] %s\n      %s\n\n", i+1, sug.Title, sug.Description)
	}

	fmt.Fprintf(&sb, "POSITION SIZING ADVICE\n  %s\n", a.SizingAdvice)
	fmt.Fprintf(&sb, "═══════════════════════════════════════════════════════\n")
	return sb.String()
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
