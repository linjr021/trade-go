package storage

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	AccessNone = "none"
	AccessRead = "read"
	AccessEdit = "edit"
)

var (
	roleNameRegexp  = regexp.MustCompile(`^[\p{Han}A-Za-z0-9_\-]{2,24}$`)
	userNameRegexp  = regexp.MustCompile(`^[A-Za-z0-9]{5,10}$`)
	lowerRegexp     = regexp.MustCompile(`[a-z]`)
	upperRegexp     = regexp.MustCompile(`[A-Z]`)
	numberRegexp    = regexp.MustCompile(`[0-9]`)
	symbolRegexp    = regexp.MustCompile(`[^A-Za-z0-9]`)
	permissionMods  = []string{"assets", "live", "paper", "skill_workflow", "builder", "backtest", "auth_admin", "system"}
	permissionLevel = map[string]int{
		AccessNone: 0,
		AccessRead: 1,
		AccessEdit: 2,
	}
)

type AuthRole struct {
	ID          int64             `json:"id"`
	Name        string            `json:"name"`
	IsSuper     bool              `json:"is_super"`
	BuiltIn     bool              `json:"built_in"`
	Permissions map[string]string `json:"permissions"`
	CreatedAt   string            `json:"created_at"`
	UpdatedAt   string            `json:"updated_at"`
}

type AuthUser struct {
	ID                    int64             `json:"id"`
	Username              string            `json:"username"`
	RoleID                int64             `json:"role_id"`
	RoleName              string            `json:"role_name"`
	IsSuper               bool              `json:"is_super"`
	Active                bool              `json:"active"`
	BuiltIn               bool              `json:"built_in"`
	MustChangeCredentials bool              `json:"must_change_credentials"`
	LastLoginAt           string            `json:"last_login_at"`
	CreatedAt             string            `json:"created_at"`
	UpdatedAt             string            `json:"updated_at"`
	Permissions           map[string]string `json:"permissions"`
}

type AuthAuditLog struct {
	ID        int64  `json:"id"`
	Ts        string `json:"ts"`
	UserID    int64  `json:"user_id"`
	Username  string `json:"username"`
	Action    string `json:"action"`
	Module    string `json:"module"`
	Target    string `json:"target"`
	Result    string `json:"result"`
	IP        string `json:"ip"`
	UserAgent string `json:"user_agent"`
	Details   string `json:"details"`
}

type authUserRow struct {
	AuthUser
	PasswordHash string
}

func PermissionModules() []string {
	out := make([]string, 0, len(permissionMods))
	out = append(out, permissionMods...)
	return out
}

func normalizeAccess(raw string) string {
	v := strings.ToLower(strings.TrimSpace(raw))
	if _, ok := permissionLevel[v]; ok {
		return v
	}
	return AccessNone
}

func normalizePermissions(in map[string]string) map[string]string {
	out := map[string]string{}
	for _, mod := range permissionMods {
		out[mod] = AccessNone
	}
	for k, v := range in {
		key := strings.TrimSpace(k)
		if key == "" {
			continue
		}
		out[key] = normalizeAccess(v)
	}
	return out
}

