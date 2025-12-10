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

	// 設定: ギャップ上限（これ以上高く寄り付いたら買わない）
	const MaxGapThreshold = 2.5 // +2.5%

	log.Printf("--- Starting Backtest (Filter: Gap < %.1f%%) ---", MaxGapThreshold)
	
	winCount := 0
	tradeCount := 0
	skippedGapCount := 0
	
	// 重複チェック用マップ (Key: "Date-Ticker")
	processed := make(map[string]bool)

	for i, record := range records {
		if i == 0 { continue }

		// 0:Date, 1:Ticker, 2:CompanyName, 3:Action, ...
		if len(record) < 4 { continue } // 安全策
		
		dateStr := record[0]
		ticker := record[1]
		// companyName := record[2] // 必要なら表示に使用
		action := record[3]

		if action != "BUY" { continue }

		// 重複排除
		key := fmt.Sprintf("%s-%s", dateStr, ticker)
		if processed[key] {
			continue
		}
		processed[key] = true

		analyzeDate, _ := time.Parse("2006-01-02", dateStr)
		fromDate := analyzeDate.Format("2006-01-02")
		toDate := analyzeDate.AddDate(0, 0, 7).Format("2006-01-02")

		quotes, err := jq.GetDailyQuotes(ticker, fromDate, toDate)
		if err != nil {
			log.Printf("API Error %s: %v", ticker, err)
			continue
		}
		if len(quotes) < 2 {
			continue
		}

		// データ検索: PrevClose(分析日) と EntryDay(翌営業日) を特定
		var prevDay, targetDay jquants.DailyQuote
		found := false
		
		// quote[0]が分析日(dateStr)と一致するか確認
		for j := 0; j < len(quotes)-1; j++ {
			if quotes[j].Date == dateStr {
				prevDay = quotes[j]
				targetDay = quotes[j+1]
				found = true
				break
			}
		}
		
		if !found || prevDay.Close <= 0 || targetDay.Open <= 0 { continue }

		// Gap計算
		prevClose := prevDay.Close
		entryPrice := targetDay.Open
		gapPercent := (entryPrice - prevClose) / prevClose * 100

		// === フィルタリング: 高すぎる寄り付きは避ける ===
		if gapPercent > MaxGapThreshold {
			fmt.Printf("⏭️  [%s] Skipped High Gap: +%.2f%%\n", ticker, gapPercent)
			skippedGapCount++
			continue
		}

		// トレード判定 (TP: +1%)
		targetPrice := entryPrice * 1.01 
		maxPrice := targetDay.High
		isWin := maxPrice >= targetPrice
		
		resultStr := "LOSE ❌"
		if isWin {
			resultStr = "WIN 🏆"
			winCount++
		}
		tradeCount++

		maxReturn := (maxPrice - entryPrice) / entryPrice * 100

		fmt.Printf("[%s] Gap:%+6.2f%% | Entry:%5.0f -> High:%5.0f (Max:+%.2f%%) | Result: %s\n", 
			ticker, gapPercent, entryPrice, maxPrice, maxReturn, resultStr)
	}

	if tradeCount > 0 {
		winRate := float64(winCount) / float64(tradeCount) * 100
		fmt.Printf("\n=== Backtest Summary ===\n")
		fmt.Printf("Valid Trades: %d\n", tradeCount)
		fmt.Printf("Wins:         %d\n", winCount)
		fmt.Printf("Win Rate:     %.1f%%\n", winRate)
		fmt.Printf("Skipped Gaps: %d\n", skippedGapCount)
	} else {
		fmt.Println("No valid trades found.")
	}
}