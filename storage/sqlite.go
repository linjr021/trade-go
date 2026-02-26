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

type TradeRecord struct {
	ID            int64   `json:"id"`
	Ts            string  `json:"ts"`
	Symbol        string  `json:"symbol"`
	Signal        string  `json:"signal"`
	Confidence    string  `json:"confidence"`
	StrategyCombo string  `json:"strategy_combo"`
	Approved      bool    `json:"approved"`
	ApprovedSize  float64 `json:"approved_size"`
	Price         float64 `json:"price"`
	StopLoss      float64 `json:"stop_loss"`
	TakeProfit    float64 `json:"take_profit"`
	RiskReason    string  `json:"risk_reason"`
	PositionSide  string  `json:"position_side"`
	PositionSize  float64 `json:"position_size"`
	UnrealizedPnL float64 `json:"unrealized_pnl"`
}

type EquityPoint struct {
	Ts     string  `json:"ts"`
	Equity float64 `json:"equity"`
}

type EquitySummary struct {
	TotalFunds       float64 `json:"total_funds"`
	TodayPnLAmount   float64 `json:"today_pnl_amount"`
	TodayPnLPct      float64 `json:"today_pnl_pct"`
	CumulativePnL    float64 `json:"cumulative_pnl"`
	CumulativePnLPct float64 `json:"cumulative_pnl_pct"`
}

type DailyPnL struct {
	Date      string  `json:"date"`
	PnLAmount float64 `json:"pnl_amount"`
	PnLPct    float64 `json:"pnl_pct"`
}

type BacktestRun struct {
	ID                      int64   `json:"id"`
	CreatedAt               string  `json:"created_at"`
	Strategy                string  `json:"strategy"`
	Pair                    string  `json:"pair"`
	Habit                   string  `json:"habit"`
	Start                   string  `json:"start"`
	End                     string  `json:"end"`
	Bars                    int     `json:"bars"`
	InitialMargin           float64 `json:"initial_margin"`
	Leverage                int     `json:"leverage"`
	PositionSizingMode      string  `json:"position_sizing_mode"`
	HighConfidenceAmount    float64 `json:"high_confidence_amount"`
	LowConfidenceAmount     float64 `json:"low_confidence_amount"`
	HighConfidenceMarginPct float64 `json:"high_confidence_margin_pct"`
	LowConfidenceMarginPct  float64 `json:"low_confidence_margin_pct"`
	TotalPnL                float64 `json:"total_pnl"`
	FinalEquity             float64 `json:"final_equity"`
	ReturnPct               float64 `json:"return_pct"`
	Wins                    int     `json:"wins"`
	Losses                  int     `json:"losses"`
	Ratio                   float64 `json:"ratio"`
}

