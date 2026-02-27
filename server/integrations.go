package server

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const integrationsPath = "data/integrations.json"

type llmIntegration struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	BaseURL string `json:"base_url"`
	APIKey  string `json:"api_key"`
	Model   string `json:"model"`
}

type exchangeIntegration struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Exchange  string `json:"exchange"`
	APIKey    string `json:"api_key"`
	Secret    string `json:"secret"`
	Passphase string `json:"passphrase"`
}

type integrationStore struct {
	LLMs             []llmIntegration      `json:"llms"`
	Exchanges        []exchangeIntegration `json:"exchanges"`
	ActiveExchangeID string                `json:"active_exchange_id"`
}

func (s *Service) handleIntegrations(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		cfg, _ := readIntegrations()
		active := findExchangeByID(cfg.Exchanges, cfg.ActiveExchangeID)
		writeJSON(w, 200, map[string]any{
			"llms":               cfg.LLMs,
			"exchanges":          cfg.Exchanges,
			"active_exchange_id": cfg.ActiveExchangeID,
			"exchange_bound":     active != nil,
		})
	default:
		writeError(w, 405, "method not allowed")
	}
}

func (s *Service) handleAddLLMIntegration(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, 405, "method not allowed")
		return
	}
	var req llmIntegration
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, 400, "invalid json body")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	req.BaseURL = strings.TrimSpace(req.BaseURL)
	req.APIKey = strings.TrimSpace(req.APIKey)
	req.Model = strings.TrimSpace(req.Model)
	if req.Name == "" || req.BaseURL == "" || req.APIKey == "" || req.Model == "" {
		writeError(w, 400, "name/base_url/api_key/model 必填")
		return
	}
	if err := validateLLMIntegration(req); err != nil {
		writeError(w, 400, "LLM 验证失败: "+err.Error())
		return
	}
	store, _ := readIntegrations()
	req.ID = nextIntegrationIDLLM(store.LLMs)
	store.LLMs = append(filterLLMByID(store.LLMs, req.ID), req)
	if err := writeIntegrations(store); err != nil {
		writeError(w, 500, "保存失败: "+err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"added": req, "llms": store.LLMs})
}

func (s *Service) handleUpdateLLMIntegration(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, 405, "method not allowed")
		return
	}
	var req llmIntegration
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, 400, "invalid json body")
		return
	}
	req.ID = strings.TrimSpace(req.ID)
	req.Name = strings.TrimSpace(req.Name)
	req.BaseURL = strings.TrimSpace(req.BaseURL)
	req.APIKey = strings.TrimSpace(req.APIKey)
	req.Model = strings.TrimSpace(req.Model)
	if req.ID == "" {
		writeError(w, 400, "id 必填")
		return
	}
	if req.Name == "" || req.BaseURL == "" || req.APIKey == "" || req.Model == "" {
		writeError(w, 400, "name/base_url/api_key/model 必填")
		return
	}
	if err := validateLLMIntegration(req); err != nil {
		writeError(w, 400, "LLM 验证失败: "+err.Error())
		return
	}

	store, _ := readIntegrations()
	if findLLMByID(store.LLMs, req.ID) == nil {
		writeError(w, 404, "未找到指定智能体参数")
		return
	}
	next := make([]llmIntegration, 0, len(store.LLMs))
	for _, x := range store.LLMs {
		if strings.TrimSpace(x.ID) == req.ID {
			next = append(next, req)
		} else {
			next = append(next, x)
		}
	}
	store.LLMs = next
	if err := writeIntegrations(store); err != nil {
		writeError(w, 500, "保存失败: "+err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"updated": req, "llms": store.LLMs})
}

func (s *Service) handleDeleteLLMIntegration(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, 405, "method not allowed")
		return
	}
	var req struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, 400, "invalid json body")
		return
	}
	id := strings.TrimSpace(req.ID)
	if id == "" {
		writeError(w, 400, "id 必填")
		return
	}

	store, _ := readIntegrations()
	if findLLMByID(store.LLMs, id) == nil {
		writeError(w, 404, "未找到指定智能体参数")
		return
	}
	store.LLMs = filterLLMByID(store.LLMs, id)
	if err := writeIntegrations(store); err != nil {
		writeError(w, 500, "保存失败: "+err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{
		"message":    "智能体参数已删除",
		"deleted_id": id,
		"llms":       store.LLMs,
	})
}

