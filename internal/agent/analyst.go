package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"google.golang.org/genai"
	"github.com/oooooorriiiii/stock-agent-jpx/internal/jquants"
)

type Evaluation struct {
	Ticker     string  `json:"ticker"`
	Action     string  `json:"action"`
	Confidence float64 `json:"confidence"`
	Reasoning  string  `json:"reasoning"`
}

func Analyze(ctx context.Context, apiKey string, data jquants.FinancialStatement) (*Evaluation, error) {
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey: apiKey,
	})
	if err != nil {
		return nil, fmt.Errorf("genai client init error: %w", err)
	}

	modelName := "gemini-2.5-pro" 

	// === バランス型（モメンタム重視）のプロンプト ===
	sysPrompt := `
You are a Momentum Trader looking for "Earnings Surprises".
Your goal is to identify stocks that will jump >1% tomorrow based on the disclosed financial data.

# Evaluation Criteria:
1. **Positive Surprise**: Does the "Result" exceed the "Current Year Forecast"? (Even a +3% beat is positive).
2. **Guidance Check**:
   - If "Next Year Forecast" shows growth -> STRONG BUY signal.
   - If "Next Year Forecast" is flat or slightly down, BUT the current beat is huge -> BUY signal (Market often reacts to the immediate surprise).
   - If "Next Year Forecast" is a disastrous drop -> IGNORE.
3. **Turnaround**: If the company moved from Loss (Red) to Profit (Black), this is a powerful BUY signal.

# Output Requirement:
Output MUST be in strict JSON format:
{"ticker": string, "action": "BUY"|"IGNORE", "confidence": float, "reasoning": string}

- "action": "BUY" if you see positive momentum.
- "confidence": 0.0 - 1.0. (Threshold for BUY is > 0.7)
`

	// ユーザープロンプト（来期予想比較を強調）
	userPrompt := fmt.Sprintf(`
Analyze Ticker: %s
Disclosed Date: %s

[Result (Current Period)]
Operating Profit: %s JPY

[Forecast (Current Year)]
Sales: %s JPY
Op Profit: %s JPY

[Forecast (Next Year)]
Sales: %s JPY
Op Profit: %s JPY
`, 
		data.LocalCode, data.DisclosedDate, 
		data.OperatingProfit,
		data.ForecastNetSales, data.ForecastOperatingProfit,
		data.NextYearForecastNetSales, data.NextYearForecastOperatingProfit,
	)

	resp, err := client.Models.GenerateContent(ctx, modelName, 
		genai.Text(userPrompt), 
		&genai.GenerateContentConfig{
			SystemInstruction: &genai.Content{
				Parts: []*genai.Part{{Text: sysPrompt}},
			},
			ResponseMIMEType: "application/json", 
		},
	)
	if err != nil {
		return nil, fmt.Errorf("generate content error: %w", err)
	}

	if len(resp.Candidates) == 0 || resp.Candidates[0].Content == nil {
		return nil, fmt.Errorf("empty response from model")
	}

	var responseText string
	for _, part := range resp.Candidates[0].Content.Parts {
		if part != nil {
			responseText += part.Text
		}
	}

	// JSONパース処理
	cleanJSON := strings.TrimSpace(responseText)
	cleanJSON = strings.TrimPrefix(cleanJSON, "```json")
	cleanJSON = strings.TrimPrefix(cleanJSON, "```")
	cleanJSON = strings.TrimSuffix(cleanJSON, "```")
	if idx := strings.Index(cleanJSON, "{"); idx != -1 {
		cleanJSON = cleanJSON[idx:]
	}

	var eval Evaluation
	if err := json.Unmarshal([]byte(cleanJSON), &eval); err != nil {
		fmt.Fprintf(os.Stderr, "JSON Parse Error. Raw: %s\n", responseText)
		return nil, fmt.Errorf("json parse error: %w", err)
	}
	eval.Ticker = data.LocalCode

	return &eval, nil
}