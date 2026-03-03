package server

import (
	"fmt"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"
	"trade-go/config"
)

func (s *Service) handleSystemRuntimeStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	s.mu.RLock()
	startedAt := s.startedAt
	restartCount := s.restartCount
	schedulerRunning := s.schedulerRunning
	realtimeRunning := s.realtimeLoopRunning
	triggerMode := strings.TrimSpace(s.triggerMode)
	nextRunAt := s.nextRunAt
	s.mu.RUnlock()
	if triggerMode == "" {
		triggerMode = "idle"
	}
	engineRunning := schedulerRunning || realtimeRunning
	schedulerMessage := "未启动"
	switch triggerMode {
	case "realtime":
		schedulerMessage = boolStatus(realtimeRunning, "事件驱动已启动", "事件驱动未启动")
	case "scheduler":
		schedulerMessage = boolStatus(schedulerRunning, "已启动", "未启动")
	default:
		schedulerMessage = boolStatus(engineRunning, "已启动", "未启动")
	}

	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	hostname, _ := os.Hostname()

	inte, _ := readIntegrations()
	activeLLM := findLLMByID(inte.LLMs, inte.ActiveLLMID)
	llmConfiguredByEnv := strings.TrimSpace(config.Config.AIAPIKey) != "" && strings.TrimSpace(config.Config.AIBaseURL) != ""
	llmConfigured := activeLLM != nil || llmConfiguredByEnv
	llmModel := strings.TrimSpace(config.Config.AIModel)
	llmStatus := "unconfigured"
	llmMessage := "AI_API_KEY/AI_BASE_URL 未配置"
	llmReachable := false
	llmCheckedAt := ""
	if activeLLM != nil {
		if m := strings.TrimSpace(activeLLM.Model); m != "" {
			llmModel = m
		}
		llmCheckedAt = strings.TrimSpace(activeLLM.CheckedAt)
		switch normalizeLLMReachabilityStatus(activeLLM.Status) {
		case "reachable":
			llmStatus = "connected"
			llmReachable = true
			msg := strings.TrimSpace(activeLLM.Message)
			if msg == "" {
				msg = "智能体可达"
			}
			llmMessage = msg
		case "unreachable":
			llmStatus = "warning"
			msg := strings.TrimSpace(activeLLM.Message)
			if msg == "" {
				msg = "智能体不可达"
			}
			llmMessage = msg
		default:
			llmStatus = "configured"
			llmMessage = "智能体参数已配置（未验证可用性）"
		}
	} else if llmConfiguredByEnv {
		llmStatus = "configured"
		llmMessage = "智能体参数已配置（未验证可用性）"
	}
	if llmModel == "" {
		llmModel = "chat-model"
	}

	activeExchange := findExchangeByID(inte.Exchanges, inte.ActiveExchangeID)
	exchangeBound := activeExchange != nil
	exchangeReady := false
	exchangeMsg := "未绑定交易所账号"
	if exchangeBound {
		if _, err := s.bot.FetchBalance(); err != nil {
			exchangeMsg = "交易所连通异常: " + err.Error()
		} else {
			exchangeReady = true
			exchangeMsg = "交易所账号已连接"
		}
	}

	uptimeSec := int64(0)
	if !startedAt.IsZero() {
		uptimeSec = int64(time.Since(startedAt).Seconds())
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"server": map[string]any{
			"hostname":      hostname,
			"started_at":    startedAt,
			"uptime_sec":    uptimeSec,
			"go_version":    runtime.Version(),
			"restart_count": restartCount,
		},
		"components": []map[string]any{
			{
				"name":    "HTTP API",
				"status":  "running",
				"message": "服务运行中",
			},
			{
				"name":    "调度器",
				"status":  boolStatus(engineRunning, "running", "stopped"),
				"message": schedulerMessage,
			},
			{
				"name":    "SQLite",
				"status":  boolStatus(s.bot.HasStore(), "connected", "disabled"),
				"message": boolStatus(s.bot.HasStore(), "持久化已启用", "持久化未启用"),
			},
			{
				"name":    "交易所连接",
				"status":  boolStatus(exchangeReady, "connected", "warning"),
				"message": exchangeMsg,
			},
			{
				"name":    "智能体连接",
				"status":  llmStatus,
				"message": llmMessage,
			},
		},
		"resources": map[string]any{
			"num_cpu":         runtime.NumCPU(),
			"gomaxprocs":      runtime.GOMAXPROCS(0),
			"goroutines":      runtime.NumGoroutine(),
			"heap_alloc_mb":   float64(mem.HeapAlloc) / 1024.0 / 1024.0,
			"heap_inuse_mb":   float64(mem.HeapInuse) / 1024.0 / 1024.0,
			"stack_inuse_mb":  float64(mem.StackInuse) / 1024.0 / 1024.0,
			"sys_memory_mb":   float64(mem.Sys) / 1024.0 / 1024.0,
			"gc_cycles_total": mem.NumGC,
		},
		"integration": map[string]any{
			"exchange": map[string]any{
				"bound":              exchangeBound,
				"ready":              exchangeReady,
				"active_exchange_id": inte.ActiveExchangeID,
				"exchange": func() string {
					if activeExchange == nil {
						return ""
					}
					return activeExchange.Exchange
				}(),
			},
			"agent": map[string]any{
				"configured":  llmConfigured,
				"reachable":   llmReachable,
				"status":      llmStatus,
				"message":     llmMessage,
				"checked_at":  llmCheckedAt,
				"model":       llmModel,
				"token_usage": getLLMUsageSnapshot(),
			},
		},
		"scheduler": map[string]any{
			"running":     schedulerRunning,
			"mode":        triggerMode,
			"engine_on":   engineRunning,
			"realtime_on": realtimeRunning,
			"next_run_at": nextRunAt,
		},
	})
}

func (s *Service) handleSystemSoftRestart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	s.mu.RLock()
	wasRunning := s.schedulerRunning
	s.mu.RUnlock()

	// Ensure no run cycle is executing while clients are reloaded.
	s.runMu.Lock()
	defer s.runMu.Unlock()

	s.StopScheduler()
	if err := s.bot.ReloadClients(); err != nil {
		if wasRunning {
			s.StartScheduler()
		}
		msg := "后台软重启失败: " + err.Error()
		fmt.Println(msg)
		writeError(w, http.StatusInternalServerError, msg)
		return
	}
	if wasRunning {
		s.StartScheduler()
	}
	runtime.GC()

	now := time.Now()
	s.mu.Lock()
	s.restartCount++
	count := s.restartCount
	s.mu.Unlock()

	writeJSON(w, http.StatusOK, map[string]any{
		"message":           "后台软重启完成（已重载交易所/智能体客户端）",
		"restarted_at":      now,
		"restart_count":     count,
		"scheduler_running": wasRunning,
	})
}

func boolStatus(v bool, t, f string) string {
	if v {
		return t
	}
	return f
}
