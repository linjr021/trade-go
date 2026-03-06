package server

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
	"trade-go/storage"
)

const authSessionTTL = 24 * time.Hour

type authPrincipal struct {
	UserID                int64             `json:"user_id"`
	Username              string            `json:"username"`
	RoleID                int64             `json:"role_id"`
	RoleName              string            `json:"role_name"`
	IsSuper               bool              `json:"is_super"`
	MustChangeCredentials bool              `json:"must_change_credentials"`
	Permissions           map[string]string `json:"permissions"`
}

type authSession struct {
	Token     string
	Principal authPrincipal
	CreatedAt time.Time
	ExpiresAt time.Time
}

type authPermissionPolicy struct {
	Public bool
	Module string
	Need   string
}

type authContextKey string

const authPrincipalCtxKey authContextKey = "auth_principal"

func (s *Service) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimSpace(r.URL.Path)
		if !strings.HasPrefix(path, "/api/") || r.Method == http.MethodOptions {
			next.ServeHTTP(w, r)
			return
		}

		policy := resolveAuthPermissionPolicy(path, r.Method)
		if policy.Public {
			next.ServeHTTP(w, r)
			return
		}

		token := parseBearerToken(r)
		if token == "" {
			writeError(w, http.StatusUnauthorized, "missing auth token")
			return
		}
		principal, ok := s.lookupSession(token)
		if !ok {
			writeError(w, http.StatusUnauthorized, "invalid or expired auth token")
			return
		}
		if principal.MustChangeCredentials {
			if path != "/api/auth/me" && path != "/api/auth/logout" && path != "/api/auth/change-credentials" {
				writeError(w, http.StatusPreconditionRequired, "must change credentials on first login")
				return
			}
		}
		if policy.Need != "" && policy.Module != "" && !principal.IsSuper {
			if !storage.CanAccess(principal.Permissions, policy.Module, policy.Need) {
				_ = s.saveAuthAudit(r, principal, "permission_denied", policy.Module, path, "denied", map[string]any{
					"need":      policy.Need,
					"have":      principal.Permissions[policy.Module],
					"http_path": path,
					"method":    r.Method,
				})
				writeError(w, http.StatusForbidden, "permission denied")
				return
			}
		}
		ctx := context.WithValue(r.Context(), authPrincipalCtxKey, principal)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func resolveAuthPermissionPolicy(path, method string) authPermissionPolicy {
	// public
	if path == "/api/auth/login" && method == http.MethodPost {
		return authPermissionPolicy{Public: true}
	}
	if path == "/api/auth/bootstrap-status" && method == http.MethodGet {
		return authPermissionPolicy{Public: true}
	}
	if path == "/api/auth/bootstrap-admin" && method == http.MethodPost {
		return authPermissionPolicy{Public: true}
	}

	// auth-admin endpoints
	if strings.HasPrefix(path, "/api/auth/") {
		switch path {
		case "/api/auth/me", "/api/auth/logout", "/api/auth/change-credentials":
			return authPermissionPolicy{}
		case "/api/auth/audit-logs", "/api/auth/users", "/api/auth/roles":
			if method == http.MethodGet {
				return authPermissionPolicy{Module: "auth_admin", Need: storage.AccessRead}
			}
			return authPermissionPolicy{Module: "auth_admin", Need: storage.AccessEdit}
		case "/api/auth/users/role", "/api/auth/users/password", "/api/auth/roles/update",
			"/api/auth/users/delete", "/api/auth/roles/delete":
			return authPermissionPolicy{Module: "auth_admin", Need: storage.AccessEdit}
		default:
			return authPermissionPolicy{Module: "auth_admin", Need: storage.AccessEdit}
		}
	}

	// read endpoints only need login
	if method == http.MethodGet {
		return authPermissionPolicy{}
	}

	// write endpoints -> module edit
	switch path {
	case "/api/strategy-preference/generate", "/api/generated-strategies":
		return authPermissionPolicy{Module: "builder", Need: storage.AccessEdit}
	case "/api/skill-workflow", "/api/auto-strategy/regen-now", "/api/risk/reset":
		return authPermissionPolicy{Module: "skill_workflow", Need: storage.AccessEdit}
	case "/api/backtest", "/api/backtest-history/delete":
		return authPermissionPolicy{Module: "backtest", Need: storage.AccessEdit}
	case "/api/system-settings", "/api/system/restart",
		"/api/integrations", "/api/integrations/llm", "/api/integrations/llm-product",
		"/api/integrations/llm-product/update", "/api/integrations/llm-product/delete",
		"/api/integrations/llm/test", "/api/integrations/llm/models", "/api/integrations/llm/update", "/api/integrations/llm/delete", "/api/integrations/llm/activate",
		"/api/integrations/exchange", "/api/integrations/exchange/activate", "/api/integrations/exchange/delete":
		return authPermissionPolicy{Module: "system", Need: storage.AccessEdit}
	case "/api/settings", "/api/run", "/api/scheduler/start", "/api/scheduler/stop":
		return authPermissionPolicy{Module: "live", Need: storage.AccessEdit}
	case "/api/paper/simulate-step", "/api/paper/config", "/api/paper/start", "/api/paper/stop", "/api/paper/reset-pnl", "/api/paper/risk/reset":
		return authPermissionPolicy{Module: "paper", Need: storage.AccessEdit}
	default:
		// fallback: authenticated only
		return authPermissionPolicy{}
	}
}

