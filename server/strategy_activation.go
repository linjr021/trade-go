package server

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

func newGeneratedStrategyID(prefix string) string {
	p := strings.TrimSpace(prefix)
	if p == "" {
		p = "gs"
	}
	return fmt.Sprintf("%s_%d", p, time.Now().UnixNano())
}

func buildEnabledStrategiesWithPriority(primary string, current []string, maxCount int) []string {
	name := strings.TrimSpace(primary)
	if name == "" {
		return parseEnabledStrategiesEnv(strings.Join(current, ","))
	}
	if maxCount <= 0 {
		maxCount = 3
	}
	next := []string{name}
	for _, item := range parseEnabledStrategiesEnv(strings.Join(current, ",")) {
		if strings.EqualFold(strings.TrimSpace(item), name) {
			continue
		}
		next = append(next, item)
		if len(next) >= maxCount {
			break
		}
	}
	return next
}

func findGeneratedStrategyByID(items []generatedStrategyRecord, id string) (generatedStrategyRecord, bool) {
	target := strings.TrimSpace(id)
	if target == "" {
		return generatedStrategyRecord{}, false
	}
	for _, item := range items {
		if strings.TrimSpace(item.ID) == target {
			return item, true
		}
	}
	return generatedStrategyRecord{}, false
}

func (s *Service) saveAndActivateGeneratedStrategy(record generatedStrategyRecord) (generatedStrategyRecord, []string, generatedStrategyStore, error) {
	now := time.Now().Format(time.RFC3339)
	candidate := record
	if strings.TrimSpace(candidate.ID) == "" {
		candidate.ID = newGeneratedStrategyID("workflow")
	}
	candidate.Name = strings.TrimSpace(candidate.Name)
	if candidate.Name == "" {
		candidate.Name = "未命名策略_" + strconv.FormatInt(time.Now().Unix(), 10)
	}
	if strings.TrimSpace(candidate.CreatedAt) == "" {
		candidate.CreatedAt = now
	}
	candidate.LastUpdatedAt = now

	store := readGeneratedStrategies()
	candidateNameKey := strings.ToLower(strings.TrimSpace(candidate.Name))
	candidateRuleKey := strings.ToLower(strings.TrimSpace(candidate.RuleKey))
	for _, item := range store.Strategies {
		itemNameKey := strings.ToLower(strings.TrimSpace(item.Name))
		itemRuleKey := strings.ToLower(strings.TrimSpace(item.RuleKey))
		matchByName := candidateNameKey != "" && itemNameKey == candidateNameKey
		matchByRuleKey := candidateRuleKey != "" && itemRuleKey != "" && itemRuleKey == candidateRuleKey
		if !matchByName && !matchByRuleKey {
			continue
		}
		// 同规则覆盖时，复用原ID与创建时间，避免前端视角出现“新增一条”
		if strings.TrimSpace(item.ID) != "" {
			candidate.ID = strings.TrimSpace(item.ID)
		}
		if strings.TrimSpace(item.CreatedAt) != "" {
			candidate.CreatedAt = strings.TrimSpace(item.CreatedAt)
		}
		break
	}

	filtered := make([]generatedStrategyRecord, 0, len(store.Strategies)+1)
	for _, item := range store.Strategies {
		itemID := strings.TrimSpace(item.ID)
		if itemID == strings.TrimSpace(candidate.ID) {
			continue
		}
		itemNameKey := strings.ToLower(strings.TrimSpace(item.Name))
		if candidateNameKey != "" && itemNameKey == candidateNameKey {
			continue
		}
		itemRuleKey := strings.ToLower(strings.TrimSpace(item.RuleKey))
		if candidateRuleKey != "" && itemRuleKey != "" && itemRuleKey == candidateRuleKey {
			continue
		}
		filtered = append(filtered, item)
	}
	store.Strategies = append([]generatedStrategyRecord{candidate}, filtered...)
	if len(store.Strategies) > 300 {
		store.Strategies = store.Strategies[:300]
	}
	if err := writeGeneratedStrategies(store); err != nil {
		return generatedStrategyRecord{}, nil, generatedStrategyStore{}, err
	}

	finalStore := readGeneratedStrategies()
	final, ok := findGeneratedStrategyByID(finalStore.Strategies, candidate.ID)
	if !ok {
		if len(finalStore.Strategies) == 0 {
			return generatedStrategyRecord{}, nil, generatedStrategyStore{}, fmt.Errorf("generated strategy 持久化后为空")
		}
		final = finalStore.Strategies[0]
	}

	currentEnabled := parseEnabledStrategiesEnv("")
	nextEnabled := buildEnabledStrategiesWithPriority(final.Name, currentEnabled, 3)
	updates := map[string]string{
		executionStrategiesEnvKey: strings.Join(nextEnabled, ","),
	}
	if err := upsertDotEnv(".env", updates); err != nil {
		return generatedStrategyRecord{}, nil, generatedStrategyStore{}, err
	}
	for k, v := range updates {
		_ = os.Setenv(k, v)
	}
	applyRuntimeConfigFromEnv()

	return final, nextEnabled, finalStore, nil
}
