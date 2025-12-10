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
	PromptID   string  `json:"-"` // JSONからは読み込まないが、CSV出力用に構造体に持たせる
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
You are an AI Trader. Goal: Find stocks with Good Earnings, Good Trend, and HIGH VOLATILITY.

# Workflow:
1. **Analyze Earnings**: Check Profit Growth/Guidance.
2. **Check Technicals (Tool)**: CALL "get_price_trend".
3. **Final Decision**:
   - **CRITICAL RULE 1**: If Tool says "Low Liquidity", IGNORE.
   - **CRITICAL RULE 2**: If Tool says "Low Volatility", IGNORE. (We need stocks that move >1.5% daily).
   
   - If Earnings Good + Trend UPTREND/FLAT + Volatility HIGH -> BUY
   - If Earnings Good + Trend DOWNTREND -> IGNORE
   - If Earnings Bad -> IGNORE

Output JSON: {"ticker": string, "action": "BUY"|"IGNORE", "confidence": float, "reasoning": string}
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

	// プロンプト作成
	userPrompt := fmt.Sprintf(`
Analyze Ticker: %s (Date: %s)
Op Profit: %s (Forecast: %s)
Next Year Op Profit: %s
`, data.LocalCode, data.DisclosedDate, data.OperatingProfit, data.ForecastOperatingProfit, data.NextYearForecastOperatingProfit)

	// 実行
	events := s.runner.Run(
		ctx,
		s.userID,
		sess.Session.ID(),
		genai.NewContentFromText(userPrompt, genai.RoleUser),
		agent.RunConfig{StreamingMode: agent.StreamingModeNone},
	)

// 結果の取得とパース
	var lastText string
	for event, err := range events {
		if err != nil {
			return nil, fmt.Errorf("agent run error: %w", err)
		}

		// event.Content が nil でないことを確認してからアクセスする
		if event.Content != nil && len(event.Content.Parts) > 0 {
			if txt := event.Content.Parts[0].Text; txt != "" {
				lastText = txt
			}
		}
	}

	// JSON部分の抽出とパース
	if lastText == "" {
		return nil, fmt.Errorf("agent returned no text response")
	}

	// === 修正箇所: 変数宣言だけでなく、エラーチェックと return を行う ===
	eval, err := parseJSONResponse(lastText)
	if err != nil {
		// JSONパース失敗時もエラーとして返す
		return nil, fmt.Errorf("json parse error: %w (raw: %s)", err, lastText)
	}

	// 銘柄コードなどのメタデータを付与
	eval.Ticker = data.LocalCode
	eval.PromptID = "v5_liquidity_filter" // バージョン管理用

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