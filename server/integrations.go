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
	LLMs      []llmIntegration      `json:"llms"`
	Exchanges []exchangeIntegration `json:"exchanges"`
}

func (s *Service) handleIntegrations(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		cfg, _ := readIntegrations()
		writeJSON(w, 200, cfg)
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
	if err := writeIntegrations(store); err != nil {
		writeError(w, 500, "保存失败: "+err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"added": req, "exchanges": store.Exchanges})
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

func filterExchangeByID(in []exchangeIntegration, id string) []exchangeIntegration {
	out := make([]exchangeIntegration, 0, len(in))
	for _, x := range in {
		if x.ID != id {
			out = append(out, x)
		}
	}
	return out
}

func validateLLMIntegration(cfg llmIntegration) error {
	u, err := url.Parse(cfg.BaseURL)
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
	req, err := http.NewRequest(http.MethodPost, cfg.BaseURL, bytes.NewReader(raw))
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
	raw, err := os.ReadFile(integrationsPath)
	if err != nil {
		return cfg, err
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return cfg, err
	}
	return cfg, nil
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