func (s *Store) migrateAuth() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS role_groups (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL UNIQUE,
			is_super INTEGER NOT NULL DEFAULT 0,
			built_in INTEGER NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS role_permissions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			role_id INTEGER NOT NULL,
			module TEXT NOT NULL,
			access TEXT NOT NULL,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			UNIQUE(role_id, module),
			FOREIGN KEY(role_id) REFERENCES role_groups(id) ON DELETE CASCADE
		);`,
		`CREATE TABLE IF NOT EXISTS users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			username TEXT NOT NULL UNIQUE,
			password_hash TEXT NOT NULL,
			role_id INTEGER NOT NULL,
			active INTEGER NOT NULL DEFAULT 1,
			built_in INTEGER NOT NULL DEFAULT 0,
			must_change_credentials INTEGER NOT NULL DEFAULT 0,
			last_login_at TEXT,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			FOREIGN KEY(role_id) REFERENCES role_groups(id)
		);`,
		`CREATE TABLE IF NOT EXISTS auth_audit_logs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			ts TEXT NOT NULL,
			user_id INTEGER,
			username TEXT,
			action TEXT NOT NULL,
			module TEXT,
			target TEXT,
			result TEXT,
			ip TEXT,
			user_agent TEXT,
			details TEXT
		);`,
		`CREATE INDEX IF NOT EXISTS idx_auth_audit_logs_ts ON auth_audit_logs(ts);`,
		`CREATE INDEX IF NOT EXISTS idx_auth_audit_logs_user_ts ON auth_audit_logs(user_id, ts);`,
	}
	for _, stmt := range stmts {
		if _, err := s.db.Exec(stmt); err != nil {
			return err
		}
	}
	compat := []string{
		`ALTER TABLE users ADD COLUMN built_in INTEGER NOT NULL DEFAULT 0;`,
		`ALTER TABLE users ADD COLUMN must_change_credentials INTEGER NOT NULL DEFAULT 0;`,
	}
	for _, stmt := range compat {
		if _, err := s.db.Exec(stmt); err != nil {
			msg := strings.ToLower(err.Error())
			if strings.Contains(msg, "duplicate column") || strings.Contains(msg, "already exists") {
				continue
			}
			return err
		}
	}
	return s.ensureDefaultAuthData()
}

func (s *Store) ensureDefaultAuthData() error {
	superRole, created, err := s.ensureDefaultSuperRole()
	if err != nil {
		return err
	}
	if created {
		_ = s.SaveAuthAuditLog(AuthAuditLog{
			Ts:      time.Now().Format(time.RFC3339),
			Action:  "bootstrap_role",
			Module:  "system",
			Target:  "super_admin",
			Result:  "ok",
			Details: `{"message":"default super role created"}`,
		})
	}

	row, err := s.getUserByUsernameRaw("admin")
	if err == nil && row.ID > 0 {
		_, _ = s.db.Exec(`UPDATE users SET built_in=1 WHERE id=?`, row.ID)
		if strings.TrimSpace(row.PasswordHash) == "" {
			initialPassword := defaultAdminInitialPassword()
			hash, hErr := hashPassword(initialPassword)
			if hErr != nil {
				return hErr
			}
			now := time.Now().Format(time.RFC3339)
			if _, uErr := s.db.Exec(
				`UPDATE users
				 SET password_hash=?, must_change_credentials=1, active=1, updated_at=?
				 WHERE id=?`,
				hash, now, row.ID,
			); uErr != nil {
				return uErr
			}
			fmt.Println("管理员账号已设置初始密码（username=admin），首次登录后需修改密码")
		}
		return nil
	}
	if err != nil && err != sql.ErrNoRows {
		return err
	}

	initialPassword := defaultAdminInitialPassword()
	hash, err := hashPassword(initialPassword)
	if err != nil {
		return err
	}
	now := time.Now().Format(time.RFC3339)
	if _, err := s.db.Exec(
		`INSERT INTO users (username, password_hash, role_id, active, built_in, must_change_credentials, created_at, updated_at)
		 VALUES (?, ?, ?, 1, 1, 1, ?, ?)`,
		"admin", hash, superRole.ID, now, now,
	); err != nil {
		return err
	}
	fmt.Println("初始管理员已创建: username=admin，password=admin（首次登录必须修改密码）")
	_ = s.SaveAuthAuditLog(AuthAuditLog{
		Ts:       now,
		Action:   "bootstrap_admin",
		Module:   "system",
		Target:   "admin",
		Result:   "ok",
		Username: "system",
		Details:  `{"message":"initial admin created"}`,
	})
	return nil
}

func defaultAdminInitialPassword() string {
	if raw := strings.TrimSpace(os.Getenv("ADMIN_INITIAL_PASSWORD")); raw != "" {
		return raw
	}
	return "admin"
}

func (s *Store) AdminNeedsBootstrap() (bool, error) {
	if s == nil || s.db == nil {
		return false, nil
	}
	var hash string
	err := s.db.QueryRow(`SELECT COALESCE(password_hash,'') FROM users WHERE username='admin' LIMIT 1`).Scan(&hash)
	if err == sql.ErrNoRows {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(hash) == "", nil
}

func (s *Store) BootstrapAdminPassword(newPassword string) (AuthUser, error) {
	if err := ValidatePassword(newPassword); err != nil {
		return AuthUser{}, err
	}
	row := s.db.QueryRow(`SELECT id, COALESCE(password_hash,'') FROM users WHERE username='admin' LIMIT 1`)
	var userID int64
	var oldHash string
	if err := row.Scan(&userID, &oldHash); err != nil {
		if err == sql.ErrNoRows {
			return AuthUser{}, fmt.Errorf("管理员账号不存在")
		}
		return AuthUser{}, err
	}
	if strings.TrimSpace(oldHash) != "" {
		return AuthUser{}, fmt.Errorf("管理员已初始化，不能重复设置")
	}
	hash, err := hashPassword(newPassword)
	if err != nil {
		return AuthUser{}, err
	}
	now := time.Now().Format(time.RFC3339)
	if _, err := s.db.Exec(
		`UPDATE users
		 SET password_hash=?, must_change_credentials=0, active=1, built_in=1, updated_at=?
		 WHERE id=?`,
		hash, now, userID,
	); err != nil {
		return AuthUser{}, err
	}
	return s.GetUserByID(userID)
}

func (s *Store) ensureDefaultSuperRole() (AuthRole, bool, error) {
	row := s.db.QueryRow(
		`SELECT id, name, is_super, built_in, created_at, updated_at FROM role_groups WHERE name='super_admin' LIMIT 1`,
	)
	var role AuthRole
	var isSuper, builtIn int
	if err := row.Scan(&role.ID, &role.Name, &isSuper, &builtIn, &role.CreatedAt, &role.UpdatedAt); err == nil {
		role.IsSuper = isSuper == 1
		role.BuiltIn = builtIn == 1
		if role.IsSuper {
			perms, pErr := s.ensureSuperRolePermissions(role.ID)
			if pErr != nil {
				return AuthRole{}, false, pErr
			}
			role.Permissions = perms
		} else {
			perms, pErr := s.getRolePermissions(role.ID)
			if pErr != nil {
				return AuthRole{}, false, pErr
			}
			role.Permissions = perms
		}
		return role, false, nil
	} else if err != sql.ErrNoRows {
		return AuthRole{}, false, err
	}

	now := time.Now().Format(time.RFC3339)
	res, err := s.db.Exec(
		`INSERT INTO role_groups (name, is_super, built_in, created_at, updated_at) VALUES ('super_admin', 1, 1, ?, ?)`,
		now, now,
	)
	if err != nil {
		return AuthRole{}, false, err
	}
	roleID, _ := res.LastInsertId()
	for _, mod := range permissionMods {
		if _, err := s.db.Exec(
			`INSERT INTO role_permissions (role_id, module, access, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
			roleID, mod, AccessEdit, now, now,
		); err != nil {
			return AuthRole{}, false, err
		}
	}
	perms, pErr := s.ensureSuperRolePermissions(roleID)
	if pErr != nil {
		return AuthRole{}, false, pErr
	}
	return AuthRole{
		ID:          roleID,
		Name:        "super_admin",
		IsSuper:     true,
		BuiltIn:     true,
		CreatedAt:   now,
		UpdatedAt:   now,
		Permissions: perms,
	}, true, nil
}