func (s *Service) handleTestLLMIntegration(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, 405, "method not allowed")
		return
	}
	var req struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, 400, "invalid json body")
		return
	}
	id := strings.TrimSpace(req.ID)
	if id == "" {
		writeError(w, 400, "id 必填")
		return
	}

	store, err := readIntegrations()
	if err != nil {
		writeError(w, 500, "读取配置失败: "+err.Error())
		return
	}
	cfg := findLLMByID(store.LLMs, id)
	if cfg == nil {
		writeError(w, 404, "未找到指定智能体参数")
		return
	}

	if err := validateLLMIntegration(*cfg); err != nil {
		writeJSON(w, 200, map[string]any{
			"id":        id,
			"reachable": false,
			"status":    "unreachable",
			"message":   err.Error(),
		})
		return
	}
	writeJSON(w, 200, map[string]any{
		"id":        id,
		"reachable": true,
		"status":    "reachable",
		"message":   "API 可达",
	})
}

func (s *Service) handleAddExchangeIntegration(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, 405, "method not allowed")
		return
	}
	var req exchangeIntegration
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, 400, "invalid json body")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	req.Exchange = strings.ToLower(strings.TrimSpace(req.Exchange))
	req.APIKey = strings.TrimSpace(req.APIKey)
	req.Secret = strings.TrimSpace(req.Secret)
	req.Passphase = strings.TrimSpace(req.Passphase)
	if req.Exchange == "" || req.APIKey == "" || req.Secret == "" {
		writeError(w, 400, "exchange/api_key/secret 必填")
		return
	}
	if req.Name == "" {
		req.Name = req.Exchange
	}
	if req.Exchange != "binance" {
		writeError(w, 400, "当前仅支持 binance")
		return
	}
	if err := validateExchangeIntegration(req); err != nil {
		writeError(w, 400, "交易所验证失败: "+err.Error())
		return
	}
	store, _ := readIntegrations()
	req.ID = nextIntegrationIDExchange(store.Exchanges)
	store.Exchanges = append(filterExchangeByID(store.Exchanges, req.ID), req)
	if strings.TrimSpace(store.ActiveExchangeID) == "" {
		store.ActiveExchangeID = req.ID
		if err := bindExchangeAccount(s, req); err != nil {
			writeError(w, 500, "绑定失败: "+err.Error())
			return
		}
	}
	if err := writeIntegrations(store); err != nil {
		writeError(w, 500, "保存失败: "+err.Error())
		return
	}
	active := findExchangeByID(store.Exchanges, store.ActiveExchangeID)
	writeJSON(w, 200, map[string]any{
		"added":              req,
		"exchanges":          store.Exchanges,
		"active_exchange_id": store.ActiveExchangeID,
		"exchange_bound":     active != nil,
	})
}

func (s *Service) handleActivateExchangeIntegration(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, 405, "method not allowed")
		return
	}
	var req struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, 400, "invalid json body")
		return
	}
	id := strings.TrimSpace(req.ID)
	if id == "" {
		writeError(w, 400, "id 必填")
		return
	}

	store, err := readIntegrations()
	if err != nil {
		writeError(w, 500, "读取配置失败: "+err.Error())
		return
	}
	cfg := findExchangeByID(store.Exchanges, id)
	if cfg == nil {
		writeError(w, 404, "未找到指定交易所账号")
		return
	}

	if err := validateExchangeIntegration(*cfg); err != nil {
		writeError(w, 400, "交易所验证失败: "+err.Error())
		return
	}
	if err := bindExchangeAccount(s, *cfg); err != nil {
		writeError(w, 500, "绑定失败: "+err.Error())
		return
	}

	store.ActiveExchangeID = cfg.ID
	if err := writeIntegrations(store); err != nil {
		writeError(w, 500, "保存失败: "+err.Error())
		return
	}

	writeJSON(w, 200, map[string]any{
		"message":            "交易账号绑定成功",
		"active_exchange_id": store.ActiveExchangeID,
		"exchange_bound":     true,
		"active_exchange": map[string]any{
			"id":       cfg.ID,
			"exchange": cfg.Exchange,
			"api_key":  maskKey(cfg.APIKey),
		},
	})
}

