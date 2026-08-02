import { append, emptyState, text } from '../components/dom'
import type { ProjectListViewModel, ProjectLiveSummaryViewModel } from '../domain/dashboard'
import { projectBlockActions, projectHealthFilters } from '../api/client'
import type { ViewState } from './Overview'

let projectCardId = 0
type ProjectViewOptions = {
  health?: typeof projectHealthFilters.degraded
  currentUserLevel?: 'admin' | 'member' | 'viewer' | string
  onBlockProject?: (request: { project: string; action: typeof projectBlockActions.BLOCK; reason: string; confirmation: string }) => Promise<void> | void
  pendingBlockProject?: string
  mutationError?: string
  refreshError?: string
}

export function renderProjects(state: ViewState<ProjectListViewModel>, options: ProjectViewOptions = {}): HTMLElement {
  const root = projectsRoot(state.status === 'ready' ? state.data.totalProjects : 0, options.health)
  if (state.status === 'loading') return append(root, statusText('Loading live project summaries…'))
  if (state.status === 'error') return append(root, alertText(state.message))

  if (state.data.projects.length === 0) {
    root.append(emptyState(options.health === projectHealthFilters.degraded ? 'No degraded projects found.' : 'No live project summaries found.'))
    return root
  }

  const list = document.createElement('div')
  list.className = 'dashboard-projects__grid'
  list.setAttribute('role', 'list')
  list.setAttribute('aria-label', 'Accessible repositories')
  list.append(...state.data.projects.map((project) => renderProjectCard(project, options)))
  root.append(list)
  return root
}

function projectsRoot(totalProjects: number, health?: typeof projectHealthFilters.degraded): HTMLElement {
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

  const filters = document.createElement('nav')
  filters.setAttribute('aria-label', 'Project health filter')
  for (const [label, href, active] of [['All', '/dashboard/projects', health === undefined], ['Degraded', '/dashboard/projects?health=degraded', health === projectHealthFilters.degraded]] as const) {
    const link = document.createElement('a')
    link.dataset.projectHealthFilter = label.toLowerCase()
    link.href = href
    link.textContent = label
    if (active) link.setAttribute('aria-current', 'page')
    filters.append(link)
  }
  header.append(eyebrow, title, filters)
  root.append(header)
  return root
}

function renderProjectCard(project: ProjectLiveSummaryViewModel, options: ProjectViewOptions): HTMLElement {
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
    governanceSection(project, options),
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
  if (project.blocked) section.append(blockedBadge())
  return section
}

function blockedBadge(): HTMLElement {
  const badge = document.createElement('span')
  badge.className = 'dashboard-project-card__blocked-badge'
  badge.textContent = 'BLOCKED'
  return badge
}

function governanceSection(project: ProjectLiveSummaryViewModel, options: ProjectViewOptions): HTMLElement {
  const section = document.createElement('section')
  section.className = 'dashboard-project-card__governance'
  section.setAttribute('aria-label', `${project.name} governance`)
  if (project.blocked) {
    section.append(governanceLine(`Status: ACK ${project.blockAckStatus ?? 'pending'}`))
    if (project.blockReason) section.append(governanceLine(`Reason: ${project.blockReason}`))
  }
  if (options.currentUserLevel === 'admin') {
    section.append(blockForm(project, options.onBlockProject, options.pendingBlockProject === project.name))
    if (options.mutationError) section.append(formError(options.mutationError))
    if (options.refreshError) section.append(formError(options.refreshError))
  } else {
    section.append(governanceLine('Admin access required to block or quarantine projects.'))
  }
  return section
}

function governanceLine(value: string): HTMLElement {
  const line = document.createElement('p')
  line.className = 'dashboard-project-card__governance-line'
  line.textContent = value
  return line
}

function blockForm(project: ProjectLiveSummaryViewModel, onBlockProject?: ProjectViewOptions['onBlockProject'], pending = false): HTMLFormElement {
  const form = document.createElement('form')
  form.className = 'dashboard-project-card__block-form'
  form.setAttribute('aria-label', `Block ${project.name}`)
  form.append(
    formHelp(`Type ${project.canonicalProjectKey} exactly`),
    input('reason', 'Reason', 'Duplicate or garbage project'),
    input('confirmation', 'Exact confirmation', project.canonicalProjectKey),
    ...(pending ? [statusText('Quarantine request in progress…')] : []),
    quarantineButton(pending)
  )
  form.addEventListener('submit', async (event) => {
    event.preventDefault()
    const data = new FormData(form)
    const reason = String(data.get('reason') ?? '').trim()
    const confirmation = String(data.get('confirmation') ?? '')
    clearFormError(form)
	if (reason === '' || confirmation !== project.canonicalProjectKey) {
		form.insertBefore(formError('Reason and exact canonical confirmation are required.'), form.firstChild)
      return
    }
    await onBlockProject?.({
      project: project.name,
		action: projectBlockActions.BLOCK,
		reason,
		confirmation
    })
  })
  return form
}

function formError(value: string): HTMLElement {
  const error = document.createElement('p')
  error.className = 'dashboard-project-card__block-error'
  error.setAttribute('role', 'alert')
  error.textContent = value
  return error
}

function clearFormError(form: HTMLFormElement): void {
  form.querySelector('.dashboard-project-card__block-error')?.remove()
}

function formHelp(value: string): HTMLElement {
  const help = document.createElement('p')
  help.className = 'dashboard-project-card__block-help'
  help.textContent = value
  return help
}

function input(name: string, labelText: string, placeholder: string): HTMLElement {
  const label = document.createElement('label')
  label.textContent = labelText
  const field = document.createElement('input')
  field.name = name
  field.placeholder = placeholder
  field.required = true
  label.append(field)
  return label
}

function quarantineButton(disabled = false): HTMLButtonElement {
  const button = document.createElement('button')
  button.type = 'submit'
  button.textContent = 'Quarantine project'
  button.disabled = disabled
  return button
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
