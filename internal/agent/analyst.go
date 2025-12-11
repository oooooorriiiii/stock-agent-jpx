package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"google.golang.org/adk/agent"
	"google.golang.org/adk/agent/llmagent"
	"google.golang.org/adk/model/gemini"
	"google.golang.org/adk/runner"
	"google.golang.org/adk/session"
	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"
	"google.golang.org/genai"

	"github.com/oooooorriiiii/stock-agent-jpx/internal/jquants"
)

// サービスの構造体
type StockAnalyzer struct {
	runner         *runner.Runner
	sessionService session.Service
	userID         string
}

type Evaluation struct {
	Ticker     string  `json:"ticker"`
	Action     string  `json:"action"`
	Confidence float64 `json:"confidence"`
	Reasoning  string  `json:"reasoning"`

	PromptID         string `json:"-"` // JSONからは読み込まないが、CSV出力用に構造体に持たせる
	FinancialSummary string `json:"-"` // 入力した財務データの要約
	TechnicalSummary string `json:"-"` // ツールが返したテクニカル分析結果
}

// 初期化関数 (ここでModelやToolのセットアップを1回だけ行う)
func NewStockAnalyzer(ctx context.Context, apiKey string, jq *jquants.Client) (*StockAnalyzer, error) {
	// 1. Model初期化
	clientConfig := &genai.ClientConfig{APIKey: apiKey}
	model, err := gemini.NewModel(ctx, "gemini-2.5-pro", clientConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create model: %w", err)
	}

	// 2. Tool初期化 (jqクライアントを注入)
	trendToolInstance := &PriceTrendTool{Client: jq}

	trendTool, err := functiontool.New(
		functiontool.Config{
			Name:        "get_price_trend",
			Description: "Get recent stock price trend to filter out downtrends.",
		},
		trendToolInstance.Execute, // メソッドをハンドラとして渡す
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create tool: %w", err)
	}

	// 3. Agent初期化
	sysPrompt := `
You are an expert Alpha Seeker AI.
Your goal is to score stocks based on "Earnings Power" vs "Market Quality".

# Input Data
1. **Financials**: Profit growth, Forecast revisions.
2. **Technicals (Tool)**: Trend, Liquidity (Trading Value), Volatility.

# Evaluation Logic (Trade-off Analysis):
- **Earnings**: Strong guidance updates are the strongest signal.
- **Liquidity**: Higher is better. If < 100M JPY, the earnings surprise must be massive to justify the risk.
- **Volatility**: Higher is better for day trading. If < 1.0%, it's hard to make profit, so only BUY if you expect a huge gap-up that sticks.

# Decision Rule:
- Do NOT blindly reject stocks based on a single threshold. Look at the full picture.
- If Earnings are "Mediocre" AND Volatility is "Low" -> IGNORE.
- If Earnings are "Superb" -> You can tolerate slightly lower liquidity or volatility.

# Output Requirement:
Output strict JSON: {"ticker": string, "action": "BUY"|"IGNORE", "confidence": float, "reasoning": string}
`
	traderAgent, err := llmagent.New(llmagent.Config{
		Name:        "ai_trader",
		Model:       model,
		Instruction: sysPrompt,
		Tools:       []tool.Tool{trendTool},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create agent: %w", err)
	}

	// 4. Runner初期化
	sessService := session.InMemoryService()
	r, err := runner.New(runner.Config{
		AppName:        "stock_analysis_app",
		Agent:          traderAgent,
		SessionService: sessService,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create runner: %w", err)
	}

	return &StockAnalyzer{
		runner:         r,
		sessionService: sessService,
		userID:         "system_analyzer",
	}, nil
}

// 分析実行関数
// 1回の呼び出しごとに新しいセッションを作成・破棄して、前の銘柄の会話履歴を引きずらないようにします
func (s *StockAnalyzer) Analyze(ctx context.Context, data jquants.FinancialStatement) (*Evaluation, error) {
	// セッションIDの生成 (銘柄ごとにユニークにするか、都度生成)
	// ここではシンプルに毎回新規セッションを作成
	sess, err := s.sessionService.Create(ctx, &session.CreateRequest{
		AppName: "stock_analysis_app",
		UserID:  s.userID,
	})
	if err != nil {
		return nil, fmt.Errorf("session create error: %w", err)
	}
	// 関数の最後でセッションを削除（履歴クリアのため）
	defer s.sessionService.Delete(ctx, &session.DeleteRequest{SessionID: sess.Session.ID()})

	// 2. プロンプト作成 & 財務サマリの記録
	finSummary := fmt.Sprintf(
		"OpProfit: %s (Fcst: %s) | NextYear: %s",
		data.OperatingProfit, data.ForecastOperatingProfit, data.NextYearForecastOperatingProfit,
	)

	// プロンプト作成
	userPrompt := fmt.Sprintf(`
Analyze Ticker: %s (Date: %s)
%s
`, data.LocalCode, data.DisclosedDate, finSummary)

	// 実行
	events := s.runner.Run(
		ctx,
		s.userID,
		sess.Session.ID(),
		genai.NewContentFromText(userPrompt, genai.RoleUser),
		agent.RunConfig{StreamingMode: agent.StreamingModeNone},
	)

	// 4. 結果の取得とパース（ツール出力のキャプチャ機能を追加）
	var lastText string
	var toolOutput string // ツールの実行結果を保持

	for event, err := range events {
		if err != nil {
			return nil, fmt.Errorf("agent run error: %w", err)
		}

		if event.Content != nil {
			for _, part := range event.Content.Parts {
				// テキスト（モデルの回答）
				if part.Text != "" {
					lastText = part.Text
				}

				if part.FunctionResponse != nil {
					// 構造体のフィールドに直接アクセス
					if val, ok := part.FunctionResponse.Response["analysis"]; ok {
						toolOutput = fmt.Sprintf("%v", val)
					} else {
						// resultキーがない場合は全体を保存
						toolOutput = fmt.Sprintf("%v", part.FunctionResponse.Response)
					}
				}
			}
		}
	}

	// JSON部分の抽出とパース
	if lastText == "" {
		return nil, fmt.Errorf("agent returned no text response")
	}

	// 5. JSONパース
	eval, err := parseJSONResponse(lastText)
	if err != nil {
		// JSONパース失敗時もエラーとして返す
		return nil, fmt.Errorf("json parse error: %w (raw: %s)", err, lastText)
	}

	// 付帯情報の格納
	eval.Ticker = data.LocalCode
	eval.PromptID = "v5_liquidity_filter"
	eval.FinancialSummary = finSummary
	eval.TechnicalSummary = toolOutput // キャプチャしたツール結果を格納

	return eval, nil
}

func parseJSONResponse(text string) (*Evaluation, error) {
	// マークダウンの ```json ... ``` を除去する簡易処理
	start := strings.Index(text, "{")
	end := strings.LastIndex(text, "}")
	if start == -1 || end == -1 {
		return nil, fmt.Errorf("no json found in response: %s", text)
	}
	jsonStr := text[start : end+1]

	var eval Evaluation
	if err := json.Unmarshal([]byte(jsonStr), &eval); err != nil {
		return nil, fmt.Errorf("json unmarshal error: %w", err)
	}
	return &eval, nil
}
