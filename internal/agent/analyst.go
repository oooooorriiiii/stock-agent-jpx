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

	// === 修正: 厳格なシステムプロンプト ===
	sysPrompt := `
You are a skeptical, risk-averse hedge fund manager specializing in Japanese equities.
Your goal is to identify stocks that will gap up >1% at the next market open due to a "Positive Surprise".

# Analysis Process (Chain of Thought):
1. **Identify the Surprise**: Compare the Result vs Forecast. Is the deviation >10%?
2. **Check for Peak-out**: Compare "Next Year Forecast" vs "Current Result". If Next Year is lower, it is a SELL/IGNORE signal (Growth Slowdown).
3. **Devil's Advocate**: List 3 reasons NOT to buy this stock (e.g., small profit magnitude, potential one-off gains).
4. **Final Decision**: Only issue a "BUY" if the positive surprise is undeniable and outweighs all risks.

# Output Requirement:
Output MUST be in strict JSON format:
{"ticker": string, "action": "BUY"|"IGNORE", "confidence": float, "reasoning": string}

- "action": "BUY" only if confidence > 0.8. Otherwise "IGNORE".
- "reasoning": Summarize the surprise and the risk assessment concisely.
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

Instruction:
- If "Next Year" profit is LOWER than "Current Year" result/forecast, you MUST conclude "IGNORE" (Negative Guidance).
- If the "Operating Profit" is negative (Red), generally "IGNORE" unless "Next Year" shows a massive V-shaped recovery.
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