package regime

import (
	"fmt"
	"math"
	"math/rand"
	"strings"
	"time"

	"trading-strategy-framework/strategy"
)

type Trend string
type VolLevel string
type VolumeState string

const (
	TrendBull     Trend = "BULL"
	TrendBear     Trend = "BEAR"
	TrendSideways Trend = "SIDEWAYS"

	VolLow    VolLevel = "LOW"
	VolMedium VolLevel = "MEDIUM"
	VolHigh   VolLevel = "HIGH"

	VolumeSurge   VolumeState = "SURGE"
	VolumeNormal  VolumeState = "NORMAL"
	VolumeDrying  VolumeState = "DRYING"
)

type Regime struct {
	Asset        string
	AnalyzedAt   time.Time
	Trend        Trend
	TrendScore   float64 // 0-100
	Volatility   VolLevel
	AnnualizedVol float64
	VIX          float64
	Volume       VolumeState
	VolumeRatio  float64 // current vs 20-day avg

	RecommendedStrategyType string
	RecommendedStrategies   []string
	Avoid                   []string
	Rationale               string
}

// Detect analyzes the market regime for a given asset.
// In production, replace synthetic generation with real OHLCV data.
func Detect(asset string, mkt strategy.Market) Regime {
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	bars := generateRecentBars(mkt, rng)
	return analyzeRegime(asset, mkt, bars, rng)
}

func generateRecentBars(mkt strategy.Market, rng *rand.Rand) []strategy.OHLCV {
	type p struct{ drift, vol, price float64 }
	params := map[strategy.Market]p{
		strategy.MarketIndianStocks: {0.14, 0.018, 22000},
		strategy.MarketMutualFunds:  {0.12, 0.010, 100},
		strategy.MarketUS:           {0.11, 0.013, 5300},
	}
	pm := params[mkt]
	if pm.price == 0 {
		pm = p{0.12, 0.015, 1000}
	}

	bars := make([]strategy.OHLCV, 60)
	price := pm.price
	t := time.Now().AddDate(0, -3, 0)
	dailyDrift := pm.drift / 252

	for i := 0; i < 60; i++ {
		ret := dailyDrift + pm.vol*rng.NormFloat64()
		price *= (1 + ret)
		bars[i] = strategy.OHLCV{
			Time:   t.AddDate(0, 0, i),
			Close:  price,
			High:   price * (1 + math.Abs(rng.NormFloat64())*0.005),
			Low:    price * (1 - math.Abs(rng.NormFloat64())*0.005),
			Volume: 1e6 * (0.8 + rng.Float64()*0.4),
		}
	}
	return bars
}

func analyzeRegime(asset string, mkt strategy.Market, bars []strategy.OHLCV, rng *rand.Rand) Regime {
	n := len(bars)
	if n < 20 {
		return Regime{Asset: asset, AnalyzedAt: time.Now()}
	}

	// SMA20 and SMA50
	sma20 := sma(bars, 20)
	sma50 := sma(bars, 50)
	current := bars[n-1].Close
	first := bars[0].Close

	// Trend score
	trendScore := 0.0
	if current > sma20 {
		trendScore += 30
	}
	if sma20 > sma50 {
		trendScore += 30
	}
	if current > first {
		gain := (current - first) / first
		trendScore += math.Min(40, gain*200)
	}

	var trend Trend
	switch {
	case trendScore >= 60:
		trend = TrendBull
	case trendScore <= 30:
		trend = TrendBear
	default:
		trend = TrendSideways
	}

	// Volatility
	returns := make([]float64, n-1)
	for i := 1; i < n; i++ {
		returns[i-1] = (bars[i].Close - bars[i-1].Close) / bars[i-1].Close
	}
	stdDev := stddev(returns)
	annVol := stdDev * math.Sqrt(252) * 100

	var vol VolLevel
	switch {
	case annVol > 25:
		vol = VolHigh
	case annVol > 15:
		vol = VolMedium
	default:
		vol = VolLow
	}

	vix := annVol * (0.8 + rng.Float64()*0.4)

	// Volume
	avgVol := 0.0
	for _, b := range bars[n-20:] {
		avgVol += b.Volume
	}
	avgVol /= 20
	recentVol := bars[n-1].Volume
	volRatio := recentVol / avgVol

	var volumeState VolumeState
	switch {
	case volRatio > 1.3:
		volumeState = VolumeSurge
	case volRatio < 0.8:
		volumeState = VolumeDrying
	default:
		volumeState = VolumeNormal
	}

	recommended, avoid, stratType, rationale := recommendations(trend, vol, volumeState, mkt)

	return Regime{
		Asset:                   asset,
		AnalyzedAt:              time.Now(),
		Trend:                   trend,
		TrendScore:              trendScore,
		Volatility:              vol,
		AnnualizedVol:           annVol,
		VIX:                     vix,
		Volume:                  volumeState,
		VolumeRatio:             volRatio,
		RecommendedStrategyType: stratType,
		RecommendedStrategies:   recommended,
		Avoid:                   avoid,
		Rationale:               rationale,
	}
}

