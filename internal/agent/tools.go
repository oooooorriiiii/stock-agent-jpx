package agent

import (
	"fmt"
	"time"

	"github.com/oooooorriiiii/stock-agent-jpx/internal/jquants"
	"google.golang.org/adk/tool"
)

// -------------------------------------------------------
// 1. Toolの引数と戻り値の定義 (ADK用タグ付き)
// -------------------------------------------------------
type PriceTrendArgs struct {
	Ticker   string `json:"ticker" jsonschema:"The stock ticker symbol (e.g., '72030')."`
	BaseDate string `json:"base_date" jsonschema:"The reference date for analysis (YYYY-MM-DD)."`
}

type PriceTrendResult struct {
	Analysis string `json:"analysis"` // User定義のGetPriceTrendが返す文字列を格納
}

// -------------------------------------------------------
// 2. Toolの実体 (依存関係を持つ構造体)
// -------------------------------------------------------
type PriceTrendTool struct {
	Client *jquants.Client
}

// ADKから呼ばれるハンドラメソッド
func (t *PriceTrendTool) Execute(ctx tool.Context, args PriceTrendArgs) (PriceTrendResult, error) {
	// 既存のロジックを呼び出す
	resultStr, err := t.getPriceTrendLogic(args.Ticker, args.BaseDate)
	if err != nil {
		return PriceTrendResult{}, err
	}
	return PriceTrendResult{Analysis: resultStr}, nil
}

func (t *PriceTrendTool) getPriceTrendLogic(ticker string, baseDateStr string) (string, error) {
	baseDate, err := time.Parse("2006-01-02", baseDateStr)
	if err != nil {
		return "", fmt.Errorf("invalid date format")
	}

	fromDate := baseDate.AddDate(0, 0, -20).Format("2006-01-02")
	toDate := baseDateStr

	quotes, err := t.Client.GetDailyQuotes(ticker, fromDate, toDate)
	if err != nil {
		return "", fmt.Errorf("failed to fetch quotes: %v", err)
	}
	if len(quotes) < 5 {
		return "Insufficient data (less than 5 days).", nil
	}

	latest := quotes[len(quotes)-1]
	start := quotes[0]

	// === 生データの計算のみを行う ===
	var totalValue float64
	var totalVolatility float64
	count := 0

	for i := len(quotes) - 1; i >= len(quotes)-5 && i >= 0; i-- {
		q := quotes[i]
		totalValue += q.Close * q.Volume

		basePrice := q.Open
		if basePrice == 0 {
			basePrice = q.Close
		}
		if basePrice > 0 {
			dayRange := (q.High - q.Low) / basePrice * 100
			totalVolatility += dayRange
		}
		count++
	}

	avgValue := totalValue / float64(count)           // 平均売買代金
	avgVolatility := totalVolatility / float64(count) // 平均変動率 (%)

	// トレンド判定
	trend := "FLAT"
	changeRate := (latest.Close - start.Close) / start.Close * 100
	if changeRate > 5.0 {
		trend = "UPTREND"
	} else if changeRate < -5.0 {
		trend = "DOWNTREND"
	}

	// === 判定なし。事実のみを返す ===
	return fmt.Sprintf(
		"Trend: %s (Change: %.2f%% in 20days)\nAvg Trading Value: %.0f JPY\nAvg Daily Volatility: %.2f%%\nLatest Close: %.0f",
		trend, changeRate, avgValue, avgVolatility, latest.Close,
	), nil
}
