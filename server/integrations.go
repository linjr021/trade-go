package server

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
	"trade-go/llmapi"
)

const integrationsPath = "data/integrations.json"

type llmIntegration struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	ProductID string `json:"product_id"`
	Product   string `json:"product"`
	BaseURL   string `json:"base_url"`
	APIKey    string `json:"api_key"`
	Model     string `json:"model"`
}

type llmProduct struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Product string `json:"product"`
	BaseURL string `json:"base_url"`
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
	LLMProducts      []llmProduct          `json:"llm_products"`
	Exchanges        []exchangeIntegration `json:"exchanges"`
	ActiveExchangeID string                `json:"active_exchange_id"`
}

func (s *Service) handleIntegrations(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		cfg, _ := readIntegrations()
		active := findExchangeByID(cfg.Exchanges, cfg.ActiveExchangeID)
		writeJSON(w, 200, map[string]any{
			"llms":                cfg.LLMs,
			"llm_products":        cfg.LLMProducts,
			"llm_product_catalog": llmProductCatalog(),
			"exchanges":           cfg.Exchanges,
			"active_exchange_id":  cfg.ActiveExchangeID,
			"exchange_bound":      active != nil,
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
	req.ProductID = strings.TrimSpace(req.ProductID)
	req.Product = normalizeLLMProduct(req.Product)
	req.BaseURL = strings.TrimSpace(req.BaseURL)
	req.APIKey = strings.TrimSpace(req.APIKey)
	req.Model = strings.TrimSpace(req.Model)
	if req.Product == "" {
		req.Product = inferLLMProductFromBaseURL(req.BaseURL)
	}
	if err := validateLLMProduct(req.Product); err != nil {
		writeError(w, 400, err.Error())
		return
	}
	req.ProductID = ""
	req.BaseURL = llmProductBaseURL(req.Product)
	if req.Name == "" || req.BaseURL == "" || req.APIKey == "" || req.Model == "" {
		writeError(w, 400, "name/base_url/api_key/model 必填")
		return
	}
	store, _ := readIntegrations()
	if err := validateLLMIntegration(req); err != nil {
		writeError(w, 400, "LLM 验证失败: "+err.Error())
		return
	}
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
	req.ProductID = strings.TrimSpace(req.ProductID)
	req.Product = normalizeLLMProduct(req.Product)
	req.BaseURL = strings.TrimSpace(req.BaseURL)
	req.APIKey = strings.TrimSpace(req.APIKey)
	req.Model = strings.TrimSpace(req.Model)
	if req.ID == "" {
		writeError(w, 400, "id 必填")
		return
	}
	store, _ := readIntegrations()
	if req.Product == "" {
		if exist := findLLMByID(store.LLMs, req.ID); exist != nil {
			req.Product = normalizeLLMProduct(exist.Product)
		}
		if req.Product == "" {
			req.Product = inferLLMProductFromBaseURL(req.BaseURL)
		}
	}
	if err := validateLLMProduct(req.Product); err != nil {
		writeError(w, 400, err.Error())
		return
	}
	req.ProductID = ""
	req.BaseURL = llmProductBaseURL(req.Product)
	if req.Name == "" || req.BaseURL == "" || req.APIKey == "" || req.Model == "" {
		writeError(w, 400, "name/base_url/api_key/model 必填")
		return
	}
	if err := validateLLMIntegration(req); err != nil {
		writeError(w, 400, "LLM 验证失败: "+err.Error())
		return
	}

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

func (s *Service) handleProbeLLMModels(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, 405, "method not allowed")
		return
	}
	var req struct {
		Product string `json:"product"`
		BaseURL string `json:"base_url"`
		APIKey  string `json:"api_key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, 400, "invalid json body")
		return
	}
	req.Product = normalizeLLMProduct(req.Product)
	req.BaseURL = strings.TrimSpace(req.BaseURL)
	req.APIKey = strings.TrimSpace(req.APIKey)
	if req.Product == "" {
		req.Product = inferLLMProductFromBaseURL(req.BaseURL)
	}
	if err := validateLLMProduct(req.Product); err != nil {
		writeError(w, 400, err.Error())
		return
	}
	req.BaseURL = llmProductBaseURL(req.Product)
	if req.BaseURL == "" || req.APIKey == "" {
		writeError(w, 400, "base_url/api_key 必填")
		return
	}

	models, chatEndpoint, modelsEndpoint, routeReachable, reachable, msg, err := fetchLLMModelList(req.Product, req.BaseURL, req.APIKey)
	if err != nil {
		writeJSON(w, 200, map[string]any{
			"product":         req.Product,
			"chat_endpoint":   chatEndpoint,
			"models_endpoint": modelsEndpoint,
			"route_reachable": routeReachable,
			"reachable":       reachable,
			"models":          []string{},
			"message":         err.Error(),
		})
		return
	}
	writeJSON(w, 200, map[string]any{
		"product":         req.Product,
		"chat_endpoint":   chatEndpoint,
		"models_endpoint": modelsEndpoint,
		"route_reachable": routeReachable,
		"reachable":       reachable,
		"models":          models,
		"message":         msg,
	})
}

func (s *Service) handleAddLLMProduct(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, 405, "method not allowed")
		return
	}
	var req llmProduct
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, 400, "invalid json body")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	req.Product = normalizeLLMProduct(req.Product)
	req.BaseURL = strings.TrimSpace(req.BaseURL)
	if req.Name == "" || req.BaseURL == "" {
		writeError(w, 400, "name/base_url 必填")
		return
	}
	if err := validateLLMProduct(req.Product); err != nil {
		writeError(w, 400, err.Error())
		return
	}
	if _, err := llmapi.ResolveChatEndpoint(req.BaseURL); err != nil {
		writeError(w, 400, err.Error())
		return
	}
	store, _ := readIntegrations()
	req.ID = nextIntegrationIDLLMProduct(store.LLMProducts)
	store.LLMProducts = append(filterLLMProductByID(store.LLMProducts, req.ID), req)
	if err := writeIntegrations(store); err != nil {
		writeError(w, 500, "保存失败: "+err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"added": req, "llm_products": store.LLMProducts})
}

func (s *Service) handleUpdateLLMProduct(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, 405, "method not allowed")
		return
	}
	var req llmProduct
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, 400, "invalid json body")
		return
	}
	req.ID = strings.TrimSpace(req.ID)
	req.Name = strings.TrimSpace(req.Name)
	req.Product = normalizeLLMProduct(req.Product)
	req.BaseURL = strings.TrimSpace(req.BaseURL)
	if req.ID == "" {
		writeError(w, 400, "id 必填")
		return
	}
	if req.Name == "" || req.BaseURL == "" {
		writeError(w, 400, "name/base_url 必填")
		return
	}
	if err := validateLLMProduct(req.Product); err != nil {
		writeError(w, 400, err.Error())
		return
	}
	if _, err := llmapi.ResolveChatEndpoint(req.BaseURL); err != nil {
		writeError(w, 400, err.Error())
		return
	}

	store, _ := readIntegrations()
	if findLLMProductByID(store.LLMProducts, req.ID) == nil {
		writeError(w, 404, "未找到指定智能体产品")
		return
	}
	nextProducts := make([]llmProduct, 0, len(store.LLMProducts))
	for _, x := range store.LLMProducts {
		if strings.TrimSpace(x.ID) == req.ID {
			nextProducts = append(nextProducts, req)
		} else {
			nextProducts = append(nextProducts, x)
		}
	}
	store.LLMProducts = nextProducts

	// sync existing llm configs using this preset
	for i := range store.LLMs {
		if strings.TrimSpace(store.LLMs[i].ProductID) == req.ID {
			store.LLMs[i].Product = req.Product
			store.LLMs[i].BaseURL = req.BaseURL
		}
	}

	if err := writeIntegrations(store); err != nil {
		writeError(w, 500, "保存失败: "+err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{
		"updated":      req,
		"llm_products": store.LLMProducts,
		"llms":         store.LLMs,
	})
}

func (s *Service) handleDeleteLLMProduct(w http.ResponseWriter, r *http.Request) {
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
	target := findLLMProductByID(store.LLMProducts, id)
	if target == nil {
		writeError(w, 404, "未找到指定智能体产品")
		return
	}
	for _, llm := range store.LLMs {
		if strings.TrimSpace(llm.ProductID) == id {
			writeError(w, 400, "该产品已被智能体参数引用，无法删除")
			return
		}
		if strings.TrimSpace(llm.ProductID) == "" &&
			strings.TrimSpace(llm.BaseURL) == strings.TrimSpace(target.BaseURL) &&
			normalizeLLMProduct(llm.Product) == normalizeLLMProduct(target.Product) {
			writeError(w, 400, "该产品已被智能体参数引用，无法删除")
			return
		}
	}
	store.LLMProducts = filterLLMProductByID(store.LLMProducts, id)
	if len(store.LLMProducts) == 0 {
		store.LLMProducts = defaultLLMProducts()
	}
	if err := writeIntegrations(store); err != nil {
		writeError(w, 500, "保存失败: "+err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{
		"message":      "智能体产品已删除",
		"deleted_id":   id,
		"llm_products": store.LLMProducts,
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
	if req.Exchange != "binance" && req.Exchange != "okx" {
		writeError(w, 400, "当前仅支持 binance / okx")
		return
	}
	if req.Exchange == "okx" && req.Passphase == "" {
		writeError(w, 400, "OKX 需要填写 passphrase")
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

func filterLLMProductByID(in []llmProduct, id string) []llmProduct {
	out := make([]llmProduct, 0, len(in))
	for _, x := range in {
		if strings.TrimSpace(x.ID) != strings.TrimSpace(id) {
			out = append(out, x)
		}
	}
	return out
}

func findLLMProductByID(in []llmProduct, id string) *llmProduct {
	target := strings.TrimSpace(id)
	if target == "" {
		return nil
	}
	for i := range in {
		if strings.TrimSpace(in[i].ID) == target {
			return &in[i]
		}
	}
	return nil
}

func resolveLLMProductForIntegration(store integrationStore, req llmIntegration) (llmProduct, bool) {
	if p := findLLMProductByID(store.LLMProducts, req.ProductID); p != nil {
		return *p, true
	}
	baseURL := strings.TrimSpace(req.BaseURL)
	product := normalizeLLMProduct(req.Product)
	if baseURL == "" {
		return llmProduct{}, false
	}
	for _, x := range store.LLMProducts {
		if strings.TrimSpace(x.BaseURL) == baseURL && normalizeLLMProduct(x.Product) == product {
			return x, true
		}
	}
	return llmProduct{}, false
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
	cfg.Product = normalizeLLMProduct(cfg.Product)
	if err := validateLLMProduct(cfg.Product); err != nil {
		return err
	}
	cfg.BaseURL = llmProductBaseURL(cfg.Product)
	endpoint, err := llmapi.ResolveChatEndpoint(cfg.BaseURL)
	if err != nil {
		return err
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

func validateLLMProduct(product string) error {
	p := normalizeLLMProduct(product)
	if _, ok := llmProductPresetByProduct(p); !ok {
		return fmt.Errorf("当前不支持该产品类型：%s", p)
	}
	return nil
}

func normalizeLLMProduct(product string) string {
	p := strings.ToLower(strings.TrimSpace(product))
	switch p {
	case "", "chatgpt", "gpt", "openai":
		return "chatgpt"
	case "deepseek":
		return "deepseek"
	case "glm", "zhipu", "bigmodel":
		return "glm"
	case "qwen", "tongyi", "dashscope":
		return "qwen"
	case "minimax", "mini_max":
		return "minimax"
	default:
		return p
	}
}

func llmProductCatalog() []llmProduct {
	return []llmProduct{
		{Name: "ChatGPT", Product: "chatgpt", BaseURL: "https://api.openai.com/v1"},
		{Name: "DeepSeek", Product: "deepseek", BaseURL: "https://api.deepseek.com/v1"},
		{Name: "GLM", Product: "glm", BaseURL: "https://open.bigmodel.cn/api/paas/v4"},
		{Name: "Qwen", Product: "qwen", BaseURL: "https://dashscope.aliyuncs.com/compatible-mode/v1"},
		{Name: "MiniMax", Product: "minimax", BaseURL: "https://api.minimax.chat/v1"},
	}
}

func llmProductPresetByProduct(product string) (llmProduct, bool) {
	p := normalizeLLMProduct(product)
	for _, item := range llmProductCatalog() {
		if normalizeLLMProduct(item.Product) == p {
			return item, true
		}
	}
	return llmProduct{}, false
}

func llmProductBaseURL(product string) string {
	if preset, ok := llmProductPresetByProduct(product); ok {
		return strings.TrimSpace(preset.BaseURL)
	}
	return ""
}

func inferLLMProductFromBaseURL(baseURL string) string {
	base := strings.ToLower(strings.TrimSpace(baseURL))
	if base == "" {
		return "chatgpt"
	}
	for _, item := range llmProductCatalog() {
		if strings.Contains(base, strings.ToLower(strings.TrimSpace(item.BaseURL))) {
			return normalizeLLMProduct(item.Product)
		}
	}
	switch {
	case strings.Contains(base, "openai.com"):
		return "chatgpt"
	case strings.Contains(base, "deepseek.com"):
		return "deepseek"
	case strings.Contains(base, "open.bigmodel.cn"):
		return "glm"
	case strings.Contains(base, "dashscope.aliyuncs.com"):
		return "qwen"
	case strings.Contains(base, "minimax.chat"):
		return "minimax"
	default:
		return "chatgpt"
	}
}

func fetchLLMModelList(product, baseURL, apiKey string) (models []string, chatEndpoint string, modelsEndpoint string, routeReachable bool, reachable bool, message string, err error) {
	chatEndpoint, err = llmapi.ResolveChatEndpoint(baseURL)
	if err != nil {
		return nil, "", "", false, false, "", err
	}
	modelsEndpoint, err = llmapi.ResolveModelsEndpoint(baseURL)
	if err != nil {
		return nil, chatEndpoint, "", false, false, "", err
	}

	cli := &http.Client{Timeout: 12 * time.Second}
	routeReq, err := http.NewRequest(http.MethodGet, modelsEndpoint, nil)
	if err != nil {
		return nil, chatEndpoint, modelsEndpoint, false, false, "", err
	}
	routeResp, err := cli.Do(routeReq)
	if err != nil {
		return nil, chatEndpoint, modelsEndpoint, false, false, "", fmt.Errorf("模型路由不可达: %v", err)
	}
	routeBody, _ := io.ReadAll(routeResp.Body)
	_ = routeResp.Body.Close()
	if routeResp.StatusCode == http.StatusNotFound {
		return nil, chatEndpoint, modelsEndpoint, false, false, "", fmt.Errorf("模型路由不可达: HTTP 404")
	}
	if routeResp.StatusCode >= 500 {
		return nil, chatEndpoint, modelsEndpoint, false, false, "", fmt.Errorf("模型路由异常: HTTP %d", routeResp.StatusCode)
	}
	_ = routeBody
	routeReachable = true

	modelReq, err := http.NewRequest(http.MethodGet, modelsEndpoint, nil)
	if err != nil {
		return nil, chatEndpoint, modelsEndpoint, routeReachable, false, "", err
	}
	modelReq.Header.Set("Authorization", "Bearer "+apiKey)
	modelReq.Header.Set("Content-Type", "application/json")
	modelResp, err := cli.Do(modelReq)
	if err != nil {
		return nil, chatEndpoint, modelsEndpoint, routeReachable, false, "", fmt.Errorf("模型列表请求失败: %v", err)
	}
	modelBody, _ := io.ReadAll(modelResp.Body)
	_ = modelResp.Body.Close()
	if modelResp.StatusCode >= 300 {
		return nil, chatEndpoint, modelsEndpoint, routeReachable, false, "", fmt.Errorf("HTTP %d: %s", modelResp.StatusCode, strings.TrimSpace(string(modelBody)))
	}

	var parsed struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(modelBody, &parsed); err != nil {
		return nil, chatEndpoint, modelsEndpoint, routeReachable, false, "", fmt.Errorf("模型列表解析失败: %v", err)
	}
	idMap := map[string]struct{}{}
	for _, item := range parsed.Data {
		id := strings.TrimSpace(item.ID)
		if id == "" {
			continue
		}
		idMap[id] = struct{}{}
	}
	models = make([]string, 0, len(idMap))
	for id := range idMap {
		models = append(models, id)
	}
	sort.Strings(models)
	if len(models) == 0 {
		return []string{}, chatEndpoint, modelsEndpoint, routeReachable, true, "API 可达，但未返回可用模型", nil
	}
	return models, chatEndpoint, modelsEndpoint, routeReachable, true, fmt.Sprintf("API 可达，获取到 %d 个可用模型", len(models)), nil
}

func validateBinanceIntegration(cfg exchangeIntegration) error {
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

func validateOKXIntegration(cfg exchangeIntegration) error {
	if strings.TrimSpace(cfg.Passphase) == "" {
		return fmt.Errorf("okx passphrase 必填")
	}
	cli := &http.Client{Timeout: 12 * time.Second}

	// public ping
	publicResp, err := cli.Get("https://www.okx.com/api/v5/public/time")
	if err != nil {
		return err
	}
	_ = publicResp.Body.Close()
	if publicResp.StatusCode >= 300 {
		return fmt.Errorf("okx public http %d", publicResp.StatusCode)
	}

	// signed account check
	method := http.MethodGet
	pathWithQuery := "/api/v5/account/balance?ccy=USDT"
	ts := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	preHash := ts + method + pathWithQuery
	h := hmac.New(sha256.New, []byte(cfg.Secret))
	h.Write([]byte(preHash))
	signature := base64.StdEncoding.EncodeToString(h.Sum(nil))
	req, err := http.NewRequest(method, "https://www.okx.com"+pathWithQuery, nil)
	if err != nil {
		return err
	}
	req.Header.Set("OK-ACCESS-KEY", cfg.APIKey)
	req.Header.Set("OK-ACCESS-SIGN", signature)
	req.Header.Set("OK-ACCESS-TIMESTAMP", ts)
	req.Header.Set("OK-ACCESS-PASSPHRASE", cfg.Passphase)
	req.Header.Set("Content-Type", "application/json")
	resp, err := cli.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return fmt.Errorf("okx account http %d: %s", resp.StatusCode, string(body))
	}
	var parsed struct {
		Code string `json:"code"`
		Msg  string `json:"msg"`
	}
	if err := json.Unmarshal(body, &parsed); err == nil {
		if strings.TrimSpace(parsed.Code) != "" && strings.TrimSpace(parsed.Code) != "0" {
			if strings.TrimSpace(parsed.Msg) != "" {
				return fmt.Errorf("okx code=%s: %s", parsed.Code, parsed.Msg)
			}
			return fmt.Errorf("okx code=%s", parsed.Code)
		}
	}
	return nil
}

func validateExchangeIntegration(cfg exchangeIntegration) error {
	switch strings.ToLower(strings.TrimSpace(cfg.Exchange)) {
	case "binance":
		return validateBinanceIntegration(cfg)
	case "okx":
		return validateOKXIntegration(cfg)
	default:
		return fmt.Errorf("当前仅支持 binance / okx")
	}
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
	normalizeIntegrationStore(&cfg)
	return cfg, nil
}

func normalizeIntegrationStore(cfg *integrationStore) {
	if cfg == nil {
		return
	}
	if len(cfg.LLMProducts) == 0 {
		cfg.LLMProducts = defaultLLMProducts()
	}
	for i := range cfg.LLMProducts {
		cfg.LLMProducts[i].ID = strings.TrimSpace(cfg.LLMProducts[i].ID)
		cfg.LLMProducts[i].Name = strings.TrimSpace(cfg.LLMProducts[i].Name)
		cfg.LLMProducts[i].Product = normalizeLLMProduct(cfg.LLMProducts[i].Product)
		if err := validateLLMProduct(cfg.LLMProducts[i].Product); err != nil {
			cfg.LLMProducts[i].Product = "chatgpt"
		}
		cfg.LLMProducts[i].BaseURL = llmProductBaseURL(cfg.LLMProducts[i].Product)
		if cfg.LLMProducts[i].ID == "" {
			cfg.LLMProducts[i].ID = nextIntegrationIDLLMProduct(cfg.LLMProducts[:i])
		}
		if cfg.LLMProducts[i].Name == "" {
			cfg.LLMProducts[i].Name = "未命名智能体"
		}
		if cfg.LLMProducts[i].BaseURL == "" {
			cfg.LLMProducts[i].BaseURL = llmProductBaseURL("chatgpt")
		}
	}
	for i := range cfg.LLMs {
		cfg.LLMs[i].ProductID = ""
		cfg.LLMs[i].Product = normalizeLLMProduct(cfg.LLMs[i].Product)
		if cfg.LLMs[i].Product == "" {
			cfg.LLMs[i].Product = inferLLMProductFromBaseURL(cfg.LLMs[i].BaseURL)
		}
		if err := validateLLMProduct(cfg.LLMs[i].Product); err != nil {
			cfg.LLMs[i].Product = "chatgpt"
		}
		cfg.LLMs[i].BaseURL = llmProductBaseURL(cfg.LLMs[i].Product)
	}
	nextExchanges := make([]exchangeIntegration, 0, len(cfg.Exchanges))
	for i := range cfg.Exchanges {
		cfg.Exchanges[i].ID = strings.TrimSpace(cfg.Exchanges[i].ID)
		if cfg.Exchanges[i].ID == "" {
			cfg.Exchanges[i].ID = nextIntegrationIDExchange(nextExchanges)
		}
		cfg.Exchanges[i].Name = strings.TrimSpace(cfg.Exchanges[i].Name)
		cfg.Exchanges[i].Exchange = strings.ToLower(strings.TrimSpace(cfg.Exchanges[i].Exchange))
		cfg.Exchanges[i].APIKey = strings.TrimSpace(cfg.Exchanges[i].APIKey)
		cfg.Exchanges[i].Secret = strings.TrimSpace(cfg.Exchanges[i].Secret)
		cfg.Exchanges[i].Passphase = strings.TrimSpace(cfg.Exchanges[i].Passphase)
		if cfg.Exchanges[i].Name == "" {
			cfg.Exchanges[i].Name = cfg.Exchanges[i].Exchange
		}
		if cfg.Exchanges[i].Exchange != "binance" && cfg.Exchanges[i].Exchange != "okx" {
			continue
		}
		nextExchanges = append(nextExchanges, cfg.Exchanges[i])
	}
	cfg.Exchanges = nextExchanges
	if strings.TrimSpace(cfg.ActiveExchangeID) != "" {
		if findExchangeByID(cfg.Exchanges, cfg.ActiveExchangeID) == nil {
			cfg.ActiveExchangeID = ""
		}
	}
	if strings.TrimSpace(cfg.ActiveExchangeID) == "" && len(cfg.Exchanges) > 0 {
		cfg.ActiveExchangeID = strings.TrimSpace(cfg.Exchanges[0].ID)
	}
}

func mergeIntegrationsFromEnv(cfg *integrationStore) {
	if cfg == nil {
		return
	}
	if len(cfg.LLMProducts) == 0 {
		cfg.LLMProducts = defaultLLMProducts()
	}
	if len(cfg.LLMs) == 0 {
		baseURL := strings.TrimSpace(os.Getenv("AI_BASE_URL"))
		apiKey := strings.TrimSpace(os.Getenv("AI_API_KEY"))
		model := strings.TrimSpace(os.Getenv("AI_MODEL"))
		product := normalizeLLMProduct(os.Getenv("AI_PRODUCT"))
		if product == "" {
			product = inferLLMProductFromBaseURL(baseURL)
		}
		if err := validateLLMProduct(product); err != nil {
			product = "chatgpt"
		}
		if baseURL != "" || apiKey != "" || model != "" {
			cfg.LLMs = append(cfg.LLMs, llmIntegration{
				ID:      "1",
				Name:    "ENV 智能体",
				Product: product,
				BaseURL: llmProductBaseURL(product),
				APIKey:  apiKey,
				Model:   model,
			})
		}
	}
	if len(cfg.Exchanges) == 0 {
		binanceKey := strings.TrimSpace(os.Getenv("BINANCE_API_KEY"))
		binanceSecret := strings.TrimSpace(os.Getenv("BINANCE_SECRET"))
		if binanceKey != "" || binanceSecret != "" {
			cfg.Exchanges = append(cfg.Exchanges, exchangeIntegration{
				ID:       "1",
				Name:     "ENV Binance",
				Exchange: "binance",
				APIKey:   binanceKey,
				Secret:   binanceSecret,
			})
		}
		okxKey := strings.TrimSpace(os.Getenv("OKX_API_KEY"))
		okxSecret := strings.TrimSpace(os.Getenv("OKX_SECRET"))
		okxPassphrase := strings.TrimSpace(os.Getenv("OKX_PASSWORD"))
		if okxKey != "" || okxSecret != "" || okxPassphrase != "" {
			cfg.Exchanges = append(cfg.Exchanges, exchangeIntegration{
				ID:        strconv.Itoa(len(cfg.Exchanges) + 1),
				Name:      "ENV OKX",
				Exchange:  "okx",
				APIKey:    okxKey,
				Secret:    okxSecret,
				Passphase: okxPassphrase,
			})
		}
	}
	if strings.TrimSpace(cfg.ActiveExchangeID) == "" && len(cfg.Exchanges) > 0 {
		cfg.ActiveExchangeID = strings.TrimSpace(cfg.Exchanges[0].ID)
	}
}

func defaultLLMProducts() []llmProduct {
	catalog := llmProductCatalog()
	out := make([]llmProduct, 0, len(catalog))
	for i, item := range catalog {
		out = append(out, llmProduct{
			ID:      strconv.Itoa(i + 1),
			Name:    item.Name,
			Product: item.Product,
			BaseURL: item.BaseURL,
		})
	}
	if len(out) == 0 {
		out = append(out, llmProduct{
			ID:      "1",
			Name:    "ChatGPT",
			Product: "chatgpt",
			BaseURL: "https://api.openai.com/v1",
		})
	}
	return out
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

func nextIntegrationIDLLMProduct(items []llmProduct) string {
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
	exName := strings.ToLower(strings.TrimSpace(cfg.Exchange))
	updates := map[string]string{
		"ACTIVE_EXCHANGE": exName,
	}
	switch exName {
	case "binance":
		updates["BINANCE_API_KEY"] = strings.TrimSpace(cfg.APIKey)
		updates["BINANCE_SECRET"] = strings.TrimSpace(cfg.Secret)
	case "okx":
		updates["OKX_API_KEY"] = strings.TrimSpace(cfg.APIKey)
		updates["OKX_SECRET"] = strings.TrimSpace(cfg.Secret)
		updates["OKX_PASSWORD"] = strings.TrimSpace(cfg.Passphase)
	default:
		return fmt.Errorf("unsupported exchange: %s", exName)
	}
	if err := upsertDotEnv(".env", updates); err != nil {
		return err
	}
	for k, v := range updates {
		_ = os.Setenv(k, v)
	}
	// 当前执行引擎仍以 Binance API 交易为主；OKX 先用于账户接入与前端行情切换。
	if exName == "okx" {
		return nil
	}
	applyRuntimeConfigFromEnv()
	return s.bot.ReloadClients()
}

func unbindExchangeAccount(s *Service) error {
	updates := map[string]string{
		"ACTIVE_EXCHANGE": "",
		"BINANCE_API_KEY": "",
		"BINANCE_SECRET":  "",
		"OKX_API_KEY":     "",
		"OKX_SECRET":      "",
		"OKX_PASSWORD":    "",
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
