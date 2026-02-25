package storage

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB
}

type RiskSnapshot struct {
	TodayPnL          float64
	PeakEquity        float64
	CurrentEquity     float64
	ConsecutiveLosses int
}

type StrategyComboScore struct {
	Combo        string  `json:"combo"`
	Score        float64 `json:"score"`
	TotalPnL     float64 `json:"total_pnl"`
	BaseEquity   float64 `json:"base_equity"`
	Observations int     `json:"observations"`
	Wins         int     `json:"wins"`
	Losses       int     `json:"losses"`
	UpdatedAt    string  `json:"updated_at"`
}

func Open(path string) (*Store, error) {
	if path == "" {
		path = "data/trade.db"
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	store := &Store{db: db}
	if err := store.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *Store) migrate() error {
	schema := []string{
		`CREATE TABLE IF NOT EXISTS ai_decisions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			ts TEXT NOT NULL,
			signal TEXT,
			confidence TEXT,
			reason TEXT,
			price REAL,
			stop_loss REAL,
			take_profit REAL,
			suggested_size REAL,
			approved_size REAL,
			approved INTEGER,
			risk_reason TEXT,
			strategy_combo TEXT,
			strategy_score REAL
		);`,
		`CREATE TABLE IF NOT EXISTS orders (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			order_id TEXT UNIQUE,
			symbol TEXT,
			side TEXT,
			size REAL,
			reduce_only INTEGER,
			status TEXT,
			payload TEXT,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS fills (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			fill_id TEXT UNIQUE,
			order_id TEXT,
			symbol TEXT,
			side TEXT,
			size REAL,
			price REAL,
			ts TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS position_snapshots (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			ts TEXT NOT NULL,
			symbol TEXT,
			side TEXT,
			size REAL,
			entry_price REAL,
			unrealized_pnl REAL,
			leverage REAL
		);`,
		`CREATE TABLE IF NOT EXISTS equity_curve (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			ts TEXT NOT NULL,
			balance REAL,
			unrealized_pnl REAL,
			equity REAL
		);`,
		`CREATE TABLE IF NOT EXISTS risk_events (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			ts TEXT NOT NULL,
			event_type TEXT,
			details TEXT
		);`,
		`CREATE TABLE IF NOT EXISTS strategy_combo_stats (
			combo TEXT PRIMARY KEY,
			base_equity REAL NOT NULL,
			last_equity REAL NOT NULL,
			total_pnl REAL NOT NULL,
			observations INTEGER NOT NULL,
			wins INTEGER NOT NULL,
			losses INTEGER NOT NULL,
			score REAL NOT NULL,
			updated_at TEXT NOT NULL
		);`,
	}
	for _, stmt := range schema {
		if _, err := s.db.Exec(stmt); err != nil {
			return err
		}
	}
	if err := s.migrateCompat(); err != nil {
		return err
	}
	return nil
}

func (s *Store) migrateCompat() error {
	alterStmts := []string{
		`ALTER TABLE ai_decisions ADD COLUMN strategy_combo TEXT;`,
		`ALTER TABLE ai_decisions ADD COLUMN strategy_score REAL;`,
	}
	for _, stmt := range alterStmts {
		if _, err := s.db.Exec(stmt); err != nil {
			msg := strings.ToLower(err.Error())
			if strings.Contains(msg, "duplicate column") || strings.Contains(msg, "already exists") {
				continue
			}
			return err
		}
	}
	return nil
}

func (s *Store) SaveAIDecision(ts time.Time, decision map[string]any) error {
	if s == nil {
		return nil
	}
	_, err := s.db.Exec(
		`INSERT INTO ai_decisions (ts, signal, confidence, reason, price, stop_loss, take_profit, suggested_size, approved_size, approved, risk_reason, strategy_combo, strategy_score)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		ts.Format(time.RFC3339),
		decision["signal"], decision["confidence"], decision["reason"],
		decision["price"], decision["stop_loss"], decision["take_profit"],
		decision["suggested_size"], decision["approved_size"], boolToInt(decision["approved"] == true),
		decision["risk_reason"], decision["strategy_combo"], decision["strategy_score"],
	)
	return err
}

func (s *Store) SaveOrder(orderID, symbol, side string, size float64, reduceOnly bool, status string, payload any) error {
	if s == nil || orderID == "" {
		return nil
	}
	raw, _ := json.Marshal(payload)
	now := time.Now().Format(time.RFC3339)
	_, err := s.db.Exec(
		`INSERT INTO orders (order_id, symbol, side, size, reduce_only, status, payload, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(order_id) DO UPDATE SET
		 	status=excluded.status,
		 	payload=excluded.payload,
		 	updated_at=excluded.updated_at`,
		orderID, symbol, side, size, boolToInt(reduceOnly), status, string(raw), now, now,
	)
	return err
}

func (s *Store) SavePositionSnapshot(symbol string, posSide string, size, entry, upl, leverage float64) error {
	if s == nil {
		return nil
	}
	_, err := s.db.Exec(
		`INSERT INTO position_snapshots (ts, symbol, side, size, entry_price, unrealized_pnl, leverage)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		time.Now().Format(time.RFC3339), symbol, posSide, size, entry, upl, leverage,
	)
	return err
}

func (s *Store) SaveFill(fillID, orderID, symbol, side string, size, price float64, ts string) error {
	if s == nil || fillID == "" {
		return nil
	}
	if ts == "" {
		ts = time.Now().Format(time.RFC3339)
	}
	_, err := s.db.Exec(
		`INSERT INTO fills (fill_id, order_id, symbol, side, size, price, ts)
		 VALUES (?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(fill_id) DO NOTHING`,
		fillID, orderID, symbol, side, size, price, ts,
	)
	return err
}

func (s *Store) SaveEquity(balance, upl float64) error {
	if s == nil {
		return nil
	}
	equity := balance + upl
	_, err := s.db.Exec(
		`INSERT INTO equity_curve (ts, balance, unrealized_pnl, equity) VALUES (?, ?, ?, ?)`,
		time.Now().Format(time.RFC3339), balance, upl, equity,
	)
	return err
}

func (s *Store) SaveRiskEvent(eventType, details string) error {
	if s == nil {
		return nil
	}
	_, err := s.db.Exec(
		`INSERT INTO risk_events (ts, event_type, details) VALUES (?, ?, ?)`,
		time.Now().Format(time.RFC3339), eventType, details,
	)
	return err
}

func (s *Store) LoadRiskSnapshot() (RiskSnapshot, error) {
	if s == nil {
		return RiskSnapshot{}, nil
	}
	var out RiskSnapshot
	today := time.Now().Format("2006-01-02")

	err := s.db.QueryRow(
		`SELECT COALESCE(SUM(unrealized_pnl),0) FROM position_snapshots WHERE substr(ts,1,10)=?`,
		today,
	).Scan(&out.TodayPnL)
	if err != nil {
		return out, err
	}

	err = s.db.QueryRow(`SELECT COALESCE(MAX(equity),0) FROM equity_curve`).Scan(&out.PeakEquity)
	if err != nil {
		return out, err
	}
	err = s.db.QueryRow(`SELECT COALESCE(equity,0) FROM equity_curve ORDER BY id DESC LIMIT 1`).Scan(&out.CurrentEquity)
	if err != nil {
		return out, err
	}

	// 连续亏损（简化）：按最近持仓快照未实现盈亏连续为负计数。
	rows, err := s.db.Query(`SELECT unrealized_pnl FROM position_snapshots ORDER BY id DESC LIMIT 30`)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	consecutive := 0
	for rows.Next() {
		var upl sql.NullFloat64
		if err := rows.Scan(&upl); err != nil {
			return out, err
		}
		if !upl.Valid {
			break
		}
		if upl.Float64 < 0 {
			consecutive++
			continue
		}
		break
	}
	out.ConsecutiveLosses = consecutive
	return out, nil
}

func (s *Store) OpenOrders() ([]string, error) {
	if s == nil {
		return nil, nil
	}
	rows, err := s.db.Query(`SELECT order_id FROM orders WHERE status IN ('live','partially_filled') ORDER BY id DESC LIMIT 200`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, nil
}

func (s *Store) UpdateStrategyComboScore(combo string, equity float64) (float64, error) {
	if s == nil || strings.TrimSpace(combo) == "" || equity <= 0 {
		return 0, nil
	}
	combo = strings.TrimSpace(combo)
	now := time.Now().Format(time.RFC3339)

	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()

	var (
		base         float64
		last         float64
		total        float64
		observations int
		wins         int
		losses       int
	)
	err = tx.QueryRow(
		`SELECT base_equity, last_equity, total_pnl, observations, wins, losses
		 FROM strategy_combo_stats WHERE combo = ?`,
		combo,
	).Scan(&base, &last, &total, &observations, &wins, &losses)
	if err == sql.ErrNoRows {
		score := 5.0
		_, execErr := tx.Exec(
			`INSERT INTO strategy_combo_stats (combo, base_equity, last_equity, total_pnl, observations, wins, losses, score, updated_at)
			 VALUES (?, ?, ?, 0, 1, 0, 0, ?, ?)`,
			combo, equity, equity, score, now,
		)
		if execErr != nil {
			return 0, execErr
		}
		if err := tx.Commit(); err != nil {
			return 0, err
		}
		return score, nil
	}
	if err != nil {
		return 0, err
	}

	delta := equity - last
	total += delta
	observations++
	if delta > 0 {
		wins++
	} else if delta < 0 {
		losses++
	}
	score := pnlScore(total, base)

	_, err = tx.Exec(
		`UPDATE strategy_combo_stats
		 SET last_equity=?, total_pnl=?, observations=?, wins=?, losses=?, score=?, updated_at=?
		 WHERE combo=?`,
		equity, total, observations, wins, losses, score, now, combo,
	)
	if err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return score, nil
}

func (s *Store) GetStrategyComboScores(limit int) ([]StrategyComboScore, error) {
	if s == nil {
		return nil, nil
	}
	if limit <= 0 {
		limit = 20
	}
	rows, err := s.db.Query(
		`SELECT combo, score, total_pnl, base_equity, observations, wins, losses, updated_at
		 FROM strategy_combo_stats
		 ORDER BY score DESC, total_pnl DESC
		 LIMIT ?`,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []StrategyComboScore
	for rows.Next() {
		var item StrategyComboScore
		if err := rows.Scan(
			&item.Combo, &item.Score, &item.TotalPnL, &item.BaseEquity,
			&item.Observations, &item.Wins, &item.Losses, &item.UpdatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, nil
}

func pnlScore(totalPnL, baseEquity float64) float64 {
	if baseEquity <= 0 {
		baseEquity = 1
	}
	roi := totalPnL / baseEquity
	score := 10.0 / (1.0 + math.Exp(-12.0*roi))
	if score < 0 {
		score = 0
	}
	if score > 10 {
		score = 10
	}
	return math.Round(score*100) / 100
}

func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func (s *Store) String() string {
	if s == nil {
		return "<nil-store>"
	}
	return fmt.Sprintf("sqlite-store(%p)", s.db)
}
