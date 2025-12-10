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

// ユーザー定義のロジック
func (t *PriceTrendTool) getPriceTrendLogic(ticker string, baseDateStr string) (string, error) {
	baseDate, err := time.Parse("2006-01-02", baseDateStr)
	if err != nil {
		return "", fmt.Errorf("invalid date format")
	}

	fromDate := baseDate.AddDate(0, 0, -20).Format("2006-01-02") // 期間を少し長めに確保
	toDate := baseDateStr

	quotes, err := t.Client.GetDailyQuotes(ticker, fromDate, toDate)
	if err != nil {
		return "", fmt.Errorf("failed to fetch quotes: %v", err)
	}
	if len(quotes) < 5 {
		return "Insufficient data.", nil
	}

	latest := quotes[len(quotes)-1]
	start := quotes[0]

	// === 1. 売買代金チェック (流動性) ===
	var totalValue float64
	var totalVolatility float64
	count := 0
	
	// 直近5日間の平均を計算
	for i := len(quotes) - 1; i >= len(quotes)-5 && i >= 0; i-- {
		q := quotes[i]
		
		// 売買代金
		totalValue += q.Close * q.Volume
		
		// 日中変動率 (High - Low) / Open
		// Openが0の場合はCloseを使うなどの安全策
		basePrice := q.Open
		if basePrice == 0 { basePrice = q.Close }
		if basePrice > 0 {
			dayRange := (q.High - q.Low) / basePrice * 100
			totalVolatility += dayRange
		}
		
		count++
	}
	
	avgValue := totalValue / float64(count)
	avgVolatility := totalVolatility / float64(count) // 平均変動率 (%)

	// === 判定ロジック ===
	liquidityStatus := "HIGH"
	warningMsg := ""

	// 条件1: 売買代金 3億円未満 -> NG
	if avgValue < 300_000_000 {
		liquidityStatus = "LOW"
		warningMsg += fmt.Sprintf("\n[WARNING] Low Liquidity (Avg Value: %.0f JPY). Hard to trade.", avgValue)
	}

	// 条件2: ボラティリティ 1.5%未満 -> NG (1%抜くのが難しい)
	volatilityStatus := "HIGH"
	if avgVolatility < 1.5 {
		volatilityStatus = "LOW"
		warningMsg += fmt.Sprintf("\n[WARNING] Low Volatility (Avg Range: %.2f%%). Stock moves too slow to hit +1%% target.", avgVolatility)
	}

	trend := "FLAT"
	if latest.Close > start.Close*1.05 {
		trend = "UPTREND"
	} else if latest.Close < start.Close*0.95 {
		trend = "DOWNTREND"
	}

	return fmt.Sprintf(
		"Trend: %s\nLiquidity: %s\nVolatility: %.2f%% (%s)%s\nLatest Close: %.0f",
		trend, liquidityStatus, avgVolatility, volatilityStatus, warningMsg, latest.Close,
	), nil
}