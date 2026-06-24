package strategy

import "time"

type Market string

const (
	MarketIndianStocks Market = "IN_STOCKS"
	MarketMutualFunds  Market = "IN_MF"
	MarketUS           Market = "US_STOCKS"
)

type Timeframe string

const (
	Timeframe1D Timeframe = "1D"
	Timeframe1H Timeframe = "1H"
)

type Direction string

const (
	Long  Direction = "LONG"
	Short Direction = "SHORT"
)

type Indicator struct {
	Name     string
	Settings map[string]float64
	Purpose  string
}

type EntryRule struct {
	Step      int
	Condition string
	Logic     string
}

type ExitRule struct {
	Step      int
	Condition string
	Logic     string
}

type StopLossLogic struct {
	Type        string  // ATR / Percent / Fixed / Swing
	Multiplier  float64 // for ATR: 1.5x ATR; for Percent: 2%
	Description string
}

type TakeProfitLogic struct {
	Type        string  // RR / Percent / Trailing / Target
	Ratio       float64 // risk-reward ratio
	Description string
}

type Strategy struct {
	Name         string
	Market       Market
	Timeframe    Timeframe
	Capital      float64
	RiskPerTrade float64 // fraction e.g. 0.01 = 1%
	Direction    Direction

	Indicators []Indicator
	EntryRules []EntryRule
	ExitRules  []ExitRule
	StopLoss   StopLossLogic
	TakeProfit TakeProfitLogic

	BestConditions  string
	WorstConditions string
	Edge            string

	GeneratedAt time.Time
}

type OHLCV struct {
	Time   time.Time
	Open   float64
	High   float64
	Low    float64
	Close  float64
	Volume float64
}