func (s *Store) ensureSuperRolePermissions(roleID int64) (map[string]string, error) {
	now := time.Now().Format(time.RFC3339)
	for _, mod := range permissionMods {
		if _, err := s.db.Exec(
			`INSERT INTO role_permissions (role_id, module, access, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?)
			 ON CONFLICT(role_id, module)
			 DO UPDATE SET access=excluded.access, updated_at=excluded.updated_at`,
			roleID, mod, AccessEdit, now, now,
		); err != nil {
			return nil, err
		}
	}
	return s.getRolePermissions(roleID)
}

func hashPassword(password string) (string, error) {
	// format: sha256$<iter>$<salt_base64>$<hash_base64>
	const iterations = 120000
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	digest := derivePassword(password, salt, iterations)
	return "sha256$" + strconv.Itoa(iterations) + "$" +
		base64.RawStdEncoding.EncodeToString(salt) + "$" +
		base64.RawStdEncoding.EncodeToString(digest), nil
}

func VerifyPassword(password, hash string) bool {
	if strings.TrimSpace(password) == "" || strings.TrimSpace(hash) == "" {
		return false
	}
	parts := strings.Split(hash, "$")
	if len(parts) != 4 {
		return false
	}
	if parts[0] != "sha256" {
		return false
	}
	iter, err := strconv.Atoi(parts[1])
	if err != nil || iter < 1000 {
		return false
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[2])
	if err != nil || len(salt) == 0 {
		return false
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[3])
	if err != nil || len(want) == 0 {
		return false
	}
	got := derivePassword(password, salt, iter)
	return hmac.Equal(got, want)
}