func (s *Service) handleDeleteExchangeIntegration(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, 405, "method not allowed")
		return
	}
	var req struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, 400, "invalid json body")
		return
	}
	id := strings.TrimSpace(req.ID)
	if id == "" {
		writeError(w, 400, "id 必填")
		return
	}

	store, err := readIntegrations()
	if err != nil {
		writeError(w, 500, "读取配置失败: "+err.Error())
		return
	}
	target := findExchangeByID(store.Exchanges, id)
	if target == nil {
		writeError(w, 404, "未找到指定交易所账号")
		return
	}

	store.Exchanges = filterExchangeByID(store.Exchanges, id)
	wasActive := store.ActiveExchangeID == id
	if wasActive {
		store.ActiveExchangeID = ""
		if err := unbindExchangeAccount(s); err != nil {
			writeError(w, 500, "解绑失败: "+err.Error())
			return
		}
	}
	if err := writeIntegrations(store); err != nil {
		writeError(w, 500, "保存失败: "+err.Error())
		return
	}

	writeJSON(w, 200, map[string]any{
		"message":            "交易所账号已删除",
		"deleted_id":         id,
		"exchanges":          store.Exchanges,
		"active_exchange_id": store.ActiveExchangeID,
		"exchange_bound":     strings.TrimSpace(store.ActiveExchangeID) != "",
	})
}

func filterLLMByID(in []llmIntegration, id string) []llmIntegration {
	out := make([]llmIntegration, 0, len(in))
	for _, x := range in {
		if x.ID != id {
			out = append(out, x)
		}
	}
	return out
}

func findLLMByID(in []llmIntegration, id string) *llmIntegration {
	for i := range in {
		if strings.TrimSpace(in[i].ID) == strings.TrimSpace(id) {
			return &in[i]
		}
	}
	return nil
}

func filterExchangeByID(in []exchangeIntegration, id string) []exchangeIntegration {
	out := make([]exchangeIntegration, 0, len(in))
	for _, x := range in {
		if x.ID != id {
			out = append(out, x)
		}
	}
	return out
}

func findExchangeByID(in []exchangeIntegration, id string) *exchangeIntegration {
	for i := range in {
		if strings.TrimSpace(in[i].ID) == strings.TrimSpace(id) {
			return &in[i]
		}
	}
	return nil
}

func maskKey(v string) string {
	s := strings.TrimSpace(v)
	if s == "" {
		return ""
	}
	if len(s) <= 8 {
		return s[:2] + "***"
	}
	return s[:4] + "****" + s[len(s)-4:]
}

func validateLLMIntegration(cfg llmIntegration) error {
	endpoint := strings.TrimSpace(cfg.BaseURL)
	u, err := url.Parse(endpoint)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return fmt.Errorf("base_url 非法")
	}
	body := map[string]any{
		"model": cfg.Model,
		"messages": []map[string]string{
			{"role": "system", "content": "reply with JSON only"},
			{"role": "user", "content": "ping"},
		},
		"temperature": 0,
		"stream":      false,
	}
	raw, _ := json.Marshal(body)
	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+cfg.APIKey)
	cli := &http.Client{Timeout: 10 * time.Second}
	resp, err := cli.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	bs, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(bs))
	}
	return nil
}

