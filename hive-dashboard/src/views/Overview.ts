import type { MetricCardViewModel, OverviewCommonViewModel, OverviewLiveActivityViewModel, OverviewSyncHealthProjectViewModel, OverviewViewModel, ProjectSyncStatus } from '../domain/dashboard'
import { renderChart } from '../components/Chart'
import { append, emptyState, error, statusBadge, statusLabel, text } from '../components/dom'

export type ViewState<T> = { status: 'loading' } | { status: 'error'; message: string } | { status: 'ready'; data: T }

const SYNC_HEALTH_OVERVIEW_PROJECT_LIMIT = 5
const SYNC_HEALTH_PRIORITY: Record<ProjectSyncStatus, number> = {
  degraded: 0,
  unknown: 1,
  healthy: 2
}

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

function renderSyncHealthRow(project: OverviewSyncHealthProjectViewModel): HTMLTableRowElement {
  const row = document.createElement('tr')
  const region = project.region.trim()
  const contributors = contributorLabel(project.contributorCount)
  const ariaParts = [
    `${project.name}: ${statusLabel(project.status)}`,
    contributors,
    project.lastActivityLabel,
    ...(region ? [`region ${region}`] : [])
  ]
  row.className = 'dashboard-sync-health__row'
  row.setAttribute('role', 'row')
  row.setAttribute('aria-label', ariaParts.join(', '))
  row.title = ariaParts.join(' · ')

  const badge = statusBadge(project.status)
  badge.textContent = statusLabel(project.status).toUpperCase()
  return append(row, tableCell(badge, 'dashboard-sync-health__status'), tableCell(project.name, 'dashboard-sync-health__project'), tableCell(contributors, 'dashboard-sync-health__metric'), tableCell(project.lastActivityLabel, 'dashboard-sync-health__activity'))
}

function contributorLabel(count: number): string {
  return `${count} ${count === 1 ? 'contributor' : 'contributors'}`
}

function tableCell(content: string | HTMLElement, className: string): HTMLTableCellElement {
  const cell = document.createElement('td')
  cell.className = className
  cell.setAttribute('role', 'cell')
  cell.append(content)
  return cell
}

function renderSyncHealthSection(projects: readonly OverviewSyncHealthProjectViewModel[], sourceLabel?: string): HTMLElement {
  const section = document.createElement('section')
  section.className = 'dashboard-sync-health'
  section.setAttribute('role', 'region')
  section.setAttribute('aria-label', 'Sync health by project')

  if (sourceLabel) section.append(sourceNotice(sourceLabel))

  if (projects.length === 0) {
    section.append(emptyState('No project sync health data is available.'))
    return section
  }

  const visibleProjects = sortSyncHealthProjects(projects).slice(0, SYNC_HEALTH_OVERVIEW_PROJECT_LIMIT)
  const table = document.createElement('table')
  table.className = 'dashboard-sync-health__table'
  table.setAttribute('aria-label', 'Sync health by project')
  table.append(syncHealthTableHeader(), tableBody(visibleProjects.map(renderSyncHealthRow)))
  section.append(table, syncHealthFooter(visibleProjects.length, projects.length))

  return section
}

function syncHealthTableHeader(): HTMLTableSectionElement {
  const head = document.createElement('thead')
  const row = document.createElement('tr')
  row.setAttribute('role', 'row')
  for (const label of ['Status', 'Project', 'Contributors', 'Last sync']) {
    const header = document.createElement('th')
    header.scope = 'col'
    header.setAttribute('role', 'columnheader')
    header.textContent = label
    row.append(header)
  }
  head.append(row)
  return head
}

function tableBody(rows: readonly HTMLTableRowElement[]): HTMLTableSectionElement {
  const body = document.createElement('tbody')
  body.append(...rows)
  return body
}

function syncHealthFooter(visibleProjectCount: number, totalProjectCount: number): HTMLElement {
  const footer = document.createElement('footer')
  footer.className = 'dashboard-sync-health__footer'
  const count = text(`Showing ${visibleProjectCount} of ${totalProjectCount}`, 'dashboard-sync-health__count')
  const link = document.createElement('a')
  link.href = '/dashboard/projects'
  link.setAttribute('aria-label', 'View all projects')
  link.textContent = 'View all projects →'
  footer.append(count, link)
  return footer
}

function sortSyncHealthProjects(projects: readonly OverviewSyncHealthProjectViewModel[]): OverviewSyncHealthProjectViewModel[] {
  return [...projects].sort((left, right) => {
    const statusComparison = SYNC_HEALTH_PRIORITY[left.status] - SYNC_HEALTH_PRIORITY[right.status]
    if (statusComparison !== 0) return statusComparison

    const activityComparison = activityAgeInMinutes(left.lastActivityLabel) - activityAgeInMinutes(right.lastActivityLabel)
    if (activityComparison !== 0) return activityComparison
    return left.name.localeCompare(right.name, undefined, { sensitivity: 'accent' })
  })
}

function activityAgeInMinutes(label: string): number {
  const normalized = label.trim().toLowerCase()
  if (normalized === 'just now') return 0
  const match = normalized.match(/^(\d+)([mhd]) ago$/)
  if (!match) return Number.POSITIVE_INFINITY
  const value = Number(match[1])
  if (match[2] === 'h') return value * 60
  if (match[2] === 'd') return value * 24 * 60
  return value
}