func derivePassword(password string, salt []byte, iterations int) []byte {
	msg := append([]byte{}, salt...)
	msg = append(msg, []byte(password)...)
	sum := sha256.Sum256(msg)
	out := sum[:]
	for i := 1; i < iterations; i++ {
		m := make([]byte, 0, len(out)+len(salt))
		m = append(m, out...)
		m = append(m, salt...)
		s := sha256.Sum256(m)
		out = s[:]
	}
	cp := make([]byte, len(out))
	copy(cp, out)
	return cp
}

func ValidateUsername(username string) error {
	name := strings.TrimSpace(username)
	if !userNameRegexp.MatchString(name) {
		return fmt.Errorf("账号需为 5-10 位英文或数字")
	}
	return nil
}

func ValidatePassword(password string) error {
	p := strings.TrimSpace(password)
	if len(p) < 8 || len(p) > 16 {
		return fmt.Errorf("密码长度需为 8-16 位")
	}
	if !lowerRegexp.MatchString(p) && !upperRegexp.MatchString(p) {
		return fmt.Errorf("密码必须包含英文")
	}
	if !numberRegexp.MatchString(p) {
		return fmt.Errorf("密码必须包含数字")
	}
	if !symbolRegexp.MatchString(p) {
		return fmt.Errorf("密码必须包含符号")
	}
	return nil
}

func ValidateRoleName(name string) error {
	n := strings.TrimSpace(name)
	if !roleNameRegexp.MatchString(n) {
		return fmt.Errorf("权限组名需为 2-24 位，允许中文、英文、数字、下划线、中划线")
	}
	return nil
}

func generateAlphaNumPassword(length int) (string, error) {
	if length <= 0 {
		length = 10
	}
	const charset = "ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz23456789"
	buf := make([]byte, length)
	rnd := make([]byte, length)
	if _, err := rand.Read(rnd); err != nil {
		return "", err
	}
	for i := 0; i < length; i++ {
		buf[i] = charset[int(rnd[i])%len(charset)]
	}
	return string(buf), nil
}

