import { append, emptyState, text } from '../components/dom'
import type { ProjectListViewModel, ProjectLiveSummaryViewModel } from '../domain/dashboard'
import type { ViewState } from './Overview'

let projectCardId = 0

export function renderProjects(state: ViewState<ProjectListViewModel>): HTMLElement {
  const root = projectsRoot(state.status === 'ready' ? state.data.totalProjects : 0)
  if (state.status === 'loading') return append(root, statusText('Loading live project summaries…'))
  if (state.status === 'error') return append(root, alertText(state.message))

  if (state.data.projects.length === 0) {
    root.append(emptyState('No live project summaries found.'))
    return root
  }

  const list = document.createElement('div')
  list.className = 'dashboard-projects__grid'
  list.setAttribute('role', 'list')
  list.setAttribute('aria-label', 'Accessible repositories')
  list.append(...state.data.projects.map(renderProjectCard))
  root.append(list)
  return root
}

function projectsRoot(totalProjects: number): HTMLElement {
  const root = document.createElement('section')
  root.className = 'dashboard-projects'
  root.dataset.dashboardView = 'projects'
  root.setAttribute('aria-labelledby', 'dashboard-projects-title')

  const header = document.createElement('div')
  header.className = 'dashboard-projects__header'

  const eyebrow = document.createElement('p')
  eyebrow.className = 'dashboard-projects__eyebrow'
  eyebrow.textContent = `ACCESSIBLE REPOSITORIES · ${formatCount(totalProjects)}`

  const title = document.createElement('h2')
  title.id = 'dashboard-projects-title'
  title.className = 'dashboard-projects__title'
  title.textContent = 'Projects'

  header.append(eyebrow, title)
  root.append(header)
  return root
}

function renderProjectCard(project: ProjectLiveSummaryViewModel): HTMLElement {
  const item = document.createElement('article')
  const titleId = `dashboard-project-card-${++projectCardId}`
  const metricsId = `${titleId}-metrics`
  item.className = 'dashboard-project-card'
  item.setAttribute('role', 'listitem')
  item.setAttribute('aria-labelledby', titleId)
  item.setAttribute('aria-describedby', metricsId)
  item.setAttribute('aria-label', projectAriaLabel(project))
  item.append(
    identitySection(project, titleId),
    metricsSection(project, metricsId),
    decorativeRail(),
    browseLink(project)
  )
  return item
}

function identitySection(project: ProjectLiveSummaryViewModel, titleId: string): HTMLElement {
  const section = document.createElement('div')
  section.className = 'dashboard-project-card__identity'
  const title = document.createElement('h3')
  title.id = titleId
  title.textContent = project.name
  section.append(title, healthSection(project))
  return section
}

function healthSection(project: ProjectLiveSummaryViewModel): HTMLElement {
  const health = document.createElement('div')
  health.className = 'dashboard-project-card__health'
  health.dataset.projectHealth = project.syncHealth
  health.textContent = healthLabel(project.syncHealth)
  return health
}

function metricsSection(project: ProjectLiveSummaryViewModel, metricsId: string): HTMLElement {
  const metrics = document.createElement('dl')
  metrics.id = metricsId
  metrics.className = 'dashboard-project-card__metrics'
  metrics.append(
    metric('MEMORIES', formatCount(project.memoryCount)),
    metric('SESSIONS', formatCount(project.sessionCount)),
    metric('LAST ACTIVITY', lastActivityValue(project.lastActivityLabel))
  )
  return metrics
}

function metric(label: string, value: string): HTMLElement {
  const group = document.createElement('div')
  group.className = 'dashboard-project-card__metric'
  const term = document.createElement('dt')
  term.textContent = label
  const description = document.createElement('dd')
  description.textContent = value
  group.append(term, description)
  return group
}

function decorativeRail(): HTMLElement {
  const rail = document.createElement('div')
  rail.className = 'dashboard-project-card__rail'
  rail.setAttribute('aria-hidden', 'true')
  return rail
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

function browseLink(project: ProjectLiveSummaryViewModel): HTMLElement {
  const footer = document.createElement('div')
  footer.className = 'dashboard-project-card__actions'
  const link = document.createElement('a')
  link.className = 'dashboard-project-card__browse'
  link.href = project.browsePath
  link.setAttribute('aria-label', `Browse memories for ${project.name}`)
  link.textContent = 'browse memories →'
  footer.append(link)
  return footer
}

function projectAriaLabel(project: ProjectLiveSummaryViewModel): string {
  return `${project.name} project: ${healthLabel(project.syncHealth)} health, ${formatCount(project.memoryCount)} memories, ${formatCount(project.sessionCount)} sessions, ${ariaLastActivityLabel(project.lastActivityLabel)}`
}

function ariaLastActivityLabel(value: string): string {
  if (value === 'Last activity unavailable') return 'last activity unavailable'
  return value.replace('Last activity:', 'last activity')
}

function lastActivityValue(value: string): string {
  if (value === 'Last activity unavailable') return 'Unavailable'
  return value.replace('Last activity:', '').trim()
}

function healthLabel(value: ProjectLiveSummaryViewModel['syncHealth']): string {
  return value.toUpperCase()
}

function formatCount(value: number): string {
  return new Intl.NumberFormat('en-US').format(value)
}
