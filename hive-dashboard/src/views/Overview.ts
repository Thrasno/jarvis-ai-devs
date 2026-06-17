import type { MetricCardViewModel, OverviewFixtureViewModel, ProjectPrimitiveViewModel } from '../domain/dashboard'
import { renderChart } from '../components/Chart'
import { append, error, grid, metricCard, panel, stack, statusBadge, text } from '../components/dom'

export type ViewState<T> = { status: 'loading' } | { status: 'error'; message: string } | { status: 'ready'; data: T }

function syncHealthDisplay(vm: MetricCardViewModel): string {
  const raw = vm.displayValue ?? String(vm.value)
  const parts = raw.split('/')
  if (parts.length !== 2) return raw
  const total = parseInt(parts[1], 10)
  if (isNaN(total) || total === 0) return raw
  const pct = Math.round((vm.value / total) * 100)
  return `${pct}% · ${raw}`
}

function metricCardFromViewModel(vm: MetricCardViewModel): HTMLElement {
  return metricCard({ label: vm.label, value: vm.displayValue ?? String(vm.value) })
}

function renderSyncHealthRow(project: ProjectPrimitiveViewModel): HTMLElement {
  return append(
    document.createElement('div'),
    text(''),
    statusBadge(project.status),
    text(project.name),
    text(String(project.memoryCount)),
    text(project.lastSyncLabel)
  )
}

export function renderOverview(state: ViewState<OverviewFixtureViewModel>): HTMLElement {
  const card = panel('Hive Overview')
  if (state.status === 'loading') return append(card, text('Loading overview…'))
  if (state.status === 'error') return error(card, state.message)
  const { totalMemories, activeProjects, healthyDaemons, knowledgeGrowth, syncHealthByProject } = state.data
  const openConflictsCard = metricCard({ label: 'Open Conflicts', value: '0' })
  return append(
    card,
    grid([
      metricCardFromViewModel(totalMemories),
      metricCardFromViewModel(activeProjects),
      metricCard({ label: healthyDaemons.label, value: syncHealthDisplay(healthyDaemons) }),
      openConflictsCard
    ]),
    renderChart({
      kind: 'time-series',
      title: knowledgeGrowth.label,
      summary: 'Knowledge growth over time.',
      series: knowledgeGrowth
    }),
    stack(syncHealthByProject.map(renderSyncHealthRow))
  )
}
