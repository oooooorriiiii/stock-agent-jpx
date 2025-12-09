package main

import (
	"context"
	"encoding/csv"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/oooooorriiiii/stock-agent-jpx/internal/agent"
	"github.com/oooooorriiiii/stock-agent-jpx/internal/config"
	"github.com/oooooorriiiii/stock-agent-jpx/internal/jquants"
)

func main() {
	cfg := config.Load()
	
	// === CSVファイルの準備 ===
	file, err := os.OpenFile("results.csv", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Fatalf("Cannot create csv file: %v", err)
	}
	defer file.Close()
	
	writer := csv.NewWriter(file)
	defer writer.Flush()

	// ファイルが空ならヘッダーを書き込む
	stat, _ := file.Stat()
	if stat.Size() == 0 {
		writer.Write([]string{"Date", "Ticker", "Action", "Confidence", "Reasoning"})
	}
	// ======================

	jq := jquants.NewClient(cfg.JQuantsRefreshToken)
	
	// 検証したい日付（過去日付でテストする場合はここを変える）
	targetDate := "2025-07-02" 
	log.Printf("Target Date: %s", targetDate)
	
	statements, err := jq.GetStatements(targetDate)
	if err != nil {
		log.Fatalf("J-Quants API Error: %v", err)
	}

	log.Printf("Fetched %d statements.", len(statements))

	ctx := context.Background()
	log.Println("Starting analysis...")

	for i, s := range statements {
		// フィルタリング
		if s.OperatingProfit == "" {
			continue
		}

		// レートリミット対策
		if i > 0 {
			log.Println("Sleeping 5s...")
			time.Sleep(5 * time.Second)
		}

		// 分析実行
		eval, err := agent.Analyze(ctx, cfg.GoogleAPIKey, s)
		if err != nil {
			log.Printf("Error [%s]: %v", s.LocalCode, err)
			continue
		}

		// コンソール出力
		log.Printf("[%s] %s (%.2f): %s", eval.Ticker, eval.Action, eval.Confidence, eval.Reasoning)

		// === CSVへの書き込み ===
		record := []string{
			targetDate,
			eval.Ticker,
			eval.Action,
			fmt.Sprintf("%.2f", eval.Confidence),
			eval.Reasoning,
		}
		if err := writer.Write(record); err != nil {
			log.Printf("CSV Write Error: %v", err)
		}
		writer.Flush() // 都度書き込み（途中で落ちても大丈夫なように）
	}

	log.Println("Done. Results saved to results.csv")
}