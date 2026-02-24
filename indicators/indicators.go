package indicators

import (
	"math"
	"trade-go/models"
)

// Calculate 计算所有技术指标
func Calculate(candles []models.OHLCV) models.TechnicalIndicators {
	n := len(candles)
	if n == 0 {
		return models.TechnicalIndicators{}
	}

	closes := make([]float64, n)
	highs := make([]float64, n)
	lows := make([]float64, n)
	volumes := make([]float64, n)

	for i, c := range candles {
		closes[i] = c.Close
		highs[i] = c.High
		lows[i] = c.Low
		volumes[i] = c.Volume
	}

	sma5 := sma(closes, 5)
	sma20 := sma(closes, 20)
	sma50 := sma(closes, 50)

	ema12 := ema(closes, 12)
	ema26 := ema(closes, 26)
	macdLine := ema12[n-1] - ema26[n-1]

	macdSeries := make([]float64, n)
	for i := range candles {
		macdSeries[i] = ema12[i] - ema26[i]
	}
	macdSignalSeries := ema(macdSeries, 9)
	macdSignal := macdSignalSeries[n-1]
	macdHist := macdLine - macdSignal

	rsiVal := rsi(closes, 14)

	bbMid := sma20[n-1]
	bbStd := rollingStd(closes, 20)
	bbUpper := bbMid + bbStd*2
	bbLower := bbMid - bbStd*2
	bbPos := 0.0
	if bbUpper-bbLower != 0 {
		bbPos = (closes[n-1] - bbLower) / (bbUpper - bbLower)
	}

	volMA := sma(volumes, 20)
	volRatio := 0.0
	if volMA[n-1] != 0 {
		volRatio = volumes[n-1] / volMA[n-1]
	}

	resistance := max20(highs)
	support := min20(lows)

	return models.TechnicalIndicators{
		SMA5:        sma5[n-1],
		SMA20:       sma20[n-1],
		SMA50:       sma50[n-1],
		EMA12:       ema12[n-1],
		EMA26:       ema26[n-1],
		MACD:        macdLine,
		MACDSignal:  macdSignal,
		MACDHist:    macdHist,
		RSI:         rsiVal,
		BBUpper:     bbUpper,
		BBMiddle:    bbMid,
		BBLower:     bbLower,
		BBPosition:  bbPos,
		VolumeMA:    volMA[n-1],
		VolumeRatio: volRatio,
		Resistance:  resistance,
		Support:     support,
	}
}

// AnalyzeTrend 趋势分析
func AnalyzeTrend(candles []models.OHLCV, ind models.TechnicalIndicators) models.TrendAnalysis {
	currentPrice := candles[len(candles)-1].Close

	shortTerm := "下跌"
	if currentPrice > ind.SMA20 {
		shortTerm = "上涨"
	}
	mediumTerm := "下跌"
	if currentPrice > ind.SMA50 {
		mediumTerm = "上涨"
	}

	macdDir := "bearish"
	if ind.MACD > ind.MACDSignal {
		macdDir = "bullish"
	}

	overall := "震荡整理"
	if shortTerm == "上涨" && mediumTerm == "上涨" {
		overall = "强势上涨"
	} else if shortTerm == "下跌" && mediumTerm == "下跌" {
		overall = "强势下跌"
	}

	return models.TrendAnalysis{
		ShortTerm:  shortTerm,
		MediumTerm: mediumTerm,
		MACD:       macdDir,
		Overall:    overall,
		RSILevel:   ind.RSI,
	}
}

// AnalyzeLevels 支撑阻力位
func AnalyzeLevels(candles []models.OHLCV, ind models.TechnicalIndicators) models.LevelsAnalysis {
	currentPrice := candles[len(candles)-1].Close

	pvr := 0.0
	if ind.Resistance != 0 {
		pvr = (ind.Resistance - currentPrice) / currentPrice * 100
	}
	pvs := 0.0
	if ind.Support != 0 {
		pvs = (currentPrice - ind.Support) / ind.Support * 100
	}

	return models.LevelsAnalysis{
		StaticResistance:  ind.Resistance,
		StaticSupport:     ind.Support,
		DynamicResistance: ind.BBUpper,
		DynamicSupport:    ind.BBLower,
		PriceVsResistance: pvr,
		PriceVsSupport:    pvs,
	}
}

// --- helpers ---

func sma(data []float64, period int) []float64 {
	result := make([]float64, len(data))
	for i := range data {
		start := i - period + 1
		if start < 0 {
			start = 0
		}
		sum := 0.0
		for _, v := range data[start : i+1] {
			sum += v
		}
		result[i] = sum / float64(i-start+1)
	}
	return result
}

func ema(data []float64, period int) []float64 {
	result := make([]float64, len(data))
	k := 2.0 / float64(period+1)
	result[0] = data[0]
	for i := 1; i < len(data); i++ {
		result[i] = data[i]*k + result[i-1]*(1-k)
	}
	return result
}

func rsi(data []float64, period int) float64 {
	if len(data) < period+1 {
		return 50
	}
	gains := 0.0
	losses := 0.0
	for i := 1; i <= period; i++ {
		diff := data[i] - data[i-1]
		if diff > 0 {
			gains += diff
		} else {
			losses -= diff
		}
	}
	avgGain := gains / float64(period)
	avgLoss := losses / float64(period)
	for i := period + 1; i < len(data); i++ {
		diff := data[i] - data[i-1]
		if diff > 0 {
			avgGain = (avgGain*float64(period-1) + diff) / float64(period)
			avgLoss = (avgLoss * float64(period-1)) / float64(period)
		} else {
			avgGain = (avgGain * float64(period-1)) / float64(period)
			avgLoss = (avgLoss*float64(period-1) - diff) / float64(period)
		}
	}
	if avgLoss == 0 {
		return 100
	}
	rs := avgGain / avgLoss
	return 100 - (100 / (1 + rs))
}

func rollingStd(data []float64, period int) float64 {
	n := len(data)
	start := n - period
	if start < 0 {
		start = 0
	}
	slice := data[start:]
	mean := 0.0
	for _, v := range slice {
		mean += v
	}
	mean /= float64(len(slice))
	variance := 0.0
	for _, v := range slice {
		d := v - mean
		variance += d * d
	}
	variance /= float64(len(slice))
	return math.Sqrt(variance)
}

func max20(data []float64) float64 {
	n := len(data)
	start := n - 20
	if start < 0 {
		start = 0
	}
	m := data[start]
	for _, v := range data[start:] {
		if v > m {
			m = v
		}
	}
	return m
}

func min20(data []float64) float64 {
	n := len(data)
	start := n - 20
	if start < 0 {
		start = 0
	}
	m := data[start]
	for _, v := range data[start:] {
		if v < m {
			m = v
		}
	}
	return m
}
