import { append, emptyState, grid, panel, statusBadge, statusLabel, text } from '../components/dom'
import type { ProjectListViewModel, ProjectLiveSummaryViewModel } from '../domain/dashboard'
import type { ViewState } from './Overview'

export function renderProjects(state: ViewState<ProjectListViewModel>): HTMLElement {
  const card = panel('Projects')
  if (state.status === 'loading') return append(card, statusText('Loading live project summaries…'))
  if (state.status === 'error') return append(card, alertText(state.message))

  if (state.data.projects.length === 0) {
    card.append(emptyState('No live project summaries found.'))
    return card
  }

  const list = grid(state.data.projects.map(renderProjectCard))
  list.classList.add('dashboard-project-grid')
  list.setAttribute('role', 'list')
  list.setAttribute('aria-label', 'Project summaries')
  card.append(list)
  return card
}

function renderProjectCard(project: ProjectLiveSummaryViewModel): HTMLElement {
  const item = document.createElement('article')
  item.className = 'dashboard-project-card'
  item.setAttribute('role', 'listitem')
  item.setAttribute('aria-label', projectAriaLabel(project))
  item.append(
    heading(project.name),
    metric(`${formatCount(project.memoryCount)} memories`),
    metric(`${formatCount(project.sessionCount)} sessions`),
    metric(project.lastActivityLabel),
    healthRow(project),
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

function statusText(value: string): HTMLElement {
  const node = text(value)
  node.setAttribute('role', 'status')
  return node
}

function alertText(value: string): HTMLElement {
  const node = text(value, 'dashboard-state state')
  node.setAttribute('role', 'alert')
  return node
}

function healthRow(project: ProjectLiveSummaryViewModel): HTMLElement {
  const row = document.createElement('p')
  row.className = 'dashboard-project-card__health'
  row.append('Health: ', statusBadge(project.syncHealth))
  return row
}

function browseLink(project: ProjectLiveSummaryViewModel): HTMLAnchorElement {
  const link = document.createElement('a')
  link.className = 'dashboard-control control dashboard-project-card__action'
  link.href = project.browsePath
  link.setAttribute('aria-label', `Open ${project.name} in Knowledge Browser`)
  link.textContent = 'Open in Knowledge Browser'
  return link
}

function projectAriaLabel(project: ProjectLiveSummaryViewModel): string {
  return `${project.name} project: ${statusLabel(project.syncHealth)} health, ${formatCount(project.memoryCount)} memories, ${formatCount(project.sessionCount)} sessions, ${ariaLastActivityLabel(project.lastActivityLabel)}`
}

function ariaLastActivityLabel(value: string): string {
  if (value === 'Last activity unavailable') return 'last activity unavailable'
  return value.replace('Last activity:', 'last activity')
}

function formatCount(value: number): string {
  return new Intl.NumberFormat('en-US').format(value)
}
