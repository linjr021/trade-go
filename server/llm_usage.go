package server

import (
	"math"
	"strings"
	"sync"
	"time"
)

type llmUsageChannel struct {
	Requests         int64  `json:"requests"`
	PromptTokens     int64  `json:"prompt_tokens"`
	CompletionTokens int64  `json:"completion_tokens"`
	TotalTokens      int64  `json:"total_tokens"`
	LastUsedAt       string `json:"last_used_at"`
}

type llmUsageState struct {
	Requests         int64
	PromptTokens     int64
	CompletionTokens int64
	TotalTokens      int64
	LastUsedAt       time.Time
	ByChannel        map[string]*llmUsageChannel
	NextID           int64
	Entries          []llmUsageLogEntry
}

type llmUsageLogEntry struct {
	ID               int64  `json:"id"`
	Channel          string `json:"channel"`
	Model            string `json:"model"`
	Prompt           string `json:"prompt"`
	Completion       string `json:"completion"`
	PromptTokens     int64  `json:"prompt_tokens"`
	CompletionTokens int64  `json:"completion_tokens"`
	TotalTokens      int64  `json:"total_tokens"`
	CreatedAt        string `json:"created_at"`
}

var (
	llmUsageMu sync.RWMutex
	llmUsage   = llmUsageState{ByChannel: map[string]*llmUsageChannel{}}
)

const (
	llmUsageMaxEntries = 500
	llmUsageTextLimit  = 12000
)

func estimateTokens(text string) int64 {
	s := strings.TrimSpace(text)
	if s == "" {
		return 0
	}
	runes := int64(len([]rune(s)))
	byRune := int64(math.Ceil(float64(runes) / 4.0))
	byWord := int64(len(strings.Fields(s)))
	if byRune > byWord {
		return byRune
	}
	return byWord
}

func recordLLMUsageWithMeta(channel, model, prompt, completion string) {
	ch := strings.TrimSpace(channel)
	if ch == "" {
		ch = "default"
	}
	m := strings.TrimSpace(model)
	promptTokens := estimateTokens(prompt)
	completionTokens := estimateTokens(completion)
	total := promptTokens + completionTokens
	now := time.Now()

	llmUsageMu.Lock()
	defer llmUsageMu.Unlock()

	llmUsage.Requests++
	llmUsage.PromptTokens += promptTokens
	llmUsage.CompletionTokens += completionTokens
	llmUsage.TotalTokens += total
	llmUsage.LastUsedAt = now

	item, ok := llmUsage.ByChannel[ch]
	if !ok {
		item = &llmUsageChannel{}
		llmUsage.ByChannel[ch] = item
	}
	item.Requests++
	item.PromptTokens += promptTokens
	item.CompletionTokens += completionTokens
	item.TotalTokens += total
	item.LastUsedAt = now.Format(time.RFC3339)

	llmUsage.NextID++
	entry := llmUsageLogEntry{
		ID:               llmUsage.NextID,
		Channel:          ch,
		Model:            m,
		Prompt:           clipForLog(prompt, llmUsageTextLimit),
		Completion:       clipForLog(completion, llmUsageTextLimit),
		PromptTokens:     promptTokens,
		CompletionTokens: completionTokens,
		TotalTokens:      total,
		CreatedAt:        now.Format(time.RFC3339),
	}
	llmUsage.Entries = append(llmUsage.Entries, entry)
	if len(llmUsage.Entries) > llmUsageMaxEntries {
		llmUsage.Entries = llmUsage.Entries[len(llmUsage.Entries)-llmUsageMaxEntries:]
	}
}

func getLLMUsageSnapshot() map[string]any {
	llmUsageMu.RLock()
	defer llmUsageMu.RUnlock()

	byChannel := map[string]any{}
	for k, v := range llmUsage.ByChannel {
		byChannel[k] = map[string]any{
			"requests":          v.Requests,
			"prompt_tokens":     v.PromptTokens,
			"completion_tokens": v.CompletionTokens,
			"total_tokens":      v.TotalTokens,
			"last_used_at":      v.LastUsedAt,
		}
	}

	lastUsedAt := ""
	if !llmUsage.LastUsedAt.IsZero() {
		lastUsedAt = llmUsage.LastUsedAt.Format(time.RFC3339)
	}
	return map[string]any{
		"requests":          llmUsage.Requests,
		"prompt_tokens":     llmUsage.PromptTokens,
		"completion_tokens": llmUsage.CompletionTokens,
		"total_tokens":      llmUsage.TotalTokens,
		"last_used_at":      lastUsedAt,
		"by_channel":        byChannel,
		"log_size":          len(llmUsage.Entries),
	}
}

func getLLMUsageLogs(limit int, channel string) []llmUsageLogEntry {
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}
	ch := strings.TrimSpace(channel)

	llmUsageMu.RLock()
	defer llmUsageMu.RUnlock()

	out := make([]llmUsageLogEntry, 0, limit)
	for i := len(llmUsage.Entries) - 1; i >= 0; i-- {
		it := llmUsage.Entries[i]
		if ch != "" && it.Channel != ch {
			continue
		}
		out = append(out, it)
		if len(out) >= limit {
			break
		}
	}
	return out
}

func clipForLog(text string, max int) string {
	s := strings.TrimSpace(text)
	if s == "" || max <= 0 {
		return s
	}
	rs := []rune(s)
	if len(rs) <= max {
		return s
	}
	return string(rs[:max]) + "...(truncated)"
}
