package ai

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
	"trade-go/config"
	"trade-go/models"
)

type Client struct {
	apiKey      string
	aiBaseURL   string
	aiModel     string
	strategyURL string
	httpClient  *http.Client
}

func NewClient() *Client {
	model := strings.TrimSpace(config.Config.AIModel)
	if model == "" {
		model = "chat-model"
	}
	return &Client{
		apiKey:      config.Config.AIAPIKey,
		aiBaseURL:   strings.TrimSpace(config.Config.AIBaseURL),
		aiModel:     model,
		strategyURL: strings.TrimSpace(config.Config.PyStrategyURL),
		httpClient:  &http.Client{Timeout: 60 * time.Second},
	}
}

type chatRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	Temperature float64       `json:"temperature"`
	Stream      bool          `json:"stream"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

func (c *Client) Analyze(priceData models.PriceData, currentPos *models.Position, lastSignals []models.TradeSignal) (models.TradeSignal, error) {
	if c.strategyURL != "" {
		sig, err := c.analyzeByPython(priceData, currentPos, lastSignals)
		if err == nil {
			return sig, nil
		}
		fmt.Printf("Python策略服务调用失败，尝试通用AI兜底: %v\n", err)
		if c.apiKey == "" || c.aiBaseURL == "" {
			return fallbackSignal(priceData), nil
		}
	}
	if c.apiKey == "" || c.aiBaseURL == "" {
		return fallbackSignal(priceData), nil
	}

	prompt := buildPrompt(priceData, currentPos, lastSignals)

	cfg := config.Config.Trade
	sysDefault := fmt.Sprintf(
		"你是专业量化交易决策引擎。交易标的=%s，周期=%s。你只能输出严格JSON，不要输出任何额外文本。你负责方向与SL/TP建议，仓位和风控由系统执行。",
		cfg.Symbol,
		cfg.Timeframe,
	)
	sysMsg := strings.TrimSpace(os.Getenv("TRADING_AI_SYSTEM_PROMPT"))
	if sysMsg == "" {
		sysMsg = sysDefault
	}

	reqBody := chatRequest{
		Model: c.aiModel,
		Messages: []chatMessage{
			{Role: "system", Content: sysMsg},
			{Role: "user", Content: prompt},
		},
		Temperature: 0.1,
		Stream:      false,
	}

	b, _ := json.Marshal(reqBody)
	req, err := http.NewRequest("POST", c.aiBaseURL, bytes.NewReader(b))
	if err != nil {
		return models.TradeSignal{}, err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return models.TradeSignal{}, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var chatResp chatResponse
	if err := json.Unmarshal(body, &chatResp); err != nil {
		return models.TradeSignal{}, fmt.Errorf("解析响应失败: %w", err)
	}
	if len(chatResp.Choices) == 0 {
		return models.TradeSignal{}, fmt.Errorf("AI 返回空响应")
	}

	content := chatResp.Choices[0].Message.Content
	fmt.Printf("AI 原始回复: %s\n", content)

	signal, err := parseSignal(content)
	if err != nil {
		return fallbackSignal(priceData), nil
	}
	ensureStrategyMeta(&signal)
	signal.Timestamp = time.Now()
	return signal, nil
}

func (c *Client) analyzeByPython(priceData models.PriceData, currentPos *models.Position, lastSignals []models.TradeSignal) (models.TradeSignal, error) {
	url := strings.TrimRight(c.strategyURL, "/") + "/analyze"
	reqBody := map[string]interface{}{
		"price_data":   priceData,
		"current_pos":  currentPos,
		"last_signals": lastSignals,
		"timeframe":    config.Config.Trade.Timeframe,
		"symbol":       config.Config.Trade.Symbol,
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return models.TradeSignal{}, err
	}
	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return models.TradeSignal{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return models.TradeSignal{}, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return models.TradeSignal{}, fmt.Errorf("python strategy http %d: %s", resp.StatusCode, string(raw))
	}

	var sig models.TradeSignal
	if err := json.Unmarshal(raw, &sig); err != nil {
		return models.TradeSignal{}, fmt.Errorf("解析python策略响应失败: %w", err)
	}
	if sig.Signal == "" || sig.StopLoss == 0 || sig.TakeProfit == 0 {
		return models.TradeSignal{}, fmt.Errorf("python策略响应字段不完整")
	}
	ensureStrategyMeta(&sig)
	sig.Timestamp = time.Now()
	return sig, nil
}

func parseSignal(content string) (models.TradeSignal, error) {
	start := strings.Index(content, "{")
	end := strings.LastIndex(content, "}") + 1
	if start == -1 || end <= start {
		return models.TradeSignal{}, fmt.Errorf("未找到 JSON")
	}
	jsonStr := content[start:end]

	var sig models.TradeSignal
	if err := json.Unmarshal([]byte(jsonStr), &sig); err != nil {
		return models.TradeSignal{}, err
	}
	if sig.Signal == "" || sig.StopLoss == 0 || sig.TakeProfit == 0 {
		return models.TradeSignal{}, fmt.Errorf("信号字段不完整")
	}
	return sig, nil
}

func fallbackSignal(pd models.PriceData) models.TradeSignal {
	return models.TradeSignal{
		Signal:        "HOLD",
		Reason:        "因技术分析暂时不可用，采取保守策略",
		StopLoss:      pd.Price * 0.98,
		TakeProfit:    pd.Price * 1.02,
		Confidence:    "LOW",
		StrategyCombo: "fallback_conservative",
		IsFallback:    true,
		Timestamp:     time.Now(),
	}
}

func ensureStrategyMeta(sig *models.TradeSignal) {
	if sig == nil {
		return
	}
	if strings.TrimSpace(sig.StrategyCombo) == "" {
		switch strings.ToUpper(strings.TrimSpace(sig.Signal)) {
		case "BUY":
			sig.StrategyCombo = "ai_buy_generic"
		case "SELL":
			sig.StrategyCombo = "ai_sell_generic"
		default:
			sig.StrategyCombo = "ai_hold_generic"
		}
	}
	if sig.StrategyScore < 0 {
		sig.StrategyScore = 0
	}
	if sig.StrategyScore > 10 {
		sig.StrategyScore = 10
	}
}

func rsiStatus(rsi float64) string {
	if rsi > 70 {
		return "超买"
	} else if rsi < 30 {
		return "超卖"
	}
	return "中性"
}

func bbPosStr(pos float64) string {
	if pos > 0.7 {
		return "上部"
	} else if pos < 0.3 {
		return "下部"
	}
	return "中部"
}

func buildPrompt(pd models.PriceData, pos *models.Position, lastSignals []models.TradeSignal) string {
	cfg := config.Config.Trade
	t := pd.Technical
	tr := pd.Trend
	lv := pd.Levels

	var klines strings.Builder
	klines.WriteString(fmt.Sprintf("【最近5根%s K线数据】\n", cfg.Timeframe))
	last5 := pd.KlineData
	if len(last5) > 5 {
		last5 = last5[len(last5)-5:]
	}
	for i, k := range last5 {
		name := "阴线"
		if k.Close > k.Open {
			name = "阳线"
		}
		change := (k.Close - k.Open) / k.Open * 100
		klines.WriteString(fmt.Sprintf("K线%d: %s 开盘:%.2f 收盘:%.2f 涨跌:%+.2f%%\n", i+1, name, k.Open, k.Close, change))
	}

	var lastSigText string
	if len(lastSignals) > 0 {
		ls := lastSignals[len(lastSignals)-1]
		lastSigText = fmt.Sprintf("\n【上次交易信号】\n信号: %s\n信心: %s", ls.Signal, ls.Confidence)
	}

	posText := "无持仓"
	posLoss := "0"
	if pos != nil {
		posText = fmt.Sprintf("%s仓, 数量: %.4f, 盈亏: %.2f USDT", pos.Side, pos.Size, pos.UnrealizedPnL)
		posLoss = fmt.Sprintf("%.2f", pos.UnrealizedPnL)
	}

	sma5Pct, sma20Pct, sma50Pct := 0.0, 0.0, 0.0
	if t.SMA5 != 0 {
		sma5Pct = (pd.Price - t.SMA5) / t.SMA5 * 100
	}
	if t.SMA20 != 0 {
		sma20Pct = (pd.Price - t.SMA20) / t.SMA20 * 100
	}
	if t.SMA50 != 0 {
		sma50Pct = (pd.Price - t.SMA50) / t.SMA50 * 100
	}

	policyPrompt := strings.TrimSpace(os.Getenv("TRADING_AI_POLICY_PROMPT"))
	if policyPrompt == "" {
		policyPrompt = "优先保护本金；信号冲突或不确定时返回HOLD；避免低置信度反转。"
	}

	return fmt.Sprintf(`请作为量化交易决策模型，基于以下%s %s数据进行判断。
