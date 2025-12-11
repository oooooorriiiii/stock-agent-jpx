import streamlit as st
import pandas as pd
import plotly.express as px

st.set_page_config(layout="wide")
st.title("📈 Stock Agent Analysis Dashboard")

# CSVの読み込み
try:
    # Goが出力するCSVのパスを指定（親ディレクトリにある想定）
    df = pd.read_csv("../results.csv", names=[
        "Date", "Ticker", "CompanyName", "Action", "Confidence", 
        "Reasoning", "Financials", "Technicals", "PromptID"
    ], header=0)
except FileNotFoundError:
    st.error("results.csv not found. Run the Go agent first.")
    st.stop()

# データ加工: Technicalsから数値を抽出（正規表現などでパース）
# 例: "Volatility: 4.06%" -> 4.06
import re

def extract_metric(text, pattern):
    match = re.search(pattern, str(text))
    return float(match.group(1)) if match else None

df['Volatility'] = df['Technicals'].apply(lambda x: extract_metric(x, r'Volatility:\s*([\d\.]+)%'))
df['Liquidity'] = df['Technicals'].apply(lambda x: extract_metric(x, r'Avg Trading Value:\s*([\d]+)'))

# サイドバーフィルタ
st.sidebar.header("Filter")
selected_action = st.sidebar.multiselect("Action", df['Action'].unique(), default=["BUY", "IGNORE"])
min_conf = st.sidebar.slider("Min Confidence", 0.0, 1.0, 0.5)

filtered_df = df[
    (df['Action'].isin(selected_action)) & 
    (df['Confidence'] >= min_conf)
]

# メイン表示
col1, col2 = st.columns(2)

with col1:
    st.subheader("Distribution of Decisions")
    fig = px.pie(filtered_df, names='Action', title='BUY vs IGNORE')
    st.plotly_chart(fig, use_container_width=True)

with col2:
    st.subheader("Volatility vs Confidence")
    if not filtered_df.empty:
        fig = px.scatter(
            filtered_df, 
            x='Volatility', 
            y='Confidence', 
            color='Action',
            hover_data=['Ticker', 'CompanyName', 'Reasoning'],
            title='Does AI prefer high volatility?'
        )
        st.plotly_chart(fig, use_container_width=True)

st.subheader("Detailed Records")
st.dataframe(filtered_df)

# ここに「実際の株価上昇率」を結合できれば、散布図で「勝てるゾーン」が可視化できます
st.info("Tip: Run 'backtest' and merge the result to see Win/Loss on the chart.")