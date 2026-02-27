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
}

var (
	llmUsageMu sync.RWMutex
	llmUsage   = llmUsageState{ByChannel: map[string]*llmUsageChannel{}}
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

func recordLLMUsage(channel, prompt, completion string) {
	ch := strings.TrimSpace(channel)
	if ch == "" {
		ch = "default"
	}
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
	}
}
