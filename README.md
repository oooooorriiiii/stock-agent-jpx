# Stock Agent JPX

Google GenAI (Gemini) と J-Quants API を活用した、日本株（JPX）向けの自律型投資エージェントシステムです。
ファンダメンタルズ分析とテクニカル分析を組み合わせ、AIが「買い」銘柄を選定します。

## 🚀 プロジェクト概要

このシステムは、単なるスクリーニングツールではありません。AIエージェントが「プロのトレーダー」として振る舞い、各銘柄の決算データと株価トレンドを分析して、自信を持って推奨できる銘柄のみを抽出します。

主な機能:
- **AIエージェント分析**: 財務データと価格トレンドを統合的に評価。
- **バックテスト**: 過去データを用いた詳細な検証と勝率算出。
- **分析ダッシュボード**: Streamlitによる分析結果の可視化。

## 🧠 投資手法 (Investment Strategy)

本システムは、**"Alpha Seeker AI"** と呼ばれるロジックを採用しており、以下の厳格なルールに基づいて意思決定を行います。

### 1. Alpha Seeker AI ロジック
AIは「稼ぐ力 (Earnings Power)」と「市場の質 (Market Quality)」のバランスを重視します。

*   **流動性は命 (Liquidity is Life)**
    *   **ルール**: 売買代金が **5,000万円未満** の銘柄は「危険」と判断し、**即座に除外 (IGNORE)** します。
    *   5,000万〜1億円の銘柄は、例外的に優れた決算である場合のみ検討します。
*   **ボラティリティは利益の源泉 (Volatility is Profit)**
    *   **ルール**: 日次変動率（Volatility）が **1.0% 未満** の銘柄は、利益機会が少ないため **除外** します。
    *   目標は日次1.5%以上の変動がある銘柄です。
*   **トレンドに従う (Don't Fight the Trend)**
    *   下降トレンド（DOWNTREND）にある銘柄は基本避けます。購入する場合は、トレンドを覆すほどの「ポジティブサプライズ」が必要です。
*   **ファンダメンタルズ**
    *   特に「来期予想営業利益 (Next Year Forecast)」の成長率を重視します。

### 2. バックテスト戦略
AIが推奨した銘柄に対して、以下のルールでトレードシミュレーションを行います。

*   **エントリー**: 分析日の**翌営業日の始値 (Open)** で購入。
*   **ギャップフィルター (Gap Filter)**:
    *   始値が前日終値より **+2.5% 以上** 高い場合（高寄り）は、高値掴みを避けるために**エントリーを見送ります**。
*   **利益確定 (Take Profit)**:
    *   エントリー価格から **+1%** 上昇した時点で勝利（WIN）とみなします（日中の高値で判定）。

## 🛠️ 前提条件 (Prerequisites)

*   **Go**: v1.25 以上
*   **Python**: v3.12 以上 (管理ツールとして `uv` 推奨)
*   **J-Quants API**: Reflesh Token が必要です（Premiumプラン推奨）。
*   **Google GenAI**: Gemini API Key が必要です。

## ⚙️ 設定 (Configuration)

プロジェクトルートに `.env` ファイルを作成し、以下の変数を設定してください。

```bash
GOOGLE_API_KEY="your_google_api_key"
JQUANTS_REFRESH_TOKEN="your_jquants_refresh_token"
```

## 💻 使用方法 (Usage)

### 1. エージェントによる分析実行
指定した期間の全上場企業（財務データ開示企業）を分析し、結果を `results.csv` に出力します。

```bash
go run cmd/app/main.go
```

### 2. バックテストの実行
`results.csv` に記録されたAIの推奨銘柄（BUY）に基づいて、勝率を検証します。

```bash
go run cmd/backtest/main.go
```
出力例:
```text
[72030] Gap: +0.50% | Entry: 2000 -> High: 2030 (Max:+1.50%) | Result: WIN 🏆
...
=== Backtest Summary ===
Valid Trades: 15
Wins:         12
Win Rate:     80.0%
Skipped Gaps: 3
```

### 3. ダッシュボードの起動
分析結果を視覚的に確認できます。AIの判断理由や、ボラティリティと自信度（Confidence）の関係などをグラフ化します。

```bash
cd analysis
uv run streamlit run app.py
```
ブラウザが起動し、`http://localhost:8501` でダッシュボードにアクセスできます。

## 📂 ディレクトリ構成

*   `cmd/app`: エージェント本体のソースコード
*   `cmd/backtest`: バックテストツールのソースコード
*   `analysis`: Python/Streamlit ダッシュボード
*   `internal`: アプリケーションの内部ロジック
    *   `agent`: Gemini API との対話、プロンプト定義
    *   `jquants`: J-Quants API クライアント
