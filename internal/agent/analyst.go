package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"google.golang.org/genai"
)

// jquantsパッケージの定義に合わせて読み替えてください
import "github.com/oooooorriiiii/stock-agent-jpx/internal/jquants"

type Evaluation struct {
	Ticker     string  `json:"ticker"`
	Action     string  `json:"action"`
	Confidence float64 `json:"confidence"`
	Reasoning  string  `json:"reasoning"`
}

func Analyze(ctx context.Context, apiKey string, data jquants.FinancialStatement) (*Evaluation, error) {
	// 1. GenAI クライアントの初期化
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey: apiKey,
	})
	if err != nil {
		return nil, fmt.Errorf("genai client init error: %w", err)
	}

	// 2. モデルとコンフィグ設定
	modelName := "gemini-2.5-pro" 

	sysPrompt := `
You are an algorithmic trading AI specializing in Japanese stocks.
Evaluate the provided financial data for immediate stock price impact (next day).
Output MUST be in strict JSON format corresponding to the schema:
{"ticker": string, "action": "BUY"|"IGNORE", "confidence": float, "reasoning": string}
`

	// 3. ユーザープロンプト作成
	userPrompt := fmt.Sprintf(`
Analyze Ticker: %s
Disclosed Date: %s

[Result]
Operating Profit: %s JPY

[Forecast (Current Year)]
Sales: %s JPY
Op Profit: %s JPY

[Forecast (Next Year)]
Sales: %s JPY
Op Profit: %s JPY

Instruction: 
Focus on the "Next Year" forecast if "Current Year" is empty (indicates FY results).
Compare Result vs Forecasts to judge the momentum.
`, 
		data.LocalCode, data.DisclosedDate, 
		data.OperatingProfit,
		data.ForecastNetSales, data.ForecastOperatingProfit,
		data.NextYearForecastNetSales, data.NextYearForecastOperatingProfit,
	)

	// 4. 推論実行
	// エラー修正: 構造体の型を厳密に合わせます
	resp, err := client.Models.GenerateContent(ctx, modelName, 
		// User Prompt (Contents)
		// GenAI SDKの仕様に合わせて []*genai.Content または単一の Content を渡す必要がある場合がありますが、
		// 多くのバージョンで可変長引数あるいは Content 構造体を受け取ります。
		// ここでは genai.Text() が期待通り動かない可能性があるため、Partを明示的に作ります。
		genai.Text(userPrompt), 
		
		// Config
		&genai.GenerateContentConfig{
			SystemInstruction: &genai.Content{
				// Error Fix: Parts は []*genai.Part (ポインタのスライス)
				Parts: []*genai.Part{
					{Text: sysPrompt}, // Error Fix: 構造体リテラルでTextを指定
				},
			},
			// Error Fix: MIMETypeは大文字
			ResponseMIMEType: "application/json", 
		},
	)
	if err != nil {
		return nil, fmt.Errorf("generate content error: %w", err)
	}

	// 5. 結果の取得
	if len(resp.Candidates) == 0 || resp.Candidates[0].Content == nil {
		return nil, fmt.Errorf("empty response from model")
	}

	var responseText string
	// Error Fix: Partはインターフェースではなく構造体ポインタとして扱います
	for _, part := range resp.Candidates[0].Content.Parts {
		// part が nil でないことを確認し、Textフィールドを結合
		if part != nil {
			responseText += part.Text
		}
	}

	// 6. JSONパース
	cleanJSON := strings.TrimSpace(responseText)
	cleanJSON = strings.TrimPrefix(cleanJSON, "```json")
	cleanJSON = strings.TrimPrefix(cleanJSON, "```")
	cleanJSON = strings.TrimSuffix(cleanJSON, "```")

	var eval Evaluation
	if err := json.Unmarshal([]byte(cleanJSON), &eval); err != nil {
		fmt.Fprintf(os.Stderr, "JSON Parse Error. Raw: %s\n", responseText)
		return nil, fmt.Errorf("json parse error: %w", err)
	}
	eval.Ticker = data.LocalCode

	return &eval, nil
}