import type { MetricCardViewModel, OverviewFixtureViewModel, ProjectPrimitiveViewModel } from '../domain/dashboard'
import { renderChart } from '../components/Chart'
import { append, emptyState, error, grid, metricCard, panel, stack, statusBadge, statusLabel, text } from '../components/dom'

export type ViewState<T> = { status: 'loading' } | { status: 'error'; message: string } | { status: 'ready'; data: T }

function syncHealthDisplay(vm: MetricCardViewModel): string {
  if (vm.totalValue !== undefined && vm.totalValue > 0) {
    const pct = Math.round((vm.value / vm.totalValue) * 100)
    return `${pct}% · ${vm.value}/${vm.totalValue}`
  }
  const raw = vm.displayValue ?? String(vm.value)
  const parts = raw.split('/')
  if (parts.length !== 2) return raw
  const total = parseInt(parts[1], 10)
  if (isNaN(total) || total === 0) return raw
  const pct = Math.round((vm.value / total) * 100)
  return `${pct}% · ${raw}`
}

function metricCardFromViewModel(vm: MetricCardViewModel): HTMLElement {
  return metricCard({ label: vm.label, value: vm.displayValue ?? String(vm.value), detail: vm.sourceLabel })
}

function renderSyncHealthRow(project: ProjectPrimitiveViewModel): HTMLElement {
  const row = document.createElement('div')
  row.setAttribute('role', 'listitem')
  row.setAttribute(
    'aria-label',
    `${project.name}: ${statusLabel(project.status)}, ${project.memoryCount} memories, last synced ${project.lastSyncLabel}`
  )
  return append(
    row,
    text(''),
    statusBadge(project.status),
    text(project.name),
    text(String(project.memoryCount)),
    text(project.lastSyncLabel)
  )
}

function renderSyncHealthSection(projects: readonly ProjectPrimitiveViewModel[], sourceLabel?: string): HTMLElement {
  const section = document.createElement('section')
  section.setAttribute('role', 'region')
  section.setAttribute('aria-label', 'Sync health by project')

  if (sourceLabel) section.append(sourceNotice(sourceLabel))

  if (projects.length === 0) {
    section.append(emptyState(`No project sync health data is available. ${sourceLabel ?? 'Live per-project sync health is unavailable.'}`))
    return section
  }

  const list = stack(projects.map(renderSyncHealthRow))
  list.setAttribute('role', 'list')
  list.setAttribute('aria-label', 'Sync health by project rows')
  section.append(list)
  return section
}

function sourceNotice(message: string): HTMLElement {
  const notice = text(message, 'dashboard-source-note')
  notice.setAttribute('role', 'note')
  return notice
}

function sourceAwareSummary(summary: string, sourceLabel?: string): string {
  return sourceLabel ? `${summary} ${sourceLabel}` : summary
}

export function renderOverview(state: ViewState<OverviewFixtureViewModel>): HTMLElement {
  const card = panel('Hive Overview')
  if (state.status === 'loading') return append(card, text('Loading overview…'))
  if (state.status === 'error') return error(card, state.message)
  const { totalMemories, activeProjects, healthyDaemons, openConflicts, knowledgeGrowth, syncHealthByProject, syncHealthByProjectSourceLabel } = state.data
  return append(
    card,
    grid([
      metricCardFromViewModel(totalMemories),
      metricCardFromViewModel(activeProjects),
      metricCard({ label: healthyDaemons.label, value: syncHealthDisplay(healthyDaemons), detail: healthyDaemons.sourceLabel }),
      metricCardFromViewModel(openConflicts)
    ]),
    renderChart({
      kind: 'time-series',
      title: knowledgeGrowth.label,
      summary: sourceAwareSummary('Knowledge growth over time.', knowledgeGrowth.sourceLabel),
      series: knowledgeGrowth
    }),
    renderSyncHealthSection(syncHealthByProject, syncHealthByProjectSourceLabel)
  )
}