func parseBearerToken(r *http.Request) string {
	raw := strings.TrimSpace(r.Header.Get("Authorization"))
	if raw == "" {
		return ""
	}
	parts := strings.SplitN(raw, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(strings.TrimSpace(parts[0]), "bearer") {
		return ""
	}
	return strings.TrimSpace(parts[1])
}

func (s *Service) newSession(principal authPrincipal) (string, error) {
	token, err := storage.BuildSessionToken()
	if err != nil {
		return "", err
	}
	now := time.Now()
	sess := authSession{
		Token:     token,
		Principal: principal,
		CreatedAt: now,
		ExpiresAt: now.Add(authSessionTTL),
	}
	s.authMu.Lock()
	s.sessions[token] = sess
	s.authMu.Unlock()
	return token, nil
}

func (s *Service) lookupSession(token string) (authPrincipal, bool) {
	s.authMu.RLock()
	sess, ok := s.sessions[token]
	s.authMu.RUnlock()
	if !ok {
		return authPrincipal{}, false
	}
	if time.Now().After(sess.ExpiresAt) {
		s.authMu.Lock()
		delete(s.sessions, token)
		s.authMu.Unlock()
		return authPrincipal{}, false
	}
	return sess.Principal, true
}

func (s *Service) dropSession(token string) {
	if token == "" {
		return
	}
	s.authMu.Lock()
	delete(s.sessions, token)
	s.authMu.Unlock()
}

func principalFromRequest(r *http.Request) (authPrincipal, bool) {
	v := r.Context().Value(authPrincipalCtxKey)
	if v == nil {
		return authPrincipal{}, false
	}
	p, ok := v.(authPrincipal)
	return p, ok
}

func (s *Service) handleAuthLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if s.db == nil {
		writeError(w, http.StatusServiceUnavailable, "auth storage unavailable")
		return
	}
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	username := strings.TrimSpace(req.Username)
	user, ok, err := s.db.AuthenticateUser(username, req.Password)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "login failed: "+err.Error())
		return
	}
	if !ok {
		_ = s.saveAuthAudit(r, authPrincipal{Username: username}, "login", "system", username, "failed", map[string]any{"reason": "invalid_credentials"})
		writeError(w, http.StatusUnauthorized, "username or password is invalid")
		return
	}

	principal := authPrincipal{
		UserID:                user.ID,
		Username:              user.Username,
		RoleID:                user.RoleID,
		RoleName:              user.RoleName,
		IsSuper:               user.IsSuper,
		MustChangeCredentials: user.MustChangeCredentials,
		Permissions:           storage.MergePermissionsForResponse(user.Permissions),
	}
	token, err := s.newSession(principal)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "create session failed")
		return
	}

	_ = s.saveAuthAudit(r, principal, "login", "system", user.Username, "ok", map[string]any{"role": user.RoleName})
	writeJSON(w, http.StatusOK, map[string]any{
		"token": token,
		"user":  principal,
	})
}

func (s *Service) handleAuthBootstrapStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if s.db == nil {
		writeError(w, http.StatusServiceUnavailable, "auth storage unavailable")
		return
	}
	needs, err := s.db.AdminNeedsBootstrap()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "bootstrap status failed: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"needs_bootstrap": needs,
	})
}

func (s *Service) handleAuthBootstrapAdmin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if s.db == nil {
		writeError(w, http.StatusServiceUnavailable, "auth storage unavailable")
		return
	}
	var req struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	user, err := s.db.BootstrapAdminPassword(req.Password)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	principal := authPrincipal{
		UserID:                user.ID,
		Username:              user.Username,
		RoleID:                user.RoleID,
		RoleName:              user.RoleName,
		IsSuper:               user.IsSuper,
		MustChangeCredentials: user.MustChangeCredentials,
		Permissions:           storage.MergePermissionsForResponse(user.Permissions),
	}
	token, err := s.newSession(principal)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "create session failed")
		return
	}
	_ = s.saveAuthAudit(r, principal, "bootstrap_admin_credentials", "system", user.Username, "ok", nil)
	writeJSON(w, http.StatusOK, map[string]any{
		"message":         "admin bootstrapped",
		"needs_bootstrap": false,
		"token":           token,
		"user":            principal,
	})
}

