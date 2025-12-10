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

	log.Println("--- Analyzing Missed Opportunities (IGNORE -> Skyrocket) ---")

	for i, record := range records {
		if i == 0 { continue }
		dateStr := record[0]
		ticker := record[1]
		action := record[2]
		reason := record[4] // Reasoning

		// IGNORE と判断したものだけをチェック
		if action != "IGNORE" {
			continue
		}

		analyzeDate, _ := time.Parse("2006-01-02", dateStr)
		fromDate := analyzeDate.Format("2006-01-02")
		toDate := analyzeDate.AddDate(0, 0, 7).Format("2006-01-02")

		quotes, err := jq.GetDailyQuotes(ticker, fromDate, toDate)
		if err != nil { continue }
		if len(quotes) < 2 { continue }

		// prevDay := quotes[0]
		targetDay := quotes[1]

		// 翌日大きく上昇したか？ (例: +3%以上)
		openPrice := targetDay.Open
		highPrice := targetDay.High
		
		if openPrice == 0 { continue }
		
		maxReturn := (highPrice - openPrice) / openPrice * 100

		// AIは見送ったが、実は3%以上取れた銘柄を表示
		if maxReturn > 3.0 {
			fmt.Printf("🔥 [MISSED] %s (Date: %s)\n", ticker, dateStr)
			fmt.Printf("   Potential Gain: +%.2f%%\n", maxReturn)
			fmt.Printf("   AI Reason: %s\n", reason)
			fmt.Println("   ------------------------------------------------")
		}
	}
}