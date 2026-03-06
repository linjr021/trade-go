package server

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	aiSettingsPath            = "skills/trading-strategy-pipeline/references/ai-settings.json"
	legacyHabitProfilesPath   = "skills/trading-strategy-pipeline/references/habit-profiles.json"
	legacyStrategySchemaPath  = "skills/trading-strategy-pipeline/references/strategy-package-schema.json"
	legacySkillWorkflowPath   = "data/skill_workflow.json"
	defaultAISettingsVersion  = "ai-settings/v1"
	defaultHabitProfileSchema = "habit-profile/v1"
)

type aiSettingsDocument struct {
	Version               string                 `json:"version"`
	UpdatedAt             string                 `json:"updated_at"`
	Workflow              skillWorkflowConfig    `json:"workflow"`
	HabitProfiles         []habitProfile         `json:"habit_profiles"`
	StrategyPackageSchema map[string]interface{} `json:"strategy_package_schema"`
}

type legacyHabitProfilesDocument struct {
	Version  string         `json:"version"`
	Profiles []habitProfile `json:"profiles"`
}

func defaultHabitProfiles() []habitProfile {
	return []habitProfile{
		{
			Habit:             "10m",
			Label:             "超短线",
			Timeframe:         "15m",
			MaxLeverage:       30,
			MaxDrawdownPct:    0.04,
			MaxRiskPerTrade:   0.008,
			AllowAddPosition:  false,
			HoldBarsMin:       1,
			HoldBarsMax:       16,
			Description:       "高频、轻仓、快进快出，优先执行明确触发条件。",
			ExecutionHint:     "触发条件不完整时保持 HOLD，减少噪音交易。",
			PreferredDataSpan: 120,
		},
		{
			Habit:             "1h",
			Label:             "日内短线",
			Timeframe:         "1h",
			MaxLeverage:       20,
			MaxDrawdownPct:    0.06,
			MaxRiskPerTrade:   0.010,
			AllowAddPosition:  true,
			HoldBarsMin:       2,
			HoldBarsMax:       24,
			Description:       "兼顾趋势与结构，适合主流交易对日内执行。",
			ExecutionHint:     "信号冲突时优先观望，等待关键位确认。",
			PreferredDataSpan: 160,
		},
		{
			Habit:             "4h",
			Label:             "波段",
			Timeframe:         "4h",
			MaxLeverage:       10,
			MaxDrawdownPct:    0.08,
			MaxRiskPerTrade:   0.012,
			AllowAddPosition:  true,
			HoldBarsMin:       4,
			HoldBarsMax:       40,
			Description:       "偏趋势波段，减少频繁交易，重视结构完整性。",
			ExecutionHint:     "重点观察突破回踩与均线共振。",
			PreferredDataSpan: 200,
		},
		{
			Habit:             "1D",
			Label:             "日线趋势",
			Timeframe:         "1d",
			MaxLeverage:       6,
			MaxDrawdownPct:    0.10,
			MaxRiskPerTrade:   0.015,
			AllowAddPosition:  true,
			HoldBarsMin:       3,
			HoldBarsMax:       60,
			Description:       "低频趋势策略，强调资金曲线稳定性。",
			ExecutionHint:     "优先主趋势方向，逆势只做备选策略。",
			PreferredDataSpan: 220,
		},
		{
			Habit:             "5D",
			Label:             "周内波段",
			Timeframe:         "1d",
			MaxLeverage:       5,
			MaxDrawdownPct:    0.10,
			MaxRiskPerTrade:   0.015,
			AllowAddPosition:  true,
			HoldBarsMin:       5,
			HoldBarsMax:       90,
			Description:       "周级别持仓，限制噪音交易与高杠杆。",
			ExecutionHint:     "以趋势延续为主，设置明确失效条件。",
			PreferredDataSpan: 240,
		},
		{
			Habit:             "30D",
			Label:             "中期配置",
			Timeframe:         "1d",
			MaxLeverage:       3,
			MaxDrawdownPct:    0.12,
			MaxRiskPerTrade:   0.020,
			AllowAddPosition:  false,
			HoldBarsMin:       12,
			HoldBarsMax:       180,
			Description:       "中周期配置，强调回撤控制与风险暴露。",
			ExecutionHint:     "尽量减少频繁换向，趋势失效再切换。",
			PreferredDataSpan: 280,
		},
		{
			Habit:             "90D",
			Label:             "长周期配置",
			Timeframe:         "1d",
			MaxLeverage:       2,
			MaxDrawdownPct:    0.15,
			MaxRiskPerTrade:   0.020,
			AllowAddPosition:  false,
			HoldBarsMin:       20,
			HoldBarsMax:       260,
			Description:       "长周期低频策略，优先风控与稳定收益。",
			ExecutionHint:     "避免噪音驱动交易，严格执行风控停机。",
			PreferredDataSpan: 320,
		},
	}
}