func recommendations(trend Trend, vol VolLevel, volume VolumeState, mkt strategy.Market) (rec, avoid []string, stratType, rationale string) {
	switch {
	case trend == TrendBull && vol == VolLow:
		stratType = "Momentum / Trend-Following"
		rec = []string{"EMA crossover breakouts", "52-week high breakouts", "Sector rotation into leaders", "SIP in equity mutual funds"}
		avoid = []string{"Short selling", "Mean reversion strategies", "Inverse ETFs", "High cash positions"}
		rationale = "Low volatility bull trend is the ideal environment for momentum strategies. Breakouts are more likely to follow through, and trend-following systems capture extended moves."
	case trend == TrendBull && vol == VolHigh:
		stratType = "Selective Momentum with Tight Risk"
		rec = []string{"Only high-quality large-cap breakouts", "Wider stops (2×ATR vs 1.5×ATR)", "Smaller position sizes (0.5% risk vs 1%)"}
		avoid = []string{"Small/micro-cap breakouts", "Full-size positions", "Leveraged ETFs"}
		rationale = "High volatility in a bull market means choppiness and false breakouts. Reduce size, tighten universe, keep winners running longer."
	case trend == TrendBear && vol == VolHigh:
		stratType = "Defensive / Cash-Heavy"
		rec = []string{"Move 70% to liquid/overnight funds", "Short duration government bonds", "Gold ETFs/funds", "Sell covered calls on existing holdings"}
		avoid = []string{"New equity longs", "Small/midcap exposure", "Sector bets", "Leverage"}
		rationale = "High-vol bear markets produce the worst drawdowns. Capital preservation is the priority. The opportunity cost of holding cash is far less than the cost of a 30%+ drawdown."
	case trend == TrendSideways:
		stratType = "Mean Reversion / Range Trading"
		rec = []string{"RSI(2) oversold entries at range support", "Bollinger Band mean reversion", "Selling options (short straddles/strangles) if IV high", "Balanced/hybrid funds for MF investors"}
		avoid = []string{"Trend-following breakout strategies", "High hold times", "Wide trailing stops"}
		rationale = "In sideways markets, breakout strategies generate many false signals and whipsaws. Mean reversion strategies profit from the lack of trend by fading extremes."
	default:
		stratType = "Mixed / Hedged"
		rec = []string{"Equal-weight momentum + mean reversion", "Multi-asset portfolio rebalancing"}
		avoid = []string{"Single-strategy over-concentration"}
		rationale = "Current regime is mixed. Diversify across strategy types."
	}
	return
}

func sma(bars []strategy.OHLCV, period int) float64 {
	n := len(bars)
	if n < period {
		return 0
	}
	sum := 0.0
	for _, b := range bars[n-period:] {
		sum += b.Close
	}
	return sum / float64(period)
}

func stddev(vals []float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	mean := 0.0
	for _, v := range vals {
		mean += v
	}
	mean /= float64(len(vals))
	variance := 0.0
	for _, v := range vals {
		d := v - mean
		variance += d * d
	}
	return math.Sqrt(variance / float64(len(vals)))
}

func PrintRegime(r Regime) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "═══════════════════════════════════════════════════════\n")
	fmt.Fprintf(&sb, "MARKET REGIME: %s  [%s]\n", r.Asset, r.AnalyzedAt.Format("2006-01-02 15:04"))
	fmt.Fprintf(&sb, "───────────────────────────────────────────────────────\n")
	fmt.Fprintf(&sb, "  Trend:          %-12s (Score: %.0f/100)\n", r.Trend, r.TrendScore)
	fmt.Fprintf(&sb, "  Volatility:     %-12s (Ann. Vol: %.1f%% | VIX: %.1f)\n", r.Volatility, r.AnnualizedVol, r.VIX)
	fmt.Fprintf(&sb, "  Volume:         %-12s (Ratio vs 20d avg: %.2f×)\n", r.Volume, r.VolumeRatio)
	fmt.Fprintf(&sb, "\nRECOMMENDED: %s\n", r.RecommendedStrategyType)
	for _, rec := range r.RecommendedStrategies {
		fmt.Fprintf(&sb, "  ✓ %s\n", rec)
	}
	fmt.Fprintf(&sb, "\nAVOID RIGHT NOW:\n")
	for _, av := range r.Avoid {
		fmt.Fprintf(&sb, "  ✗ %s\n", av)
	}
	fmt.Fprintf(&sb, "\nRATIONALE: %s\n", r.Rationale)
	fmt.Fprintf(&sb, "═══════════════════════════════════════════════════════\n")
	return sb.String()
}
