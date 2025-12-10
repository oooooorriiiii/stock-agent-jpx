package agent

import (
	"fmt"
	"time"

	"google.golang.org/adk/tool"
	"github.com/oooooorriiiii/stock-agent-jpx/internal/jquants"
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

// ユーザー定義のロジック (元のGetPriceTrendをメソッド化)
func (t *PriceTrendTool) getPriceTrendLogic(ticker string, baseDateStr string) (string, error) {
	baseDate, err := time.Parse("2006-01-02", baseDateStr)
	if err != nil {
		return "", fmt.Errorf("invalid date format")
	}

	fromDate := baseDate.AddDate(0, 0, -14).Format("2006-01-02")
	toDate := baseDateStr

	quotes, err := t.Client.GetDailyQuotes(ticker, fromDate, toDate)
	if err != nil {
		return "", fmt.Errorf("failed to fetch quotes: %v", err)
	}
	if len(quotes) < 5 {
		return "Insufficient data.", nil
	}

	latest := quotes[len(quotes)-1]
	mid := quotes[len(quotes)/2]
	start := quotes[0]

	// === 売買代金の計算 ===
	// 売買代金 = 終値 * 出来高
	// 直近5日間の平均売買代金を計算
	var totalValue float64
	count := 0
	for i := len(quotes) - 1; i >= 0 && count < 5; i-- {
		totalValue += quotes[i].Close * quotes[i].Volume
		count++
	}
	avgValue := totalValue / float64(count)

	// 閾値: 3億円 (300,000,000 JPY)
	// これ以下は「過疎銘柄」として警告
	liquidityStatus := "HIGH"
	liquidityWarning := ""
	if avgValue < 300_000_000 {
		liquidityStatus = "LOW"
		liquidityWarning = fmt.Sprintf("\nCRITICAL WARNING: Low Liquidity (Avg Value: %.0f JPY). Stock may not move. IGNORE recommended.", avgValue)
	}

	trend := "FLAT"
	if latest.Close > start.Close*1.05 {
		trend = "UPTREND"
	} else if latest.Close < start.Close*0.95 {
		trend = "DOWNTREND"
	}

	return fmt.Sprintf(
		"Trend: %s\nLiquidity: %s%s\nLatest Close: %.0f\n5-day ago: %.0f",
		trend, liquidityStatus, liquidityWarning, latest.Close, mid.Close,
	), nil
}