func defaultStrategyPackageSchemaMap() map[string]interface{} {
	return map[string]interface{}{
		"version": "skill-pipeline/v1",
		"required": []string{
			"version",
			"generated_at",
			"workflow",
			"symbol",
			"habit",
			"habit_profile",
			"spec_builder",
			"strategy_draft",
			"optimizer",
			"risk_reviewer",
			"release_packager",
		},
		"workflow": []string{
			"spec-builder",
			"strategy-draft",
			"optimizer",
			"risk-reviewer",
			"release-packager",
		},
		"notes": []string{
			"任一步骤失败必须回退 HOLD",
			"仓位与杠杆由 risk-plan/risk-engine 最终覆盖",
			"仅允许结构化 JSON 输出",
		},
	}
}

func defaultAISettingsDocument() aiSettingsDocument {
	return normalizeAISettingsDocument(aiSettingsDocument{
		Version:               defaultAISettingsVersion,
		UpdatedAt:             time.Now().Format(time.RFC3339),
		Workflow:              defaultSkillWorkflowConfigBuiltin(),
		HabitProfiles:         defaultHabitProfiles(),
		StrategyPackageSchema: defaultStrategyPackageSchemaMap(),
	})
}

func readAISettingsDocument() (aiSettingsDocument, error) {
	raw, err := os.ReadFile(aiSettingsPath)
	if err != nil {
		return aiSettingsDocument{}, err
	}
	var out aiSettingsDocument
	if err := json.Unmarshal(raw, &out); err != nil {
		return aiSettingsDocument{}, err
	}
	return normalizeAISettingsDocument(out), nil
}

func readLegacySkillWorkflowConfig() (skillWorkflowConfig, error) {
	raw, err := os.ReadFile(legacySkillWorkflowPath)
	if err != nil {
		return skillWorkflowConfig{}, err
	}
	var cfg skillWorkflowConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return skillWorkflowConfig{}, err
	}
	return normalizeSkillWorkflowConfig(cfg), nil
}

func readLegacyHabitProfiles() ([]habitProfile, error) {
	raw, err := os.ReadFile(legacyHabitProfilesPath)
	if err != nil {
		return nil, err
	}
	var doc legacyHabitProfilesDocument
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, err
	}
	return normalizeHabitProfiles(doc.Profiles), nil
}