你需要先判断市场状态（趋势/震荡/高波动），再决定信号。若信号不清晰，必须返回HOLD。

%s

【技术指标】
移动平均线:
- 5周期: %.2f | 价格相对: %+.2f%%
- 20周期: %.2f | 价格相对: %+.2f%%
- 50周期: %.2f | 价格相对: %+.2f%%

趋势分析:
- 短期趋势: %s | 中期趋势: %s | 整体趋势: %s | MACD方向: %s

动量指标:
- RSI: %.2f (%s) | MACD: %.4f | 信号线: %.4f
- 布林带位置: %.2f%% (%s)

关键水平:
- 静态阻力: %.2f | 静态支撑: %.2f

%s

【风控与执行约束】
1. 你只负责方向和止盈止损建议，仓位大小由Risk Engine决定。
2. 非高置信度时避免频繁反转；若与当前持仓冲突且证据不足，应优先HOLD。
3. 止损必须有效（>0），且不应过近；止盈应与止损形成合理风险收益比（建议>=1.2）。
4. 当趋势与动量冲突或波动异常时，优先保守。
5. 不要输出固定下单金额/固定仓位建议；实际下单数量、保证金比例、杠杆以系统实盘设置为准。

【当前实盘参数（仅供参考，执行以系统为准）】
- 仓位模式: %s
- 高信心张数: %.6f | 低信心张数: %.6f
- 高信心保证金比例: %.2f%% | 低信心保证金比例: %.2f%%
- 杠杆: %d

