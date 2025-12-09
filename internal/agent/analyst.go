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

	// === ガイダンス重視（Guidance Growth）プロンプト ===
	sysPrompt := `
You are a "Growth Hunter" AI trader.
Your sole objective is to identify stocks where the **Future Guidance (Forecast)** has been revised upward, causing institutional buying pressure throughout the day.

# Critical Rule: "Don't Buy the Peak"
- If a company reports good current results but the "Next Year Forecast" is weak or flat, the stock will likely "Gap Up and Sell Off". **IGNORE** these.
- Only BUY if the **Future Forecast** implies accelerating growth.

# Evaluation Criteria (Priority Order):
1. **Guidance Growth**: Is "Next Year Forecast" significantly higher than "Current Result"? (e.g., +10% growth).
2. **Profit Margin**: Is the Operating Profit Margin improving? (Profit/Sales ratio).
3. **Surprise Factor**: Did they beat the current forecast by a wide margin?

# Output Requirement:
Output MUST be in strict JSON format:
{"ticker": string, "action": "BUY"|"IGNORE", "confidence": float, "reasoning": string}

- "action": "BUY" only if you predict sustained buying after the open.
- "confidence": > 0.8 for BUY.
`

	// ユーザープロンプト（計算補助を追加）
	userPrompt := fmt.Sprintf(`
Analyze Ticker: %s
Disclosed Date: %s

[Result (Recent)]
Sales: - (Not provided in summary)
Op Profit: %s JPY

[Forecast (Current FY)]
Sales: %s JPY
Op Profit: %s JPY

[Forecast (Next FY)]
Sales: %s JPY
Op Profit: %s JPY

Instruction:
Compare [Forecast (Next FY)] vs [Result (Recent)] or [Forecast (Current FY)].
If the Next FY Op Profit is NOT higher than the Current FY Op Profit, output IGNORE immediately.
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