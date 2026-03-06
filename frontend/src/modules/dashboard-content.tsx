import { AssetsPageSection } from '@/modules/assets-page-section'
import { AdvancedSettingsSection } from '@/modules/advanced-settings-section'
import { AuthAdminPanel } from '@/modules/auth-admin-panel'
import { BacktestPageSection, BuilderPageSection, SkillWorkflowPageSection } from '@/modules/builder-backtest-sections'
import { LivePageSection, PaperPageSection } from '@/modules/live-paper-sections'
import { SystemPageSection } from '@/modules/system-page-section'
import {
  ASSET_MIN_MONTH,
  BACKTEST_MAX_MONTH,
  BACKTEST_MIN_MONTH,
  envFieldGroups,
  HISTORY_MAX_MONTH,
  PAIRS,
} from '@/modules/constants'
import { normalizeDecimal, normalizeLeverage } from '@/modules/trade-utils'
import { fmtNum, fmtPct, fmtTime } from '@/modules/format'

export function DashboardContent({ c, renderOverviewCards }) {
  const autoReviewGroup = envFieldGroups.find((group) => String(group?.title || '') === '自动评估')
  const autoReviewFields = Array.isArray(autoReviewGroup?.fields) ? autoReviewGroup.fields : []
  const systemEnvGroups = envFieldGroups.filter((group) => String(group?.title || '') !== '自动评估')

  return (
    <>
      {c.menu === 'live' && (
        <LivePageSection
          activePair={c.activePair}
          setActivePair={c.setActivePair}
          pairs={PAIRS}
          strategyPickerRef={c.strategyPickerRef}
          strategyPickerOpen={c.strategyPickerOpen}
          setStrategyPickerOpen={c.setStrategyPickerOpen}
          enabledStrategies={c.enabledStrategies}
          strategyMetaMap={c.strategyMetaMap}
          setStrategyDraft={c.setStrategyDraft}
          selectedStrategyText={c.selectedStrategyText}
          executionStrategyOptions={c.executionStrategyOptions}
          strategyDraft={c.strategyDraft}
          toggleStrategyDraft={c.toggleStrategyDraft}
          confirmStrategySelection={c.confirmStrategySelection}
          settings={c.settings}
          setSettings={c.setSettings}
          normalizeDecimal={normalizeDecimal}
          normalizeLeverage={normalizeLeverage}
          refreshCore={c.refreshCore}
          runningNow={c.runningNow}
          runOneCycle={c.runOneCycle}
          startLiveTrading={c.startLiveTrading}
          startingLive={c.startingLive}
          toggleScheduler={c.toggleScheduler}
          schedulerRunning={c.schedulerRunning}
          savingSettings={c.savingSettings}
          saveLiveConfig={c.saveLiveConfig}
          status={c.status}
          liveViewTab={c.liveViewTab}
          setLiveViewTab={c.setLiveViewTab}
          renderOverviewCards={renderOverviewCards}
          liveStrategyLabel={c.liveStrategyLabel}
          liveStrategyHistory={c.liveStrategyHistory}
          liveMarketSnapshot={c.liveMarketSnapshot}
          tradeRecords={c.tradeRecords}
        />
      )}

      {c.menu === 'paper' && (
        <PaperPageSection
          paperPair={c.paperPair}
          setPaperPair={c.setPaperPair}
          pairs={PAIRS}
          paperStrategyPickerRef={c.paperStrategyPickerRef}
          paperStrategyPickerOpen={c.paperStrategyPickerOpen}
          setPaperStrategyPickerOpen={c.setPaperStrategyPickerOpen}
          paperStrategySelection={c.paperStrategySelection}
          strategyMetaMap={c.strategyMetaMap}
          setPaperStrategyDraft={c.setPaperStrategyDraft}
          paperSelectedStrategyText={c.paperSelectedStrategyText}
          executionStrategyOptions={c.executionStrategyOptions}
          paperStrategyDraft={c.paperStrategyDraft}
          togglePaperStrategyDraft={c.togglePaperStrategyDraft}
          confirmPaperStrategySelection={c.confirmPaperStrategySelection}
          paperMargin={c.paperMargin}
          setPaperMargin={c.setPaperMargin}
          paperIntervalSec={c.paperIntervalSec}
          setPaperIntervalSec={c.setPaperIntervalSec}
          paperSettings={c.paperSettings}
          setPaperSettings={c.setPaperSettings}
          normalizeDecimal={normalizeDecimal}
          normalizeLeverage={normalizeLeverage}
          startPaperSim={c.startPaperSim}
          paperSimLoading={c.paperSimLoading}
          paperSimRunning={c.paperSimRunning}
          pausePaperSim={c.pausePaperSim}
          paperViewTab={c.paperViewTab}
          setPaperViewTab={c.setPaperViewTab}
          renderOverviewCards={renderOverviewCards}
          paperTradeRecords={c.paperTradeRecords}
          paperLatestDecision={c.paperLatestDecision}
          paperStrategyHistory={c.paperStrategyHistory}
          paperPnlBaselineMap={c.paperPnlBaselineMap}
          resetPaperCurrentPnL={c.resetPaperCurrentPnL}
        />
      )}

      {c.menu === 'assets' && (
        <AssetsPageSection
          assetOverview={c.assetOverview}
          assetDistribution={c.assetDistribution}
          assetMonth={c.assetMonth}
          setAssetMonth={c.setAssetMonth}
          assetCalendar={c.assetCalendar}
          assetRange={c.assetRange}
          setAssetRange={c.setAssetRange}
          assetTrend={c.assetTrend}
          minMonth={ASSET_MIN_MONTH}
          maxMonth={HISTORY_MAX_MONTH}
        />
      )}

      {c.menu === 'builder' && (
        <BuilderPageSection
          builderTab={c.builderTab}
          setBuilderTab={c.setBuilderTab}
          strategyGenMode={c.strategyGenMode}
          habit={c.habit}
          setHabit={c.setHabit}
          habitOptions={c.habitOptions}
          genPair={c.genPair}
          setGenPair={c.setGenPair}
          pairs={PAIRS}
          genStyle={c.genStyle}
          setGenStyle={c.setGenStyle}
          genMinRR={c.genMinRR}
          setGenMinRR={c.setGenMinRR}
          genLowConfAction={c.genLowConfAction}
          setGenLowConfAction={c.setGenLowConfAction}
          genDirectionBias={c.genDirectionBias}
          setGenDirectionBias={c.setGenDirectionBias}
          genAllowReversal={c.genAllowReversal}
          setGenAllowReversal={c.setGenAllowReversal}
          generateStrategy={c.generateStrategy}
          generatingStrategy={c.generatingStrategy}
          generatedStrategies={c.generatedStrategies}
          selectedRuleId={c.selectedRuleId}
          setSelectedRuleId={c.setSelectedRuleId}
          selectedRule={c.selectedRule}
          renameRuleName={c.renameRuleName}
          setRenameRuleName={c.setRenameRuleName}
          renameGeneratedStrategy={c.renameGeneratedStrategy}
          deleteGeneratedStrategy={c.deleteGeneratedStrategy}
          renameHanCount={c.renameHanCount}
          fmtNum={fmtNum}
        />
      )}

      {c.menu === 'skill_workflow' && (
        <SkillWorkflowPageSection
          skillWorkflow={c.skillWorkflow}
          loadingSkillWorkflow={c.loadingSkillWorkflow}
          savingSkillWorkflow={c.savingSkillWorkflow}
          runningWorkflowUpgradeNow={c.runningWorkflowUpgradeNow}
          aiWorkflowTab={c.aiWorkflowTab}
          setAiWorkflowTab={c.setAiWorkflowTab}
          aiWorkflowLogs={c.aiWorkflowLogs}
          aiWorkflowLogsLoading={c.aiWorkflowLogsLoading}
          aiWorkflowLogChannel={c.aiWorkflowLogChannel}
          setAiWorkflowLogChannel={c.setAiWorkflowLogChannel}
          aiWorkflowLogLimit={c.aiWorkflowLogLimit}
          setAiWorkflowLogLimit={c.setAiWorkflowLogLimit}
          coreRiskSettings={c.coreRiskSettings}
          setCoreRiskField={c.setCoreRiskField}
          savingCoreRiskSettings={c.savingCoreRiskSettings}
          coreRiskSaveHint={c.coreRiskSaveHint}
          applyingRiskPreset={c.applyingRiskPreset}
          applyRiskPreset={c.applyRiskPreset}
          saveCoreRiskSettings={c.saveCoreRiskSettings}
          resettingRiskBaseline={c.resettingRiskBaseline}
          resetRiskManually={c.resetRiskManually}
          resettingPaperRiskBaseline={c.resettingPaperRiskBaseline}
          resetPaperRiskManually={c.resetPaperRiskManually}
          autoReviewFields={autoReviewFields}
          systemSettings={c.systemSettings}
          setSystemSettings={c.setSystemSettings}
          autoReviewSaveHint={c.autoReviewSaveHint}
          savingAutoReviewSettings={c.savingAutoReviewSettings}
          saveAutoReviewEnv={c.saveAutoReviewEnv}
          updateSkillStepField={c.updateSkillStepField}
          updateSkillConstraintField={c.updateSkillConstraintField}
          updateSkillPromptField={c.updateSkillPromptField}
          saveSkillWorkflowConfig={c.saveSkillWorkflowConfig}
          resetSkillWorkflowConfig={c.resetSkillWorkflowConfig}
          runWorkflowUpgradeNow={c.runWorkflowUpgradeNow}
          loadSkillWorkflowConfig={c.loadSkillWorkflowConfig}
          loadAIWorkflowLogs={c.loadAIWorkflowLogs}
          fmtNum={fmtNum}
          fmtTime={fmtTime}
        />
      )}

      {c.menu === 'advanced' && (
        <AdvancedSettingsSection
          advancedTab={c.advancedTab}
          setAdvancedTab={c.setAdvancedTab}
          systemSettings={c.systemSettings}
          setSystemSettings={c.setSystemSettings}
          savingAdvancedEnvSettings={c.savingAdvancedEnvSettings}
          saveAdvancedEnvSettings={c.saveAdvancedEnvSettings}
          advancedEnvSaveHint={c.advancedEnvSaveHint}
          habitProfilesJSON={c.habitProfilesJSON}
          setHabitProfilesJSON={c.setHabitProfilesJSON}
          savingHabitProfiles={c.savingHabitProfiles}
          saveHabitProfilesConfig={c.saveHabitProfilesConfig}
          resetHabitProfilesToDefault={c.resetHabitProfilesToDefault}
          strategySchemaJSON={c.strategySchemaJSON}
          setStrategySchemaJSON={c.setStrategySchemaJSON}
          savingStrategySchema={c.savingStrategySchema}
          saveStrategySchemaConfig={c.saveStrategySchemaConfig}
          resetStrategySchemaToDefault={c.resetStrategySchemaToDefault}
          loadAdvancedContent={c.loadAdvancedContent}
        />
      )}

      {c.menu === 'backtest' && (
        <BacktestPageSection
          btStrategyPickerRef={c.btStrategyPickerRef}
          btStrategyPickerOpen={c.btStrategyPickerOpen}
          setBtStrategyPickerOpen={c.setBtStrategyPickerOpen}
          btStrategySelection={c.btStrategySelection}
          setBtStrategyDraft={c.setBtStrategyDraft}
          btSelectedStrategyText={c.btSelectedStrategyText}
          executionStrategyOptions={c.executionStrategyOptions}
          btStrategyDraft={c.btStrategyDraft}
          toggleBtStrategyDraft={c.toggleBtStrategyDraft}
          confirmBtStrategySelection={c.confirmBtStrategySelection}
          btPair={c.btPair}
          setBtPair={c.setBtPair}
          pairs={PAIRS}
          btInitialMargin={c.btInitialMargin}
          setBtInitialMargin={c.setBtInitialMargin}
          btLeverage={c.btLeverage}
          setBtLeverage={c.setBtLeverage}
          normalizeLeverage={normalizeLeverage}
          btPositionSizingMode={c.btPositionSizingMode}
          setBtPositionSizingMode={c.setBtPositionSizingMode}
          btHighConfidenceAmount={c.btHighConfidenceAmount}
          setBtHighConfidenceAmount={c.setBtHighConfidenceAmount}
          btLowConfidenceAmount={c.btLowConfidenceAmount}
          setBtLowConfidenceAmount={c.setBtLowConfidenceAmount}
          btHighConfidenceMarginPct={c.btHighConfidenceMarginPct}
          setBtHighConfidenceMarginPct={c.setBtHighConfidenceMarginPct}
          btLowConfidenceMarginPct={c.btLowConfidenceMarginPct}
          setBtLowConfidenceMarginPct={c.setBtLowConfidenceMarginPct}
          normalizeDecimal={normalizeDecimal}
          btStart={c.btStart}
          setBtStart={c.setBtStart}
          btEnd={c.btEnd}
          setBtEnd={c.setBtEnd}
          backtestMinMonth={BACKTEST_MIN_MONTH}
          backtestMaxMonth={BACKTEST_MAX_MONTH}
          runBacktest={c.runBacktest}
          btRunning={c.btRunning}
          btHistoryLoading={c.btHistoryLoading}
          btHistory={c.btHistory}
          btHistorySelectedId={c.btHistorySelectedId}
          viewBacktestHistoryDetail={c.viewBacktestHistoryDetail}
          fmtNum={fmtNum}
          fmtTime={fmtTime}
          selectedBacktestHistory={c.selectedBacktestHistory}
          fmtPct={fmtPct}
          btSummary={c.btSummary}
          btHistoryDeletingId={c.btHistoryDeletingId}
          removeBacktestHistory={c.removeBacktestHistory}
          btRecords={c.btRecords}
        />
      )}

      {c.menu === 'auth_admin' && (
        <AuthAdminPanel inline />
      )}

      {c.menu === 'system' && (
        <SystemPageSection
          systemSubTab={c.systemSubTab}
          setSystemSubTab={c.setSystemSubTab}
          loadingSystemRuntime={c.loadingSystemRuntime}
          loadSystemRuntime={c.loadSystemRuntime}
          restartingBackend={c.restartingBackend}
          restartBackend={c.restartBackend}
          runtimeComponents={c.runtimeComponents}
          systemRuntime={c.systemRuntime}
          fmtNum={fmtNum}
          fmtTime={fmtTime}
          envFieldGroups={systemEnvGroups}
          systemSettings={c.systemSettings}
          setSystemSettings={c.setSystemSettings}
          systemSaveHint={c.systemSaveHint}
          savingSystemSettings={c.savingSystemSettings}
          saveSystemEnv={c.saveSystemEnv}
          setEditingLLMId={c.setEditingLLMId}
          resetLLMModalDraft={c.resetLLMModalDraft}
          setShowLLMModal={c.setShowLLMModal}
          llmProductCatalog={c.llmProductCatalog}
          llmConfigs={c.llmConfigs}
          testingLLMId={c.testingLLMId}
          llmStatusMap={c.llmStatusMap}
          testLLMConfigReachability={c.testLLMConfigReachability}
          openEditLLMModal={c.openEditLLMModal}
          deletingLLMId={c.deletingLLMId}
          removeLLMConfig={c.removeLLMConfig}
          setShowExchangeModal={c.setShowExchangeModal}
          exchangeBound={c.exchangeBound}
          activeExchangeId={c.activeExchangeId}
          exchangeConfigs={c.exchangeConfigs}
          activatingExchangeId={c.activatingExchangeId}
          bindExchangeAccount={c.bindExchangeAccount}
          deletingExchangeId={c.deletingExchangeId}
          removeExchangeAccount={c.removeExchangeAccount}
        />
      )}
    </>
  )
}