【策略偏好补充】
%s

【当前行情快照】
- 价格: $%.2f | 时间: %s
- 最高: $%.2f | 最低: $%.2f | 成交量: %.2f BTC | 变化: %+.2f%%
- 持仓: %s | 盈亏: %s USDT

【输出要求】
只返回JSON对象，字段必须齐全：
{"signal":"BUY|SELL|HOLD","reason":"<=80字","stop_loss":数字,"take_profit":数字,"confidence":"HIGH|MEDIUM|LOW","strategy_combo":"trend_following|mean_reversion|breakout|no_trade"}

禁止输出markdown、代码块、解释性前后缀。`,
		cfg.Symbol, cfg.Timeframe, klines.String(),
		t.SMA5, sma5Pct, t.SMA20, sma20Pct, t.SMA50, sma50Pct,
		tr.ShortTerm, tr.MediumTerm, tr.Overall, tr.MACD,
		t.RSI, rsiStatus(t.RSI), t.MACD, t.MACDSignal,
		t.BBPosition*100, bbPosStr(t.BBPosition),
		lv.StaticResistance, lv.StaticSupport,
		lastSigText,
		cfg.PositionSizingMode, cfg.HighConfidenceAmount, cfg.LowConfidenceAmount,
		cfg.HighConfidenceMarginPct*100, cfg.LowConfidenceMarginPct*100, cfg.Leverage,
		policyPrompt,
		pd.Price, pd.Timestamp.Format("2006-01-02 15:04:05"),
		pd.High, pd.Low, pd.Volume, pd.PriceChange,
		posText, posLoss,
	)
}
