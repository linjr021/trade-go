// @ts-nocheck
import {
  AntdApp,
  ConfigProvider,
} from '@/components/ui/dashboard-primitives'
import { OverviewCardsPanel } from '@/modules/overview-cards-panel'
import { TopToast } from '@/modules/top-toast'
import { LLMConfigModal, ExchangeConfigModal } from '@/modules/system-modals'
import { DashboardFrame } from '@/modules/dashboard-frame'
import { DashboardContent } from '@/modules/dashboard-content'
import { useDashboardController } from '@/modules/use-dashboard-controller'

export default function App() {
  const c = useDashboardController()

  const renderOverviewCards = (pair, strategyName, extra = null) => (
    <OverviewCardsPanel
      pair={pair}
      strategyName={strategyName}
      marketEmotion={c.marketEmotion}
      totalPnL={c.totalPnL}
      account={c.account}
      strategyDurationText={c.strategyDurationText}
      pnlRatio={c.pnlRatio}
      extra={extra}
      resolvedTheme={c.resolvedTheme}
      activeExchangeType={c.activeExchangeType}
    />
  )

  return (
    <ConfigProvider>
      <AntdApp>
        <>
          <TopToast toast={c.toast} />
          <DashboardFrame
            productName={c.productName}
            menu={c.menu}
            setMenu={c.setMenu}
            sidebarMenuItems={c.sidebarMenuItems}
            loading={c.loading}
            error={c.error}
            themeMode={c.themeMode}
            setThemeMode={c.setThemeMode}
          >
            <DashboardContent c={c} renderOverviewCards={renderOverviewCards} />
          </DashboardFrame>
          <LLMConfigModal
            open={c.showLLMModal}
            editingLLMId={c.editingLLMId}
            setShowLLMModal={c.setShowLLMModal}
            setEditingLLMId={c.setEditingLLMId}
            resetLLMModalDraft={c.resetLLMModalDraft}
            setNewLLM={c.setNewLLM}
            newLLM={c.newLLM}
            llmProductCatalog={c.llmProductCatalog}
            selectedLLMPreset={c.selectedLLMPreset}
            llmModelOptions={c.llmModelOptions}
            probingLLMModels={c.probingLLMModels}
            llmProbeMessage={c.llmProbeMessage}
            probeLLMModelOptions={c.probeLLMModelOptions}
            addingLLM={c.addingLLM}
            handleAddLLM={c.handleAddLLM}
          />
          <ExchangeConfigModal
            open={c.showExchangeModal}
            setShowExchangeModal={c.setShowExchangeModal}
            addingExchange={c.addingExchange}
            handleAddExchange={c.handleAddExchange}
            newExchange={c.newExchange}
            setNewExchange={c.setNewExchange}
          />
        </>
      </AntdApp>
    </ConfigProvider>
  )
}
