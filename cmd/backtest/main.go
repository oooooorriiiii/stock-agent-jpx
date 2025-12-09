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
		if i == 0 { continue } // Header

		dateStr := record[0] // 分析日 (例: 2025-06-25)
		ticker := record[1]
		action := record[2]

		// BUYのみ検証
		if action != "BUY" {
			continue
		}

		// 2. 翌日以降の株価を取得
		// 分析日の「次の日」から「7日後」までのデータを取得し、最初に見つかった取引日を「翌営業日」とする
		baseDate, _ := time.Parse("2006-01-02", dateStr)
		fromDate := baseDate.AddDate(0, 0, 1).Format("2006-01-02")
		toDate := baseDate.AddDate(0, 0, 7).Format("2006-01-02")

		quotes, err := jq.GetDailyQuotes(ticker, fromDate, toDate)
		if err != nil {
			log.Printf("Error fetching quotes for %s: %v", ticker, err)
			continue
		}

		log.Printf("quotes: %v", quotes)

		if len(quotes) == 0 {
			log.Printf("[%s] No price data found after %s (Market holiday?)", ticker, dateStr)
			continue
		}

		// 翌営業日のデータ
		targetDay := quotes[0]
		
		// 3. 勝敗判定 (Day Trade)
		// Entry: Open
		// Target: Open * 1.01
		entryPrice := targetDay.Open
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

		fmt.Printf("[%s] Date:%s -> Trade:%s | Entry:%.0f -> High:%.0f (+%.2f%%) | Result: %s\n", 
			ticker, dateStr, targetDay.Date, entryPrice, maxPrice, maxReturn, resultStr)
	}

	// 結果サマリ
	if tradeCount > 0 {
		winRate := float64(winCount) / float64(tradeCount) * 100
		fmt.Printf("\n=== Backtest Summary ===\n")
		fmt.Printf("Total Trades: %d\n", tradeCount)
		fmt.Printf("Wins:         %d\n", winCount)
		fmt.Printf("Win Rate:     %.1f%%\n", winRate)
	} else {
		fmt.Println("No BUY trades found in csv.")
	}
}