import { appendDashboardFilters } from '../api/urlFilters'
import { append, emptyState, error, grid, panel, statusBadge, statusLabel, text } from '../components/dom'
import type { ProjectListFixtureViewModel, ProjectPrimitiveViewModel, ProjectSyncStatus } from '../domain/dashboard'
import { deriveProjectHealth, sortProjectSummaries } from '../domain/projectHealth'
import type { ViewState } from './Overview'

export function renderProjects(state: ViewState<ProjectListFixtureViewModel>): HTMLElement {
  const card = panel('Projects')
  if (state.status === 'loading') return append(card, text('Loading projects…'))
  if (state.status === 'error') return error(card, state.message)
  const healthEvaluationDate = new Date(state.data.healthEvaluationDate)

  if (state.data.sourceLabel) card.append(sourceNotice(state.data.sourceLabel))

  if (state.data.projects.length === 0) {
    card.append(emptyState(`No project summaries are available. ${state.data.sourceLabel ?? 'Live project summaries are unavailable.'}`))
    return card
  }

  const list = grid(sortProjectSummaries(state.data.projects, healthEvaluationDate).map((project) => renderProjectCard(project, healthEvaluationDate)))
  list.classList.add('dashboard-project-grid')
  list.setAttribute('role', 'list')
  list.setAttribute('aria-label', 'Project summaries')
  card.append(list)
  return card
}

function renderProjectCard(project: ProjectPrimitiveViewModel, healthEvaluationDate: Date): HTMLElement {
  const health = deriveProjectHealth(project, healthEvaluationDate)
  const item = document.createElement('article')
  item.className = 'dashboard-project-card'
  item.setAttribute('role', 'listitem')
  item.setAttribute('aria-label', projectAriaLabel(project, health))
  item.append(
    heading(project.name),
    text(project.region),
    metric(`${formatCount(project.memoryCount)} memories`),
    metric(`${formatCount(project.contributorCount)} contributors`),
    metric(`Last sync: ${project.lastSyncLabel}`),
    healthRow(health),
    browseLink(project)
  )
  return item
}

function heading(value: string): HTMLHeadingElement {
  const node = document.createElement('h3')
  node.textContent = value
  return node
}

function metric(value: string): HTMLElement {
  return text(value, 'dashboard-project-card__metric')
}

function healthRow(health: ProjectSyncStatus): HTMLElement {
  const row = document.createElement('p')
  row.className = 'dashboard-project-card__health'
  row.append('Health: ', statusBadge(health))
  return row
}

function browseLink(project: ProjectPrimitiveViewModel): HTMLAnchorElement {
  const link = document.createElement('a')
  link.className = 'dashboard-control control dashboard-project-card__action'
  link.href = appendDashboardFilters('/dashboard/memories', { project: project.id })
  link.setAttribute('aria-label', `Browse memories for ${project.name}`)
  link.textContent = 'Browse memories'
  return link
}

function sourceNotice(message: string): HTMLElement {
  const notice = text(message, 'dashboard-source-note')
  notice.setAttribute('role', 'note')
  return notice
}

function projectAriaLabel(project: ProjectPrimitiveViewModel, health: ProjectSyncStatus): string {
  return `${project.name} project: ${statusLabel(health)} health, ${formatCount(project.memoryCount)} memories, ${formatCount(project.contributorCount)} contributors, last synced ${project.lastSyncLabel}`
}

function formatCount(value: number): string {
  return new Intl.NumberFormat('en-US').format(value)
}
