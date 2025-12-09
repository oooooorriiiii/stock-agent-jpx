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

	// === 期間指定の設定 ===
	startDateStr := "2025-06-20"
	endDateStr := "2025-06-30" 
	// ===================

	// CSV準備（PromptID列を追加）
	file, _ := os.OpenFile("results.csv", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	defer file.Close()
	writer := csv.NewWriter(file)
	defer writer.Flush()

	stat, _ := file.Stat()
	if stat.Size() == 0 {
		writer.Write([]string{"Date", "Ticker", "Action", "Confidence", "Reasoning", "PromptID"})
	}

	ctx := context.Background()

	// 1. J-Quants Clientの初期化
	jq := jquants.NewClient(cfg.JQuantsRefreshToken)

	// 2. Analyzer (Agent) の初期化 【ここを追加】
	// ループの外で一度だけ作成することで、モデル定義やTool設定のオーバーヘッドを削減します
	analyzer, err := agent.NewStockAnalyzer(ctx, cfg.GoogleAPIKey, jq)
	if err != nil {
		log.Fatalf("Failed to initialize StockAnalyzer: %v", err)
	}

	// 日付ループ
	start, _ := time.Parse("2006-01-02", startDateStr)
	end, _ := time.Parse("2006-01-02", endDateStr)

	for d := start; !d.After(end); d = d.AddDate(0, 0, 1) {
		targetDate := d.Format("2006-01-02")
		log.Printf("--- Processing Date: %s ---", targetDate)

		statements, err := jq.GetStatements(targetDate)
		if err != nil {
			log.Printf("Failed to fetch data for %s: %v", targetDate, err)
			continue
		}

		if len(statements) == 0 {
			log.Printf("No statements found for %s (Holiday or no disclosure). Skipping.", targetDate)
			continue
		}

		log.Printf("Found %d statements.", len(statements))

		for _, s := range statements {
			if s.OperatingProfit == "" {
				continue
			}

			// レートリミット (Tier 1)
			time.Sleep(5 * time.Second)

			// 3. Analyzeの実行 【ここを変更】
			// インスタンスメソッドとして呼び出します。jqなどは初期化時に渡済みなので引数が減ります。
			eval, err := analyzer.Analyze(ctx, s)
			if err != nil {
				log.Printf("Error [%s]: %v", s.LocalCode, err)
				continue
			}

			if eval.Action == "BUY" {
				log.Printf("🚀 [%s] BUY (Conf: %.2f): %s", eval.Ticker, eval.Confidence, eval.Reasoning)
			} else {
				log.Printf("💤 [%s] IGNORE: %s", eval.Ticker, eval.Reasoning)
			}

			// CSV書き込み
			writer.Write([]string{
				targetDate,
				eval.Ticker,
				eval.Action,
				fmt.Sprintf("%.2f", eval.Confidence),
				eval.Reasoning,
				eval.PromptID, // SessionIDなどが入る想定
			})
			writer.Flush()
		}
	}
	log.Println("Batch Analysis Completed.")
}