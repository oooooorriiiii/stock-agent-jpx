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

	// 過去14日間のデータを取得（休場日考慮で少し長めに）
	fromDate := baseDate.AddDate(0, 0, -14).Format("2006-01-02")
	toDate := baseDateStr

	// 構造体に保持しているClientを使用
	quotes, err := t.Client.GetDailyQuotes(ticker, fromDate, toDate)
	if err != nil {
		return "", fmt.Errorf("failed to fetch quotes: %v", err)
	}
	if len(quotes) < 5 {
		return "Insufficient price data to determine trend.", nil
	}

	latest := quotes[len(quotes)-1]
	mid := quotes[len(quotes)/2]
	start := quotes[0]

	trend := "FLAT"
	if latest.Close > start.Close*1.05 {
		trend = "UPTREND"
	} else if latest.Close < start.Close*0.95 {
		trend = "DOWNTREND"
	}

	return fmt.Sprintf(
		"Trend: %s\nLatest Close: %.0f (%s)\n5-day ago: %.0f\n10-day ago: %.0f\nWarning: Ensure the stock is NOT in a sharp downtrend before buying.",
		trend, latest.Close, latest.Date, mid.Close, start.Close,
	), nil
}