func (s *Service) handleAuthLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	token := parseBearerToken(r)
	p, _ := principalFromRequest(r)
	s.dropSession(token)
	_ = s.saveAuthAudit(r, p, "logout", "system", p.Username, "ok", nil)
	writeJSON(w, http.StatusOK, map[string]any{"message": "logout success"})
}

func (s *Service) handleAuthMe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	p, ok := principalFromRequest(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"user": p})
}

func (s *Service) handleAuthChangeCredentials(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if s.db == nil {
		writeError(w, http.StatusServiceUnavailable, "auth storage unavailable")
		return
	}
	principal, ok := principalFromRequest(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var req struct {
		CurrentPassword string `json:"current_password"`
		NewUsername     string `json:"new_username"`
		NewPassword     string `json:"new_password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	user, err := s.db.ChangeOwnCredentials(principal.UserID, req.CurrentPassword, req.NewUsername, req.NewPassword)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	next := authPrincipal{
		UserID:                user.ID,
		Username:              user.Username,
		RoleID:                user.RoleID,
		RoleName:              user.RoleName,
		IsSuper:               user.IsSuper,
		MustChangeCredentials: user.MustChangeCredentials,
		Permissions:           storage.MergePermissionsForResponse(user.Permissions),
	}
	token := parseBearerToken(r)
	s.replaceSessionPrincipal(token, next)
	_ = s.saveAuthAudit(r, principal, "change_credentials", "system", user.Username, "ok", map[string]any{
		"from_username": principal.Username,
		"to_username":   user.Username,
	})
	writeJSON(w, http.StatusOK, map[string]any{
		"message": "credentials updated",
		"user":    next,
	})
}

func (s *Service) handleAuthRoles(w http.ResponseWriter, r *http.Request) {
	if s.db == nil {
		writeError(w, http.StatusServiceUnavailable, "auth storage unavailable")
		return
	}
	switch r.Method {
	case http.MethodGet:
		roles, err := s.db.ListRoles()
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		for i := range roles {
			roles[i].Permissions = storage.MergePermissionsForResponse(roles[i].Permissions)
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"modules": storage.PermissionModules(),
			"roles":   roles,
		})
	case http.MethodPost:
		var req struct {
			Name        string            `json:"name"`
			IsSuper     bool              `json:"is_super"`
			Permissions map[string]string `json:"permissions"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid json body")
			return
		}
		role, err := s.db.CreateRole(req.Name, req.Permissions, req.IsSuper)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		p, _ := principalFromRequest(r)
		_ = s.saveAuthAudit(r, p, "role_create", "system", role.Name, "ok", role)
		writeJSON(w, http.StatusOK, map[string]any{"role": role})
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Service) handleAuthRoleUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if s.db == nil {
		writeError(w, http.StatusServiceUnavailable, "auth storage unavailable")
		return
	}
	var req struct {
		ID          int64             `json:"id"`
		Name        string            `json:"name"`
		Permissions map[string]string `json:"permissions"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	role, err := s.db.UpdateRole(req.ID, req.Name, req.Permissions)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	p, _ := principalFromRequest(r)
	_ = s.saveAuthAudit(r, p, "role_update", "system", role.Name, "ok", role)
	writeJSON(w, http.StatusOK, map[string]any{"role": role})
}

func (s *Service) handleAuthUsers(w http.ResponseWriter, r *http.Request) {
	if s.db == nil {
		writeError(w, http.StatusServiceUnavailable, "auth storage unavailable")
		return
	}
	switch r.Method {
	case http.MethodGet:
		users, err := s.db.ListUsers()
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		for i := range users {
			users[i].Permissions = storage.MergePermissionsForResponse(users[i].Permissions)
		}
		writeJSON(w, http.StatusOK, map[string]any{"users": users})
	case http.MethodPost:
		var req struct {
			Username string `json:"username"`
			Password string `json:"password"`
			RoleID   int64  `json:"role_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid json body")
			return
		}
		user, err := s.db.CreateUser(req.Username, req.Password, req.RoleID)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		p, _ := principalFromRequest(r)
		_ = s.saveAuthAudit(r, p, "user_create", "system", user.Username, "ok", map[string]any{"role": user.RoleName})
		writeJSON(w, http.StatusOK, map[string]any{"user": user})
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Service) handleAuthUserRoleUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if s.db == nil {
		writeError(w, http.StatusServiceUnavailable, "auth storage unavailable")
		return
	}
	var req struct {
		UserID int64 `json:"user_id"`
		RoleID int64 `json:"role_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	if err := s.db.UpdateUserRole(req.UserID, req.RoleID); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	user, err := s.db.GetUserByID(req.UserID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	p, _ := principalFromRequest(r)
	_ = s.saveAuthAudit(r, p, "user_role_update", "system", user.Username, "ok", map[string]any{"role": user.RoleName})
	writeJSON(w, http.StatusOK, map[string]any{"user": user})
}

func (s *Service) handleAuthUserPasswordUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if s.db == nil {
		writeError(w, http.StatusServiceUnavailable, "auth storage unavailable")
		return
	}
	var req struct {
		UserID   int64  `json:"user_id"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	if err := s.db.UpdateUserPassword(req.UserID, req.Password, true); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	user, err := s.db.GetUserByID(req.UserID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	p, _ := principalFromRequest(r)
	_ = s.saveAuthAudit(r, p, "user_password_update", "system", user.Username, "ok", nil)
	writeJSON(w, http.StatusOK, map[string]any{"message": "password updated"})
}

func (s *Service) handleAuthUserDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if s.db == nil {
		writeError(w, http.StatusServiceUnavailable, "auth storage unavailable")
		return
	}
	var req struct {
		UserID int64 `json:"user_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	principal, _ := principalFromRequest(r)
	if req.UserID <= 0 {
		writeError(w, http.StatusBadRequest, "用户无效")
		return
	}
	if principal.UserID == req.UserID {
		writeError(w, http.StatusBadRequest, "不可删除当前登录用户")
		return
	}
	target, err := s.db.GetUserByID(req.UserID)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.db.DeleteUser(req.UserID); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.dropSessionsByUserID(req.UserID)
	_ = s.saveAuthAudit(r, principal, "user_delete", "system", target.Username, "ok", map[string]any{
		"user_id": req.UserID,
	})
	writeJSON(w, http.StatusOK, map[string]any{"message": "user deleted"})
}

func (s *Service) handleAuthRoleDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if s.db == nil {
		writeError(w, http.StatusServiceUnavailable, "auth storage unavailable")
		return
	}
	var req struct {
		RoleID int64 `json:"role_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	if req.RoleID <= 0 {
		writeError(w, http.StatusBadRequest, "权限组无效")
		return
	}
	role, err := s.db.GetRoleByID(req.RoleID)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.db.DeleteRole(req.RoleID); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	principal, _ := principalFromRequest(r)
	_ = s.saveAuthAudit(r, principal, "role_delete", "system", role.Name, "ok", map[string]any{
		"role_id": req.RoleID,
	})
	writeJSON(w, http.StatusOK, map[string]any{"message": "role deleted"})
}