type BacktestRunRecord struct {
	ID         string  `json:"id"`
	TS         int64   `json:"ts"`
	Side       string  `json:"side"`
	Confidence string  `json:"confidence"`
	Size       float64 `json:"size"`
	Leverage   int     `json:"leverage"`
	Entry      float64 `json:"entry"`
	Exit       float64 `json:"exit"`
	PnL        float64 `json:"pnl"`
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
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(0)
	if err := configureSQLite(db); err != nil {
		_ = db.Close()
		return nil, err
	}
	store := &Store{db: db}
	if err := store.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func configureSQLite(db *sql.DB) error {
	pragmas := []string{
		`PRAGMA journal_mode = WAL;`,
		`PRAGMA synchronous = NORMAL;`,
		`PRAGMA temp_store = MEMORY;`,
		`PRAGMA foreign_keys = ON;`,
		`PRAGMA busy_timeout = 5000;`,
	}
	for _, stmt := range pragmas {
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("apply sqlite pragma failed: %s: %w", stmt, err)
		}
	}
	return nil
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
		`CREATE TABLE IF NOT EXISTS backtest_runs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			created_at TEXT NOT NULL,
			strategy TEXT,
			pair TEXT,
			habit TEXT,
			start_month TEXT,
			end_month TEXT,
			bars INTEGER,
			initial_margin REAL,
			leverage INTEGER,
			position_sizing_mode TEXT,
			high_confidence_amount REAL,
			low_confidence_amount REAL,
			high_confidence_margin_pct REAL,
			low_confidence_margin_pct REAL,
			total_pnl REAL,
			final_equity REAL,
			return_pct REAL,
			wins INTEGER,
			losses INTEGER,
			ratio REAL
		);`,
		`CREATE TABLE IF NOT EXISTS backtest_run_records (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			run_id INTEGER NOT NULL,
			seq INTEGER NOT NULL,
			ts INTEGER,
			side TEXT,
			confidence TEXT,
			size REAL,
			leverage INTEGER,
			entry REAL,
			exit REAL,
			pnl REAL
		);`,
		`CREATE INDEX IF NOT EXISTS idx_backtest_run_records_run_id ON backtest_run_records(run_id);`,
		`CREATE INDEX IF NOT EXISTS idx_ai_decisions_ts ON ai_decisions(ts);`,
		`CREATE INDEX IF NOT EXISTS idx_orders_status_updated_at ON orders(status, updated_at);`,
		`CREATE INDEX IF NOT EXISTS idx_orders_created_at ON orders(created_at);`,
		`CREATE INDEX IF NOT EXISTS idx_fills_order_id ON fills(order_id);`,
		`CREATE INDEX IF NOT EXISTS idx_fills_ts ON fills(ts);`,
		`CREATE INDEX IF NOT EXISTS idx_position_snapshots_ts ON position_snapshots(ts);`,
		`CREATE INDEX IF NOT EXISTS idx_equity_curve_ts ON equity_curve(ts);`,
		`CREATE INDEX IF NOT EXISTS idx_risk_events_ts ON risk_events(ts);`,
		`CREATE INDEX IF NOT EXISTS idx_backtest_runs_created_at ON backtest_runs(created_at);`,
		`CREATE INDEX IF NOT EXISTS idx_backtest_runs_pair_created_at ON backtest_runs(pair, created_at);`,
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

func (s *Store) RecentTradeRecords(limit int) ([]TradeRecord, error) {
	if s == nil {
		return nil, nil
	}
	if limit <= 0 {
		limit = 20
	}
	if limit > 200 {
		limit = 200
	}
	rows, err := s.db.Query(
		`SELECT
			d.id, d.ts, d.signal, d.confidence, d.strategy_combo, d.approved, d.approved_size,
			d.price, d.stop_loss, d.take_profit, d.risk_reason,
			(SELECT p.symbol FROM position_snapshots p WHERE p.ts <= d.ts ORDER BY p.id DESC LIMIT 1) AS symbol,
			(SELECT p.side FROM position_snapshots p WHERE p.ts <= d.ts ORDER BY p.id DESC LIMIT 1) AS position_side,
			(SELECT p.size FROM position_snapshots p WHERE p.ts <= d.ts ORDER BY p.id DESC LIMIT 1) AS position_size,
			(SELECT p.unrealized_pnl FROM position_snapshots p WHERE p.ts <= d.ts ORDER BY p.id DESC LIMIT 1) AS unrealized_pnl
		FROM ai_decisions d
		ORDER BY d.id DESC
		LIMIT ?`,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []TradeRecord
	for rows.Next() {
		var (
			item                   TradeRecord
			symbol, side, riskNote sql.NullString
			signal, conf, combo    sql.NullString
			price, sl, tp, size    sql.NullFloat64
			pSize, upl             sql.NullFloat64
			approved               sql.NullInt64
		)
		if err := rows.Scan(
			&item.ID, &item.Ts, &signal, &conf, &combo, &approved, &size,
			&price, &sl, &tp, &riskNote,
			&symbol, &side, &pSize, &upl,
		); err != nil {
			return nil, err
		}
		item.Symbol = symbol.String
		item.Signal = signal.String
		item.Confidence = conf.String
		item.StrategyCombo = combo.String
		item.Approved = approved.Valid && approved.Int64 == 1
		if size.Valid {
			item.ApprovedSize = size.Float64
		}
		if price.Valid {
			item.Price = price.Float64
		}
		if sl.Valid {
			item.StopLoss = sl.Float64
		}
		if tp.Valid {
			item.TakeProfit = tp.Float64
		}
		item.RiskReason = riskNote.String
		item.PositionSide = side.String
		if pSize.Valid {
			item.PositionSize = pSize.Float64
		}
		if upl.Valid {
			item.UnrealizedPnL = upl.Float64
		}
		out = append(out, item)
	}
	return out, nil
}

func (s *Store) EquitySummary() (EquitySummary, error) {
	if s == nil {
		return EquitySummary{}, nil
	}
	var out EquitySummary
	rows, err := s.db.Query(`SELECT ts, equity, balance FROM equity_curve ORDER BY id ASC`)
	if err != nil {
		return out, err
	}
	defer rows.Close()

	var (
		firstEquity float64
		lastEquity  float64
		lastBalance float64
		firstToday  float64
		hasAny      bool
		hasToday    bool
	)
	today := time.Now().Format("2006-01-02")
	for rows.Next() {
		var ts string
		var equity, balance sql.NullFloat64
		if err := rows.Scan(&ts, &equity, &balance); err != nil {
			return out, err
		}
		if !equity.Valid {
			continue
		}
		v := equity.Float64
		if !hasAny {
			firstEquity = v
			hasAny = true
		}
		lastEquity = v
		if balance.Valid {
			lastBalance = balance.Float64
		}
		if strings.HasPrefix(ts, today) && !hasToday {
			firstToday = v
			hasToday = true
		}
	}
	if !hasAny {
		return out, nil
	}
	out.TotalFunds = lastEquity
	if hasToday {
		out.TodayPnLAmount = lastEquity - firstToday
		if firstToday != 0 {
			out.TodayPnLPct = out.TodayPnLAmount / firstToday * 100
		}
	}
	out.CumulativePnL = lastEquity - firstEquity
	if firstEquity != 0 {
		out.CumulativePnLPct = out.CumulativePnL / firstEquity * 100
	}
	if out.TotalFunds == 0 && lastBalance > 0 {
		out.TotalFunds = lastBalance
	}
	return out, nil
}

func (s *Store) EquityTrendSince(since time.Time) ([]EquityPoint, error) {
	if s == nil {
		return nil, nil
	}
	rows, err := s.db.Query(
		`SELECT ts, equity FROM equity_curve WHERE ts >= ? ORDER BY id ASC`,
		since.Format(time.RFC3339),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []EquityPoint
	for rows.Next() {
		var ts string
		var equity sql.NullFloat64
		if err := rows.Scan(&ts, &equity); err != nil {
			return nil, err
		}
		if !equity.Valid {
			continue
		}
		out = append(out, EquityPoint{Ts: ts, Equity: equity.Float64})
	}
	return out, nil
}

func (s *Store) DailyPnLByMonth(month string) ([]DailyPnL, error) {
	if s == nil {
		return nil, nil
	}
	start, err := time.Parse("2006-01", month)
	if err != nil {
		return nil, err
	}
	end := start.AddDate(0, 1, 0)
	rows, err := s.db.Query(
		`SELECT ts, equity FROM equity_curve WHERE ts >= ? AND ts < ? ORDER BY id ASC`,
		start.Format(time.RFC3339), end.Format(time.RFC3339),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type dayAgg struct{ first, last float64 }
	agg := map[string]dayAgg{}
	order := []string{}
	for rows.Next() {
		var ts string
		var equity sql.NullFloat64
		if err := rows.Scan(&ts, &equity); err != nil {
			return nil, err
		}
		if !equity.Valid || len(ts) < 10 {
			continue
		}
		day := ts[:10]
		v := equity.Float64
		if a, ok := agg[day]; ok {
			a.last = v
			agg[day] = a
		} else {
			agg[day] = dayAgg{first: v, last: v}
			order = append(order, day)
		}
	}
	out := make([]DailyPnL, 0, len(order))
	for _, day := range order {
		a := agg[day]
		pnl := a.last - a.first
		pct := 0.0
		if a.first != 0 {
			pct = pnl / a.first * 100
		}
		out = append(out, DailyPnL{Date: day, PnLAmount: pnl, PnLPct: pct})
	}
	return out, nil
}

func (s *Store) SaveBacktestRun(run BacktestRun, records []BacktestRunRecord) (int64, error) {
	if s == nil {
		return 0, nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()

	createdAt := run.CreatedAt
	if strings.TrimSpace(createdAt) == "" {
		createdAt = time.Now().Format(time.RFC3339)
	}
	res, err := tx.Exec(
		`INSERT INTO backtest_runs (
			created_at, strategy, pair, habit, start_month, end_month, bars, initial_margin, leverage,
			position_sizing_mode, high_confidence_amount, low_confidence_amount,
			high_confidence_margin_pct, low_confidence_margin_pct,
			total_pnl, final_equity, return_pct, wins, losses, ratio
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		createdAt,
		run.Strategy, run.Pair, run.Habit, run.Start, run.End, run.Bars, run.InitialMargin, run.Leverage,
		run.PositionSizingMode, run.HighConfidenceAmount, run.LowConfidenceAmount,
		run.HighConfidenceMarginPct, run.LowConfidenceMarginPct,
		run.TotalPnL, run.FinalEquity, run.ReturnPct, run.Wins, run.Losses, run.Ratio,
	)
	if err != nil {
		return 0, err
	}
	runID, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}

	stmt, err := tx.Prepare(
		`INSERT INTO backtest_run_records (
			run_id, seq, ts, side, confidence, size, leverage, entry, exit, pnl
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
	)
	if err != nil {
		return 0, err
	}
	defer stmt.Close()

	for i, r := range records {
		if _, err := stmt.Exec(
			runID, i+1, r.TS, r.Side, r.Confidence, r.Size, r.Leverage, r.Entry, r.Exit, r.PnL,
		); err != nil {
			return 0, err
		}
	}

	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return runID, nil
}

func (s *Store) BacktestRuns(limit int) ([]BacktestRun, error) {
	if s == nil {
		return nil, nil
	}
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}
	rows, err := s.db.Query(
		`SELECT
			id, created_at, strategy, pair, habit, start_month, end_month, bars,
			initial_margin, leverage, position_sizing_mode,
			high_confidence_amount, low_confidence_amount, high_confidence_margin_pct, low_confidence_margin_pct,
			total_pnl, final_equity, return_pct, wins, losses, ratio
		 FROM backtest_runs
		 ORDER BY id DESC
		 LIMIT ?`,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []BacktestRun
	for rows.Next() {
		var item BacktestRun
		if err := rows.Scan(
			&item.ID, &item.CreatedAt, &item.Strategy, &item.Pair, &item.Habit, &item.Start, &item.End, &item.Bars,
			&item.InitialMargin, &item.Leverage, &item.PositionSizingMode,
			&item.HighConfidenceAmount, &item.LowConfidenceAmount, &item.HighConfidenceMarginPct, &item.LowConfidenceMarginPct,
			&item.TotalPnL, &item.FinalEquity, &item.ReturnPct, &item.Wins, &item.Losses, &item.Ratio,
		); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, nil
}

func (s *Store) BacktestRunDetail(id int64) (BacktestRun, []BacktestRunRecord, error) {
	if s == nil {
		return BacktestRun{}, nil, nil
	}
	if id <= 0 {
		return BacktestRun{}, nil, fmt.Errorf("invalid backtest id")
	}

	var run BacktestRun
	err := s.db.QueryRow(
		`SELECT
			id, created_at, strategy, pair, habit, start_month, end_month, bars,
			initial_margin, leverage, position_sizing_mode,
			high_confidence_amount, low_confidence_amount, high_confidence_margin_pct, low_confidence_margin_pct,
			total_pnl, final_equity, return_pct, wins, losses, ratio
		 FROM backtest_runs
		 WHERE id = ?`,
		id,
	).Scan(
		&run.ID, &run.CreatedAt, &run.Strategy, &run.Pair, &run.Habit, &run.Start, &run.End, &run.Bars,
		&run.InitialMargin, &run.Leverage, &run.PositionSizingMode,
		&run.HighConfidenceAmount, &run.LowConfidenceAmount, &run.HighConfidenceMarginPct, &run.LowConfidenceMarginPct,
		&run.TotalPnL, &run.FinalEquity, &run.ReturnPct, &run.Wins, &run.Losses, &run.Ratio,
	)
	if err != nil {
		return BacktestRun{}, nil, err
	}

	rows, err := s.db.Query(
		`SELECT seq, ts, side, confidence, size, leverage, entry, exit, pnl
		 FROM backtest_run_records
		 WHERE run_id = ?
		 ORDER BY seq ASC`,
		id,
	)
	if err != nil {
		return BacktestRun{}, nil, err
	}
	defer rows.Close()

	records := make([]BacktestRunRecord, 0, 256)
	for rows.Next() {
		var (
			seq int
			r   BacktestRunRecord
		)
		if err := rows.Scan(&seq, &r.TS, &r.Side, &r.Confidence, &r.Size, &r.Leverage, &r.Entry, &r.Exit, &r.PnL); err != nil {
			return BacktestRun{}, nil, err
		}
		r.ID = fmt.Sprintf("%d-%d", id, seq)
		records = append(records, r)
	}
	return run, records, nil
}

func (s *Store) DeleteBacktestRun(id int64) error {
	if s == nil {
		return nil
	}
	if id <= 0 {
		return fmt.Errorf("invalid backtest id")
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.Exec(`DELETE FROM backtest_run_records WHERE run_id = ?`, id); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM backtest_runs WHERE id = ?`, id); err != nil {
		return err
	}
	return tx.Commit()
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
