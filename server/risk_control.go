package server

import (
	"encoding/json"
	"net/http"
	"strings"
)

type riskManualResetRequest struct {
	Reason string `json:"reason"`
}

func (s *Service) handleRiskManualReset(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if s.db == nil {
		writeError(w, http.StatusInternalServerError, "数据库未初始化")
		return
	}

	var req riskManualResetRequest
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&req)
	}

	reason := strings.TrimSpace(req.Reason)
	if reason == "" {
		reason = "manual_clear"
	}
	principal, _ := principalFromRequest(r)
	operator := strings.TrimSpace(principal.Username)
	if operator == "" {
		operator = "unknown"
	}

	resetAt, err := s.db.ResetRiskBaseline(operator, reason)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "重置风控基线失败: "+err.Error())
		return
	}
	payload, _ := json.Marshal(map[string]any{
		"operator": operator,
		"reason":   reason,
		"reset_at": resetAt,
		"exchange": s.bot.ActiveExchange(),
	})
	_ = s.db.SaveRiskEvent("risk_manual_reset", string(payload))
	_ = s.saveAuthAudit(r, principal, "risk_manual_reset", "skill_workflow", s.bot.ActiveExchange(), "ok", map[string]any{
		"reason":   reason,
		"reset_at": resetAt,
		"exchange": s.bot.ActiveExchange(),
	})

	writeJSON(w, http.StatusOK, map[string]any{
		"message":  "风险基线已重置",
		"reset_at": resetAt,
		"exchange": s.bot.ActiveExchange(),
	})
}