func (s *Store) getRolePermissions(roleID int64) (map[string]string, error) {
	rows, err := s.db.Query(
		`SELECT module, access FROM role_permissions WHERE role_id=?`,
		roleID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	perms := map[string]string{}
	for rows.Next() {
		var module, access string
		if err := rows.Scan(&module, &access); err != nil {
			return nil, err
		}
		perms[module] = normalizeAccess(access)
	}
	return normalizePermissions(perms), nil
}

func (s *Store) getUserByUsernameRaw(username string) (authUserRow, error) {
	var row authUserRow
	var active, isSuper, builtIn, mustChange int
	err := s.db.QueryRow(
		`SELECT u.id, u.username, u.password_hash, u.role_id, u.active, COALESCE(u.built_in,0), COALESCE(u.must_change_credentials,0),
		        COALESCE(u.last_login_at,''), u.created_at, u.updated_at, COALESCE(r.name,''), COALESCE(r.is_super,0)
		 FROM users u
		 LEFT JOIN role_groups r ON r.id=u.role_id
		 WHERE u.username=? LIMIT 1`,
		strings.TrimSpace(username),
	).Scan(
		&row.ID, &row.Username, &row.PasswordHash, &row.RoleID, &active, &builtIn, &mustChange,
		&row.LastLoginAt, &row.CreatedAt, &row.UpdatedAt, &row.RoleName, &isSuper,
	)
	if err != nil {
		return authUserRow{}, err
	}
	row.Active = active == 1
	row.IsSuper = isSuper == 1
	row.BuiltIn = builtIn == 1
	row.MustChangeCredentials = mustChange == 1
	perms, err := s.getRolePermissions(row.RoleID)
	if err != nil {
		return authUserRow{}, err
	}
	row.Permissions = perms
	return row, nil
}

func (s *Store) AuthenticateUser(username, password string) (AuthUser, bool, error) {
	row, err := s.getUserByUsernameRaw(username)
	if err == sql.ErrNoRows {
		return AuthUser{}, false, nil
	}
	if err != nil {
		return AuthUser{}, false, err
	}
	if !row.Active {
		return AuthUser{}, false, nil
	}
	if !VerifyPassword(password, row.PasswordHash) {
		return AuthUser{}, false, nil
	}
	now := time.Now().Format(time.RFC3339)
	_, _ = s.db.Exec(`UPDATE users SET last_login_at=?, updated_at=? WHERE id=?`, now, now, row.ID)
	row.LastLoginAt = now
	return row.AuthUser, true, nil
}

func (s *Store) CreateUser(username, password string, roleID int64) (AuthUser, error) {
	if err := ValidateUsername(username); err != nil {
		return AuthUser{}, err
	}
	if err := ValidatePassword(password); err != nil {
		return AuthUser{}, err
	}
	if roleID <= 0 {
		return AuthUser{}, fmt.Errorf("角色无效")
	}
	hash, err := hashPassword(password)
	if err != nil {
		return AuthUser{}, err
	}
	now := time.Now().Format(time.RFC3339)
	res, err := s.db.Exec(
		`INSERT INTO users (username, password_hash, role_id, active, built_in, must_change_credentials, created_at, updated_at)
		 VALUES (?, ?, ?, 1, 0, 1, ?, ?)`,
		strings.TrimSpace(username), hash, roleID, now, now,
	)
	if err != nil {
		msg := strings.ToLower(err.Error())
		if strings.Contains(msg, "unique") {
			return AuthUser{}, fmt.Errorf("账号已存在")
		}
		return AuthUser{}, err
	}
	id, _ := res.LastInsertId()
	return s.GetUserByID(id)
}

func (s *Store) GetUserByID(userID int64) (AuthUser, error) {
	if userID <= 0 {
		return AuthUser{}, fmt.Errorf("用户无效")
	}
	var out AuthUser
	var active, isSuper, builtIn, mustChange int
	err := s.db.QueryRow(
		`SELECT u.id, u.username, u.role_id, u.active, COALESCE(u.built_in,0), COALESCE(u.must_change_credentials,0),
		        COALESCE(u.last_login_at,''), u.created_at, u.updated_at, COALESCE(r.name,''), COALESCE(r.is_super,0)
		 FROM users u
		 LEFT JOIN role_groups r ON r.id=u.role_id
		 WHERE u.id=? LIMIT 1`,
		userID,
	).Scan(
		&out.ID, &out.Username, &out.RoleID, &active, &builtIn, &mustChange,
		&out.LastLoginAt, &out.CreatedAt, &out.UpdatedAt, &out.RoleName, &isSuper,
	)
	if err != nil {
		return AuthUser{}, err
	}
	out.Active = active == 1
	out.IsSuper = isSuper == 1
	out.BuiltIn = builtIn == 1
	out.MustChangeCredentials = mustChange == 1
	perms, err := s.getRolePermissions(out.RoleID)
	if err != nil {
		return AuthUser{}, err
	}
	out.Permissions = perms
	return out, nil
}

func (s *Store) ListUsers() ([]AuthUser, error) {
	rows, err := s.db.Query(
		`SELECT u.id, u.username, u.role_id, u.active, COALESCE(u.built_in,0), COALESCE(u.must_change_credentials,0),
		        COALESCE(u.last_login_at,''), u.created_at, u.updated_at, COALESCE(r.name,''), COALESCE(r.is_super,0)
		 FROM users u
		 LEFT JOIN role_groups r ON r.id=u.role_id
		 ORDER BY u.id ASC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AuthUser
	for rows.Next() {
		var item AuthUser
		var active, isSuper, builtIn, mustChange int
		if err := rows.Scan(
			&item.ID, &item.Username, &item.RoleID, &active, &builtIn, &mustChange,
			&item.LastLoginAt, &item.CreatedAt, &item.UpdatedAt, &item.RoleName, &isSuper,
		); err != nil {
			return nil, err
		}
		item.Active = active == 1
		item.IsSuper = isSuper == 1
		item.BuiltIn = builtIn == 1
		item.MustChangeCredentials = mustChange == 1
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range out {
		perms, err := s.getRolePermissions(out[i].RoleID)
		if err != nil {
			return nil, err
		}
		out[i].Permissions = perms
	}
	return out, nil
}

func (s *Store) UpdateUserRole(userID, roleID int64) error {
	if userID <= 0 || roleID <= 0 {
		return fmt.Errorf("参数无效")
	}
	var builtIn int
	if err := s.db.QueryRow(`SELECT COALESCE(built_in,0) FROM users WHERE id=? LIMIT 1`, userID).Scan(&builtIn); err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("用户不存在")
		}
		return err
	}
	if builtIn == 1 {
		return fmt.Errorf("默认管理员角色不可变更")
	}
	now := time.Now().Format(time.RFC3339)
	res, err := s.db.Exec(`UPDATE users SET role_id=?, updated_at=? WHERE id=?`, roleID, now, userID)
	if err != nil {
		return err
	}
	aff, _ := res.RowsAffected()
	if aff == 0 {
		return fmt.Errorf("用户不存在")
	}
	return nil
}

func (s *Store) UpdateUserPassword(userID int64, password string, requireReset bool) error {
	if err := ValidatePassword(password); err != nil {
		return err
	}
	hash, err := hashPassword(password)
	if err != nil {
		return err
	}
	now := time.Now().Format(time.RFC3339)
	res, err := s.db.Exec(
		`UPDATE users SET password_hash=?, must_change_credentials=?, updated_at=? WHERE id=?`,
		hash, boolToInt(requireReset), now, userID,
	)
	if err != nil {
		return err
	}
	aff, _ := res.RowsAffected()
	if aff == 0 {
		return fmt.Errorf("用户不存在")
	}
	return nil
}

func (s *Store) DeleteUser(userID int64) error {
	if userID <= 0 {
		return fmt.Errorf("用户无效")
	}
	var builtIn int
	if err := s.db.QueryRow(`SELECT COALESCE(built_in,0) FROM users WHERE id=? LIMIT 1`, userID).Scan(&builtIn); err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("用户不存在")
		}
		return err
	}
	if builtIn == 1 {
		return fmt.Errorf("内置用户不可删除")
	}
	res, err := s.db.Exec(`DELETE FROM users WHERE id=?`, userID)
	if err != nil {
		return err
	}
	aff, _ := res.RowsAffected()
	if aff == 0 {
		return fmt.Errorf("用户不存在")
	}
	return nil
}

func (s *Store) ChangeOwnCredentials(userID int64, currentPassword, newUsername, newPassword string) (AuthUser, error) {
	if userID <= 0 {
		return AuthUser{}, fmt.Errorf("用户无效")
	}
	nUser := strings.TrimSpace(newUsername)
	if err := ValidateUsername(nUser); err != nil {
		return AuthUser{}, err
	}
	if err := ValidatePassword(newPassword); err != nil {
		return AuthUser{}, err
	}
	row := s.db.QueryRow(`SELECT username, password_hash FROM users WHERE id=? LIMIT 1`, userID)
	var curUser, hash string
	if err := row.Scan(&curUser, &hash); err != nil {
		if err == sql.ErrNoRows {
			return AuthUser{}, fmt.Errorf("用户不存在")
		}
		return AuthUser{}, err
	}
	if !VerifyPassword(currentPassword, hash) {
		return AuthUser{}, fmt.Errorf("当前密码错误")
	}
	newHash, err := hashPassword(newPassword)
	if err != nil {
		return AuthUser{}, err
	}
	now := time.Now().Format(time.RFC3339)
	_, err = s.db.Exec(
		`UPDATE users
		 SET username=?, password_hash=?, must_change_credentials=0, updated_at=?
		 WHERE id=?`,
		nUser, newHash, now, userID,
	)
	if err != nil {
		msg := strings.ToLower(err.Error())
		if strings.Contains(msg, "unique") {
			return AuthUser{}, fmt.Errorf("账号已存在")
		}
		return AuthUser{}, err
	}
	return s.GetUserByID(userID)
}

func (s *Store) ListRoles() ([]AuthRole, error) {
	rows, err := s.db.Query(
		`SELECT id, name, is_super, built_in, created_at, updated_at
		 FROM role_groups
		 ORDER BY built_in DESC, id ASC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AuthRole
	for rows.Next() {
		var item AuthRole
		var isSuper, builtIn int
		if err := rows.Scan(&item.ID, &item.Name, &isSuper, &builtIn, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		item.IsSuper = isSuper == 1
		item.BuiltIn = builtIn == 1
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range out {
		perms, err := s.getRolePermissions(out[i].ID)
		if err != nil {
			return nil, err
		}
		out[i].Permissions = perms
	}
	return out, nil
}

func (s *Store) CreateRole(name string, permissions map[string]string, isSuper bool) (AuthRole, error) {
	if err := ValidateRoleName(name); err != nil {
		return AuthRole{}, err
	}
	now := time.Now().Format(time.RFC3339)
	tx, err := s.db.Begin()
	if err != nil {
		return AuthRole{}, err
	}
	defer func() { _ = tx.Rollback() }()

	res, err := tx.Exec(
		`INSERT INTO role_groups (name, is_super, built_in, created_at, updated_at) VALUES (?, ?, 0, ?, ?)`,
		strings.TrimSpace(name), boolToInt(isSuper), now, now,
	)
	if err != nil {
		msg := strings.ToLower(err.Error())
		if strings.Contains(msg, "unique") {
			return AuthRole{}, fmt.Errorf("角色名已存在")
		}
		return AuthRole{}, err
	}
	roleID, _ := res.LastInsertId()
	perms := normalizePermissions(permissions)
	if isSuper {
		for mod := range perms {
			perms[mod] = AccessEdit
		}
	}
	for mod, access := range perms {
		if _, err := tx.Exec(
			`INSERT INTO role_permissions (role_id, module, access, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
			roleID, mod, normalizeAccess(access), now, now,
		); err != nil {
			return AuthRole{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return AuthRole{}, err
	}
	return s.GetRoleByID(roleID)
}

func (s *Store) GetRoleByID(roleID int64) (AuthRole, error) {
	if roleID <= 0 {
		return AuthRole{}, fmt.Errorf("角色无效")
	}
	row := s.db.QueryRow(
		`SELECT id, name, is_super, built_in, created_at, updated_at FROM role_groups WHERE id=? LIMIT 1`,
		roleID,
	)
	var out AuthRole
	var isSuper, builtIn int
	if err := row.Scan(&out.ID, &out.Name, &isSuper, &builtIn, &out.CreatedAt, &out.UpdatedAt); err != nil {
		return AuthRole{}, err
	}
	out.IsSuper = isSuper == 1
	out.BuiltIn = builtIn == 1
	perms, err := s.getRolePermissions(out.ID)
	if err != nil {
		return AuthRole{}, err
	}
	out.Permissions = perms
	return out, nil
}

func (s *Store) UpdateRole(roleID int64, name string, permissions map[string]string) (AuthRole, error) {
	if roleID <= 0 {
		return AuthRole{}, fmt.Errorf("角色无效")
	}
	role, err := s.GetRoleByID(roleID)
	if err != nil {
		return AuthRole{}, err
	}
	if role.BuiltIn {
		return AuthRole{}, fmt.Errorf("内置角色不可修改")
	}
	if err := ValidateRoleName(name); err != nil {
		return AuthRole{}, err
	}
	now := time.Now().Format(time.RFC3339)
	tx, err := s.db.Begin()
	if err != nil {
		return AuthRole{}, err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.Exec(`UPDATE role_groups SET name=?, updated_at=? WHERE id=?`, strings.TrimSpace(name), now, roleID); err != nil {
		msg := strings.ToLower(err.Error())
		if strings.Contains(msg, "unique") {
			return AuthRole{}, fmt.Errorf("角色名已存在")
		}
		return AuthRole{}, err
	}
	if _, err := tx.Exec(`DELETE FROM role_permissions WHERE role_id=?`, roleID); err != nil {
		return AuthRole{}, err
	}
	perms := normalizePermissions(permissions)
	for mod, access := range perms {
		if _, err := tx.Exec(
			`INSERT INTO role_permissions (role_id, module, access, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
			roleID, mod, normalizeAccess(access), now, now,
		); err != nil {
			return AuthRole{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return AuthRole{}, err
	}
	return s.GetRoleByID(roleID)
}

func (s *Store) DeleteRole(roleID int64) error {
	if roleID <= 0 {
		return fmt.Errorf("权限组无效")
	}
	role, err := s.GetRoleByID(roleID)
	if err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("权限组不存在")
		}
		return err
	}
	if role.BuiltIn || role.IsSuper {
		return fmt.Errorf("内置权限组不可删除")
	}

	var usedCount int
	if err := s.db.QueryRow(`SELECT COUNT(1) FROM users WHERE role_id=?`, roleID).Scan(&usedCount); err != nil {
		return err
	}
	if usedCount > 0 {
		return fmt.Errorf("该权限组仍有 %d 个用户在使用", usedCount)
	}

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.Exec(`DELETE FROM role_permissions WHERE role_id=?`, roleID); err != nil {
		return err
	}
	res, err := tx.Exec(`DELETE FROM role_groups WHERE id=?`, roleID)
	if err != nil {
		return err
	}
	aff, _ := res.RowsAffected()
	if aff == 0 {
		return fmt.Errorf("权限组不存在")
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	return nil
}

func (s *Store) SaveAuthAuditLog(log AuthAuditLog) error {
	if s == nil || s.db == nil {
		return nil
	}
	ts := strings.TrimSpace(log.Ts)
	if ts == "" {
		ts = time.Now().Format(time.RFC3339)
	}
	_, err := s.db.Exec(
		`INSERT INTO auth_audit_logs (ts, user_id, username, action, module, target, result, ip, user_agent, details)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		ts, log.UserID, strings.TrimSpace(log.Username), strings.TrimSpace(log.Action), strings.TrimSpace(log.Module),
		strings.TrimSpace(log.Target), strings.TrimSpace(log.Result), strings.TrimSpace(log.IP), strings.TrimSpace(log.UserAgent),
		strings.TrimSpace(log.Details),
	)
	return err
}

func (s *Store) ListAuthAuditLogs(limit int) ([]AuthAuditLog, error) {
	if limit <= 0 {
		limit = 200
	}
	if limit > 2000 {
		limit = 2000
	}
	rows, err := s.db.Query(
		`SELECT id, ts, user_id, username, action, module, target, result, ip, user_agent, details
		 FROM auth_audit_logs
		 ORDER BY id DESC
		 LIMIT ?`,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AuthAuditLog
	for rows.Next() {
		var item AuthAuditLog
		var userID sql.NullInt64
		var username, module, target, result, ip, ua, details sql.NullString
		if err := rows.Scan(
			&item.ID, &item.Ts, &userID, &username, &item.Action, &module, &target, &result, &ip, &ua, &details,
		); err != nil {
			return nil, err
		}
		if userID.Valid {
			item.UserID = userID.Int64
		}
		item.Username = username.String
		item.Module = module.String
		item.Target = target.String
		item.Result = result.String
		item.IP = ip.String
		item.UserAgent = ua.String
		item.Details = details.String
		out = append(out, item)
	}
	return out, nil
}

func EncodeAuthAuditDetails(v any) string {
	if v == nil {
		return ""
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return string(raw)
}

func CanAccess(permissions map[string]string, module, need string) bool {
	required := normalizeAccess(need)
	current := normalizeAccess(permissions[strings.TrimSpace(module)])
	return permissionLevel[current] >= permissionLevel[required]
}

func MergePermissionsForResponse(permissions map[string]string) map[string]string {
	out := normalizePermissions(permissions)
	keys := make([]string, 0, len(out))
	for k := range out {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	ordered := map[string]string{}
	for _, k := range keys {
		ordered[k] = out[k]
	}
	return ordered
}

func BuildSessionToken() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}
