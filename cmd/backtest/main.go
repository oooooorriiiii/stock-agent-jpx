package main

import (
	"encoding/csv"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/oooooorriiiii/stock-agent-jpx/internal/config"
	"github.com/oooooorriiiii/stock-agent-jpx/internal/jquants"
)

func main() {
	cfg := config.Load()
	jq := jquants.NewClient(cfg.JQuantsRefreshToken)

	// 1. CSVの読み込み
	file, err := os.Open("results.csv")
	if err != nil {
		log.Fatalf("Failed to open results.csv: %v", err)
	}
	defer file.Close()

	reader := csv.NewReader(file)
	records, err := reader.ReadAll()
	if err != nil {
		log.Fatal(err)
	}

	log.Println("--- Starting Backtest ---")
	
	winCount := 0
	tradeCount := 0

	// ヘッダーをスキップしてループ
	for i, record := range records {
		if i == 0 { continue }

		dateStr := record[0]
		ticker := record[1]
		action := record[2]

		// BUYのみ検証
		if action != "BUY" {
			continue
		}

		// 分析日（＝取引の前日）
		analyzeDate, _ := time.Parse("2006-01-02", dateStr)
		
		// 分析日〜1週間後までのデータを取得（前日終値を知るため分析日も含める）
		fromDate := analyzeDate.Format("2006-01-02")
		toDate := analyzeDate.AddDate(0, 0, 7).Format("2006-01-02")

		quotes, err := jq.GetDailyQuotes(ticker, fromDate, toDate)
		if err != nil {
			log.Printf("API Error %s: %v", ticker, err)
			continue
		}
		if len(quotes) < 2 {
			log.Printf("⚠️ [%s] Not enough quotes (Need at least 2 days: PrevClose & TradeDay)", ticker)
			continue
		}

		// quotes[0] が分析日(前日)、quotes[1] が取引日(当日) と想定
		// ※日付が飛んでいる場合もあるので簡易的にチェック
		prevDay := quotes[0]
		targetDay := quotes[1]

		// 取引日が分析日の「翌営業日」であることを確認（簡易チェック）
		if targetDay.Date <= dateStr {
			// 順番が逆、あるいはデータ欠損の場合の安全策
			if len(quotes) > 2 { targetDay = quotes[2] }
		}

		// Gap判定
		prevClose := prevDay.Close
		entryPrice := targetDay.Open
		gapPercent := (entryPrice - prevClose) / prevClose * 100

		// トレード判定
		// 目標: +1.0% (デイトレ)
		// 緩和策: +0.8%以上で微益撤退成功とみなすならここを 1.008 にする
		targetPrice := entryPrice * 1.01 
		maxPrice := targetDay.High

		isWin := maxPrice >= targetPrice
		
		resultStr := "LOSE ❌"
		if isWin {
			resultStr = "WIN 🏆"
			winCount++
		}
		tradeCount++

		// 最大上昇率
		maxReturn := (maxPrice - entryPrice) / entryPrice * 100

		fmt.Printf("[%s] Gap: %+.2f%% | Entry:%.0f -> High:%.0f (Max: +%.2f%%) | Result: %s\n", 
			ticker, gapPercent, entryPrice, maxPrice, maxReturn, resultStr)
	}

	// 結果サマリ
	if tradeCount > 0 {
		winRate := float64(winCount) / float64(tradeCount) * 100
		fmt.Printf("\n=== Backtest Summary ===\n")
		fmt.Printf("Total Trades: %d\n", tradeCount)
		fmt.Printf("Wins:         %d\n", winCount)
		fmt.Printf("Win Rate:     %.1f%%\n", winRate)
	} else {
		fmt.Println("No BUY trades found.")
	}
}