func (s *Service) handleAuthAuditLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if s.db == nil {
		writeError(w, http.StatusServiceUnavailable, "auth storage unavailable")
		return
	}
	limit := 200
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			limit = n
		}
	}
	logs, err := s.db.ListAuthAuditLogs(limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"logs": logs})
}

func (s *Service) saveAuthAudit(r *http.Request, principal authPrincipal, action, module, target, result string, details any) error {
	if s == nil || s.db == nil {
		return nil
	}
	ip, _, _ := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if ip == "" {
		ip = strings.TrimSpace(r.RemoteAddr)
	}
	return s.db.SaveAuthAuditLog(storage.AuthAuditLog{
		Ts:        time.Now().Format(time.RFC3339),
		UserID:    principal.UserID,
		Username:  principal.Username,
		Action:    action,
		Module:    module,
		Target:    target,
		Result:    result,
		IP:        ip,
		UserAgent: r.UserAgent(),
		Details:   storage.EncodeAuthAuditDetails(details),
	})
}

func (s *Service) replaceSessionPrincipal(token string, principal authPrincipal) {
	token = strings.TrimSpace(token)
	if token == "" {
		return
	}
	s.authMu.Lock()
	defer s.authMu.Unlock()
	sess, ok := s.sessions[token]
	if !ok {
		return
	}
	sess.Principal = principal
	s.sessions[token] = sess
}

func (s *Service) dropSessionsByUserID(userID int64) {
	if userID <= 0 {
		return
	}
	s.authMu.Lock()
	defer s.authMu.Unlock()
	for token, sess := range s.sessions {
		if sess.Principal.UserID == userID {
			delete(s.sessions, token)
		}
	}
}
