package main

import (
	"context"
	"encoding/csv"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/oooooorriiiii/stock-agent-jpx/internal/agent"
	"github.com/oooooorriiiii/stock-agent-jpx/internal/config"
	"github.com/oooooorriiiii/stock-agent-jpx/internal/jquants"
)

const MaxRetries = 5

func main() {
	cfg := config.Load()

	// 検証期間
	startDateStr := "2025-07-01"
	endDateStr := "2025-07-22"

	// CSV準備（CompanyNameを追加）
	file, _ := os.OpenFile("results.csv", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	defer file.Close()
	writer := csv.NewWriter(file)
	defer writer.Flush()

	stat, _ := file.Stat()
	if stat.Size() == 0 {
		// ヘッダーに CompanyName を追加
		writer.Write([]string{
			"Date", "Ticker", "CompanyName", "Action", "Confidence", "Reasoning",
			"Financials", "Technicals", "PromptID",
		})
	}

	jq := jquants.NewClient(cfg.JQuantsRefreshToken)
	ctx := context.Background()

	log.Println("Loading listed company info...")
	nameMap, err := jq.GetListedInfoMap()
	if err != nil {
		log.Printf("Warning: Failed to load company names: %v", err)
		nameMap = make(map[string]string)
	}
	log.Printf("Loaded %d companies.", len(nameMap))

	analyzer, err := agent.NewStockAnalyzer(ctx, cfg.GoogleAPIKey, jq)
	if err != nil {
		log.Fatalf("Failed to init analyzer: %v", err)
	}

	start, _ := time.Parse("2006-01-02", startDateStr)
	end, _ := time.Parse("2006-01-02", endDateStr)

	for d := start; !d.After(end); d = d.AddDate(0, 0, 1) {
		targetDate := d.Format("2006-01-02")
		log.Printf("\n========== Processing Date: %s ==========", targetDate)

		statements, err := jq.GetStatements(targetDate)
		if err != nil {
			log.Printf("Failed to fetch data: %v", err)
			continue
		}
		if len(statements) == 0 {
			log.Printf("No statements found. Skipping.")
			continue
		}

		log.Printf("Found %d statements. Starting analysis...\n", len(statements))

		for i, s := range statements {
			if s.OperatingProfit == "" {
				continue
			}

			companyName := nameMap[s.LocalCode]
			if companyName == "" {
				companyName = "Unknown"
			}

			fmt.Printf("--------------------------------------------------\n")
			fmt.Printf("🔍 [%d/%d] Analyzing %s (%s)\n", i+1, len(statements), s.LocalCode, companyName)

			// time.Sleep(5 * time.Second)

			var eval *agent.Evaluation
			var err error

			for attempt := 1; attempt <= MaxRetries; attempt++ {
				eval, err = analyzer.Analyze(ctx, s)

				if err == nil {
					break
				}

				// 失敗: エラーの内容に応じてログを出力
				log.Printf("❌ Attempt %d failed for %s. Error: %v", attempt, s.LocalCode, err)

				if attempt < MaxRetries {
					// リトライ前に短い時間待つ (指数バックオフのイメージ)
					// API overload対策
					waitTime := time.Duration(attempt) * 2 * time.Second // 2秒, 4秒, ...
					log.Printf("   -> Retrying in %v...", waitTime)
					time.Sleep(waitTime)
				}
			}

			if err != nil {
				// 最大リトライ回数を超えても失敗した場合
				log.Printf("❌ FAILED to analyze %s after %d attempts.", s.LocalCode, MaxRetries)
				continue
			}

			fmt.Printf("   📊 Financials: %s\n", eval.FinancialSummary)
			if eval.TechnicalSummary != "" {
				fmt.Printf("   📈 Technicals:\n      %s\n", eval.TechnicalSummary)
			} else {
				fmt.Printf("   📈 Technicals: (Not checked)\n")
			}

			icon := "💤"
			if eval.Action == "BUY" {
				icon = "🚀"
			}
			fmt.Printf("   🤖 Decision: %s %s (Conf: %.2f)\n", icon, eval.Action, eval.Confidence)
			fmt.Printf("      Reason: %s\n", eval.Reasoning)

			// === CSV書き込みデータの整形 ===
			// 改行を " | " に置換して1行にする
			cleanTech := strings.ReplaceAll(eval.TechnicalSummary, "\n", " | ")

			writer.Write([]string{
				targetDate,
				eval.Ticker,
				companyName, // 追加
				eval.Action,
				fmt.Sprintf("%.2f", eval.Confidence),
				eval.Reasoning,
				eval.FinancialSummary,
				cleanTech, // 整形済みデータ
				eval.PromptID,
			})
			writer.Flush()
		}
	}
	log.Println("\n========== Batch Analysis Completed ==========")
}