function sourceNotice(message: string): HTMLElement {
  const notice = text(message, 'dashboard-source-note')
  notice.setAttribute('role', 'note')
  return notice
}

function renderLiveActivity(activity: OverviewLiveActivityViewModel): HTMLElement {
  const section = document.createElement('section')
  section.setAttribute('role', 'region')
  section.setAttribute('aria-label', 'Live activity')
  if (activity.count <= 0) {
    section.append(emptyState('No recent activity is available.'))
    return section
  }
  section.append(text(`${activity.count} recent sync ${activity.count === 1 ? 'event' : 'events'}`))
  if (activity.newestSyncId?.trim()) section.append(text(`Newest sync: ${activity.newestSyncId}`))
  return section
}
function renderMostActiveProjects(points: OverviewCommonViewModel['mostActiveProjects']): HTMLElement {
  return renderChart({
    kind: 'categorical',
    title: 'Most active projects',
    points
  })
}

export function renderOverview(state: ViewState<OverviewViewModel>): HTMLElement {
  const root = document.createElement('section')
  root.className = 'dashboard-overview'
  root.dataset.dashboardView = 'overview'
  // Expose as a region for a11y + test compatibility (loading/error states)
  root.setAttribute('role', 'region')
  root.setAttribute('aria-label', 'Hive Overview')

  if (state.status === 'loading') {
    root.append(text('Loading overview…'))
    return root
  }
  if (state.status === 'error') {
    return error(root, state.message)
  }

  const { totalMemories, activeProjects, liveActivity, mostActiveProjects } = state.data
  const statsRow = document.createElement('div')
  statsRow.className = 'dashboard-overview__stats'
  statsRow.append(
    statTile({ label: totalMemories.label, value: totalMemories.displayValue ?? String(totalMemories.value), detail: totalMemories.sourceLabel, accent: '#3B82E8', href: '/dashboard/knowledgeBrowser', destination: 'Knowledge Browser' }),
    statTile({ label: activeProjects.label, value: activeProjects.displayValue ?? String(activeProjects.value), detail: activeProjects.sourceLabel, accent: '#22B85C', href: '/dashboard/projects', destination: 'Projects' })
  )

  const row = document.createElement('div')
  row.className = 'dashboard-overview__row'
  row.append(flushPanel('Live activity', renderLiveActivity(liveActivity)), flushPanel('Most active projects', renderMostActiveProjects(mostActiveProjects)))

  if (state.data.capability === 'admin') {
    const { syncingUsers, degradedProjects, knowledgeGrowth, syncHealthByProject, syncHealthByProjectSourceLabel } = state.data
    statsRow.append(
      statTile({ label: syncingUsers.label, value: syncHealthDisplay(syncingUsers), detail: syncingUsers.sourceLabel, accent: '#22B85C', href: '/dashboard/userManagement', destination: 'User Management' }),
      statTile({ label: degradedProjects.label, value: degradedProjects.displayValue ?? String(degradedProjects.value), detail: degradedProjects.sourceLabel, accent: '#E0246F', href: '/dashboard/projects?health=degraded', destination: 'degraded Projects' })
    )
    const operations = document.createElement('div')
    operations.className = 'dashboard-overview__row'
    operations.append(
      flushPanel('Knowledge growth', renderChart({ kind: 'time-series', title: knowledgeGrowth.label, series: knowledgeGrowth })),
      flushPanel('Sync health by project', renderSyncHealthSection(syncHealthByProject, syncHealthByProjectSourceLabel))
    )
    return append(root, statsRow, operations, row)
  }

  return append(root, statsRow, row)
}

function statTile(input: { label: string; value: string; detail?: string; accent?: string; href: string; destination: string }): HTMLElement {
  const metric = document.createElement('a')
  metric.className = 'dashboard-metric metric'
  metric.dataset.dashboardPrimitive = 'metric'
  metric.href = input.href
  metric.setAttribute('aria-label', `${input.label}: ${input.value}. View ${input.destination}`)

  // Label row with optional accent chip
  const labelRow = document.createElement('div')
  labelRow.className = 'dashboard-metric-label-row'

  if (input.accent) {
    const chip = document.createElement('span')
    chip.className = 'dashboard-tile__accent'
    chip.style.background = input.accent
    chip.setAttribute('aria-hidden', 'true')
    labelRow.append(chip)
  }

  const label = document.createElement('p')
  label.className = 'dashboard-metric-label'
  label.textContent = input.label
  labelRow.append(label)

  const value = document.createElement('p')
  value.className = 'dashboard-metric-value'
  value.textContent = input.value

  metric.append(labelRow, value)
  if (input.detail) {
    const detail = document.createElement('p')
    detail.className = 'dashboard-metric-detail'
    detail.textContent = input.detail
    metric.append(detail)
  }

  return metric
}

function flushPanel(title: string, content: HTMLElement): HTMLElement {
  const section = document.createElement('section')
  section.className = 'dashboard-panel panel dashboard-panel--flush'
  section.dataset.dashboardPrimitive = 'panel'

  // Header bar — 44px height, mono uppercase title
  const header = document.createElement('div')
  header.className = 'dashboard-panel__header'

  const titleEl = document.createElement('h2')
  titleEl.className = 'dashboard-panel__title'
  titleEl.textContent = title
  header.append(titleEl)

  // Body — flush variant: no padding (content manages its own spacing)
  const body = document.createElement('div')
  body.className = 'dashboard-panel__body'
  body.append(content)

  section.append(header, body)
  return section
}