func validateExchangeIntegration(cfg exchangeIntegration) error {
	cli := &http.Client{Timeout: 10 * time.Second}
	pingResp, err := cli.Get("https://fapi.binance.com/fapi/v1/ping")
	if err != nil {
		return err
	}
	_ = pingResp.Body.Close()
	if pingResp.StatusCode >= 300 {
		return fmt.Errorf("ping http %d", pingResp.StatusCode)
	}

	vals := url.Values{}
	vals.Set("timestamp", strconv.FormatInt(time.Now().UnixMilli(), 10))
	vals.Set("recvWindow", "5000")
	raw := vals.Encode()
	h := hmac.New(sha256.New, []byte(cfg.Secret))
	h.Write([]byte(raw))
	sig := hex.EncodeToString(h.Sum(nil))
	u := "https://fapi.binance.com/fapi/v2/account?" + raw + "&signature=" + sig
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("X-MBX-APIKEY", cfg.APIKey)
	resp, err := cli.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return fmt.Errorf("account http %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

func readIntegrations() (integrationStore, error) {
	var cfg integrationStore
	fileMissing := false
	raw, err := os.ReadFile(integrationsPath)
	if err == nil {
		if err := json.Unmarshal(raw, &cfg); err != nil {
			// Keep behavior robust: fallback to env snapshot when file is malformed.
			cfg = integrationStore{}
		}
	} else if os.IsNotExist(err) {
		fileMissing = true
	} else {
		// Keep endpoint available even when file read fails for transient reasons.
		cfg = integrationStore{}
	}

	// Only bootstrap from env when local integration file does not exist yet.
	if fileMissing {
		mergeIntegrationsFromEnv(&cfg)
	}
	return cfg, nil
}

func mergeIntegrationsFromEnv(cfg *integrationStore) {
	if cfg == nil {
		return
	}
	if len(cfg.LLMs) == 0 {
		baseURL := strings.TrimSpace(os.Getenv("AI_BASE_URL"))
		apiKey := strings.TrimSpace(os.Getenv("AI_API_KEY"))
		model := strings.TrimSpace(os.Getenv("AI_MODEL"))
		if baseURL != "" || apiKey != "" || model != "" {
			cfg.LLMs = append(cfg.LLMs, llmIntegration{
				ID:      "1",
				Name:    "ENV 智能体",
				BaseURL: baseURL,
				APIKey:  apiKey,
				Model:   model,
			})
		}
	}
	if len(cfg.Exchanges) == 0 {
		apiKey := strings.TrimSpace(os.Getenv("BINANCE_API_KEY"))
		secret := strings.TrimSpace(os.Getenv("BINANCE_SECRET"))
		if apiKey != "" || secret != "" {
			cfg.Exchanges = append(cfg.Exchanges, exchangeIntegration{
				ID:       "1",
				Name:     "ENV Binance",
				Exchange: "binance",
				APIKey:   apiKey,
				Secret:   secret,
			})
		}
	}
	if strings.TrimSpace(cfg.ActiveExchangeID) == "" && len(cfg.Exchanges) > 0 {
		cfg.ActiveExchangeID = strings.TrimSpace(cfg.Exchanges[0].ID)
	}
}

func writeIntegrations(cfg integrationStore) error {
	if err := os.MkdirAll(filepath.Dir(integrationsPath), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	return os.WriteFile(integrationsPath, raw, 0o644)
}

func nextIntegrationIDLLM(items []llmIntegration) string {
	maxID := 0
	for _, x := range items {
		id, err := strconv.Atoi(strings.TrimSpace(x.ID))
		if err == nil && id > maxID {
			maxID = id
		}
	}
	return strconv.Itoa(maxID + 1)
}

func nextIntegrationIDExchange(items []exchangeIntegration) string {
	maxID := 0
	for _, x := range items {
		id, err := strconv.Atoi(strings.TrimSpace(x.ID))
		if err == nil && id > maxID {
			maxID = id
		}
	}
	return strconv.Itoa(maxID + 1)
}

func bindExchangeAccount(s *Service, cfg exchangeIntegration) error {
	updates := map[string]string{
		"BINANCE_API_KEY": strings.TrimSpace(cfg.APIKey),
		"BINANCE_SECRET":  strings.TrimSpace(cfg.Secret),
	}
	if err := upsertDotEnv(".env", updates); err != nil {
		return err
	}
	for k, v := range updates {
		_ = os.Setenv(k, v)
	}
	applyRuntimeConfigFromEnv()
	return s.bot.ReloadClients()
}

func unbindExchangeAccount(s *Service) error {
	updates := map[string]string{
		"BINANCE_API_KEY": "",
		"BINANCE_SECRET":  "",
	}
	if err := upsertDotEnv(".env", updates); err != nil {
		return err
	}
	for k, v := range updates {
		_ = os.Setenv(k, v)
	}
	applyRuntimeConfigFromEnv()
	return s.bot.ReloadClients()
}