func readLegacyStrategySchema() (map[string]interface{}, error) {
	raw, err := os.ReadFile(legacyStrategySchemaPath)
	if err != nil {
		return nil, err
	}
	var out map[string]interface{}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func loadAISettingsDocument() aiSettingsDocument {
	doc, err := readAISettingsDocument()
	if err == nil {
		return doc
	}

	// Auto-migrate once from legacy runtime/config files if available.
	migrated := defaultAISettingsDocument()
	if legacyWorkflow, e := readLegacySkillWorkflowConfig(); e == nil {
		migrated.Workflow = normalizeSkillWorkflowConfig(legacyWorkflow)
	}
	if legacyProfiles, e := readLegacyHabitProfiles(); e == nil && len(legacyProfiles) > 0 {
		migrated.HabitProfiles = normalizeHabitProfiles(legacyProfiles)
	}
	if legacySchema, e := readLegacyStrategySchema(); e == nil && len(legacySchema) > 0 {
		migrated.StrategyPackageSchema = legacySchema
	}
	_ = writeAISettingsDocument(migrated)
	return migrated
}

func writeAISettingsDocument(doc aiSettingsDocument) error {
	next := normalizeAISettingsDocument(doc)
	next.UpdatedAt = time.Now().Format(time.RFC3339)
	raw, err := json.MarshalIndent(next, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(aiSettingsPath), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(aiSettingsPath, append(raw, '\n'), 0o644); err != nil {
		return err
	}
	_ = syncLegacyAIReferenceFiles(next)
	return nil
}

func syncLegacyAIReferenceFiles(doc aiSettingsDocument) error {
	// Keep legacy references synchronized for compatibility/readability.
	habitDoc := legacyHabitProfilesDocument{
		Version:  defaultHabitProfileSchema,
		Profiles: normalizeHabitProfiles(doc.HabitProfiles),
	}
	habitRaw, err := json.MarshalIndent(habitDoc, "", "  ")
	if err == nil {
		_ = os.MkdirAll(filepath.Dir(legacyHabitProfilesPath), 0o755)
		_ = os.WriteFile(legacyHabitProfilesPath, append(habitRaw, '\n'), 0o644)
	}

	schema := doc.StrategyPackageSchema
	if len(schema) == 0 {
		schema = defaultStrategyPackageSchemaMap()
	}
	schemaRaw, err := json.MarshalIndent(schema, "", "  ")
	if err == nil {
		_ = os.MkdirAll(filepath.Dir(legacyStrategySchemaPath), 0o755)
		_ = os.WriteFile(legacyStrategySchemaPath, append(schemaRaw, '\n'), 0o644)
	}

	workflowRaw, err := json.MarshalIndent(normalizeSkillWorkflowConfig(doc.Workflow), "", "  ")
	if err == nil {
		_ = os.MkdirAll(filepath.Dir(legacySkillWorkflowPath), 0o755)
		_ = os.WriteFile(legacySkillWorkflowPath, append(workflowRaw, '\n'), 0o644)
	}
	return nil
}

func normalizeHabitProfiles(in []habitProfile) []habitProfile {
	defaults := defaultHabitProfiles()
	byKey := map[string]habitProfile{}
	for _, p := range defaults {
		byKey[normalizeHabitKey(p.Habit)] = p
	}

	seen := map[string]bool{}
	out := make([]habitProfile, 0, len(defaults))
	for _, row := range in {
		key := normalizeHabitKey(row.Habit)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		def, hasDef := byKey[key]
		cur := row
		cur.Habit = strings.TrimSpace(cur.Habit)
		if cur.Habit == "" {
			cur.Habit = def.Habit
		}
		if cur.Label == "" && hasDef {
			cur.Label = def.Label
		}
		if cur.Timeframe == "" && hasDef {
			cur.Timeframe = def.Timeframe
		}
		if cur.MaxLeverage <= 0 && hasDef {
			cur.MaxLeverage = def.MaxLeverage
		}
		if cur.MaxLeverage < 1 {
			cur.MaxLeverage = 1
		}
		if cur.MaxLeverage > 150 {
			cur.MaxLeverage = 150
		}
		if cur.MaxDrawdownPct <= 0 && hasDef {
			cur.MaxDrawdownPct = def.MaxDrawdownPct
		}
		if cur.MaxDrawdownPct <= 0 {
			cur.MaxDrawdownPct = 0.10
		}
		if cur.MaxDrawdownPct > 0.8 {
			cur.MaxDrawdownPct = 0.8
		}
		if cur.MaxRiskPerTrade <= 0 && hasDef {
			cur.MaxRiskPerTrade = def.MaxRiskPerTrade
		}
		if cur.MaxRiskPerTrade <= 0 {
			cur.MaxRiskPerTrade = 0.01
		}
		if cur.MaxRiskPerTrade > 0.2 {
			cur.MaxRiskPerTrade = 0.2
		}
		if cur.HoldBarsMin <= 0 && hasDef {
			cur.HoldBarsMin = def.HoldBarsMin
		}
		if cur.HoldBarsMin <= 0 {
			cur.HoldBarsMin = 1
		}
		if cur.HoldBarsMax <= 0 && hasDef {
			cur.HoldBarsMax = def.HoldBarsMax
		}
		if cur.HoldBarsMax < cur.HoldBarsMin {
			cur.HoldBarsMax = cur.HoldBarsMin
		}
		if cur.PreferredDataSpan <= 0 && hasDef {
			cur.PreferredDataSpan = def.PreferredDataSpan
		}
		if cur.PreferredDataSpan <= 0 {
			cur.PreferredDataSpan = 120
		}
		if cur.Description == "" && hasDef {
			cur.Description = def.Description
		}
		if cur.ExecutionHint == "" && hasDef {
			cur.ExecutionHint = def.ExecutionHint
		}
		out = append(out, cur)
	}

	if len(out) == 0 {
		return defaults
	}
	return out
}

func normalizeAISettingsDocument(in aiSettingsDocument) aiSettingsDocument {
	out := in
	if strings.TrimSpace(out.Version) == "" {
		out.Version = defaultAISettingsVersion
	}
	out.Workflow = normalizeSkillWorkflowConfig(out.Workflow)
	out.HabitProfiles = normalizeHabitProfiles(out.HabitProfiles)
	if len(out.StrategyPackageSchema) == 0 {
		out.StrategyPackageSchema = defaultStrategyPackageSchemaMap()
	}
	if strings.TrimSpace(out.UpdatedAt) == "" {
		out.UpdatedAt = time.Now().Format(time.RFC3339)
	}
	return out
}

func loadHabitProfiles() []habitProfile {
	doc := loadAISettingsDocument()
	return normalizeHabitProfiles(doc.HabitProfiles)
}

func normalizeHabitKey(v string) string {
	return strings.ToLower(strings.TrimSpace(v))
}
