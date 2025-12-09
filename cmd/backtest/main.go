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

		baseDate, err := time.Parse("2006-01-02", dateStr)
		if err != nil {
			log.Printf("Date parse error: %v", err)
			continue
		}
		
		// 翌日から1週間分を検索範囲とする
		fromDate := baseDate.AddDate(0, 0, 1).Format("2006-01-02")
		toDate := baseDate.AddDate(0, 0, 7).Format("2006-01-02")

		quotes, err := jq.GetDailyQuotes(ticker, fromDate, toDate)
		if err != nil {
			log.Printf("API Error fetching quotes for %s: %v", ticker, err)
			continue
		}

		// === デバッグ用: データが空の場合はURLのパラメータが正しいか疑う ===
		if len(quotes) == 0 {
			log.Printf("⚠️ [%s] No quotes found between %s and %s.", ticker, fromDate, toDate)
			log.Printf("   Debug Info: Analyzed Date=%s. Maybe Ticker code change or delisted?", dateStr)
			continue
		}

		// 翌営業日のデータ
		targetDay := quotes[0]
		
		// 3. 勝敗判定 (Day Trade)
		// Entry: Open
		// Target: Open * 1.01
		entryPrice := targetDay.Open
		// 目標は +1%
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

		fmt.Printf("[%s] Analyzed:%s -> Trade:%s | Entry:%.0f -> High:%.0f (+%.2f%%) | Result: %s\n", 
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
		fmt.Println("No BUY trades found in csv to backtest.")
	}
}