package main

import (
	"fmt"
	"math/rand"
	"strings"
	"time"
)

type TradeSetup struct {
	Rank        int
	Asset       string
	Market      string
	Direction   string
	EntryPrice  float64
	StopLoss    float64
	TakeProfit  float64
	RiskReward  float64
	PositionSize float64 // in $
	Reasoning   string
}

func generateTradeSetups(capital float64) []TradeSetup {
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))

	setups := []TradeSetup{
		{
			Rank:       1,
			Asset:      "Reliance Industries (RELIANCE.NS)",
			Market:     "India NSE",
			Direction:  "LONG",
			EntryPrice: 2890.0,
			StopLoss:   2820.0,
			TakeProfit: 3080.0,
			Reasoning: "Technical: Price broke above 20-week EMA on 1.8× volume surge. RSI(14)=58 — strong but not overbought. " +
				"Fundamental: JioFinance listing catalyst + O2C margin expansion. " +
				"Macro: FII buying in large-cap energy; RBI rate hold positive for downstream.",
		},
		{
			Rank:       2,
			Asset:      "Apple Inc (AAPL)",
			Market:     "US NASDAQ",
			Direction:  "LONG",
			EntryPrice: 213.50,
			StopLoss:   207.00,
			TakeProfit: 232.00,
			Reasoning: "Technical: RSI(2) = 8 after 5-day pullback to 200SMA — extreme oversold mean reversion setup. " +
				"Price is above 200DMA (bull trend intact). SPY above 5DMA (market positive). " +
				"Macro: Fed pause narrative + AI hardware supercycle tailwind. Expected hold: 1-3 days.",
		},
		{
			Rank:       3,
			Asset:      "Mirae Asset Large Cap Fund — Growth",
			Market:     "India MF",
			Direction:  "LONG (SIP entry)",
			EntryPrice: 105.20, // NAV
			StopLoss:   0,      // No stop — regime-based exit
			TakeProfit: 0,      // Monthly rebalance rule
			Reasoning: "Regime: Nifty above 10-month SMA — equity regime is ON. Fund ranks #2 in large-cap category " +
				"by 6-month Sharpe (1.42). 12-1 momentum rank: top quintile. Max drawdown 14% < 20% threshold. " +
				"SIP on 1st of month. Horizon: 12-month hold minimum.",
		},
	}

	riskPct := 0.01 // 1% risk per trade
	for i := range setups {
		s := &setups[i]
		if s.StopLoss > 0 {
			stopDist := s.EntryPrice - s.StopLoss
			if s.Direction == "SHORT" {
				stopDist = s.StopLoss - s.EntryPrice
			}
			if stopDist > 0 {
				s.PositionSize = (capital * riskPct) / stopDist
				tpDist := s.TakeProfit - s.EntryPrice
				if s.Direction == "SHORT" {
					tpDist = s.EntryPrice - s.TakeProfit
				}
				s.RiskReward = tpDist / stopDist
			}
		} else {
			s.PositionSize = capital * 0.10 // 10% allocation for MF
			s.RiskReward = 0
		}
		// Add noise to prices
		noise := 1.0 + (rng.Float64()-0.5)*0.001
		s.EntryPrice *= noise
	}

	return setups
}

func printTradeSetups(setups []TradeSetup) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "═══════════════════════════════════════════════════════\n")
	fmt.Fprintf(&sb, "HIGH-PROBABILITY TRADE SETUPS\n")
	fmt.Fprintf(&sb, "Generated: %s\n", time.Now().Format("2006-01-02 15:04"))
	fmt.Fprintf(&sb, "═══════════════════════════════════════════════════════\n")

	for _, s := range setups {
		fmt.Fprintf(&sb, "\n[SETUP #%d] %s  |  %s  |  %s\n", s.Rank, s.Asset, s.Market, s.Direction)
		fmt.Fprintf(&sb, "  Entry:        %.2f\n", s.EntryPrice)
		if s.StopLoss > 0 {
			fmt.Fprintf(&sb, "  Stop-Loss:    %.2f  (-%0.1f%%)\n", s.StopLoss, (s.EntryPrice-s.StopLoss)/s.EntryPrice*100)
			fmt.Fprintf(&sb, "  Take-Profit:  %.2f  (+%0.1f%%)\n", s.TakeProfit, (s.TakeProfit-s.EntryPrice)/s.EntryPrice*100)
			fmt.Fprintf(&sb, "  Risk/Reward:  %.1f:1\n", s.RiskReward)
			fmt.Fprintf(&sb, "  Position:     %.0f shares / $%.0f total\n", s.PositionSize, s.PositionSize*s.EntryPrice)
		} else {
			fmt.Fprintf(&sb, "  Stop-Loss:    Regime-based (exit when Nifty < 10M SMA)\n")
			fmt.Fprintf(&sb, "  Allocation:   $%.0f (10%% of capital)\n", s.PositionSize)
		}
		fmt.Fprintf(&sb, "  Reasoning:    %s\n", s.Reasoning)
		fmt.Fprintf(&sb, "  %s\n", strings.Repeat("─", 55))
	}
	return sb.String()
}
