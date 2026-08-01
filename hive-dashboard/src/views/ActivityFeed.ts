import { append, control, emptyState, error, text } from '../components/dom'
import type { ActivityEntryViewModel, ActivityFeedViewModel, MemoryCategory } from '../domain/dashboard'
import type { ViewState } from './Overview'

export type ActivityFeedDeps = {
  onNavigate: (path: string) => void
  onLoadMore?: () => void
}

export type ActivityFeedHandle = { element: HTMLElement; dispose: () => void }

function categoryBadge(category: MemoryCategory): HTMLElement {
  const badge = document.createElement('span')
  badge.className = 'dashboard-activity-feed__source-badge dashboard-status'
  badge.setAttribute('data-activity-category', category)
  badge.textContent = category.replaceAll('_', ' ')
  return badge
}

function renderEntryContent(root: HTMLElement, entry: ActivityEntryViewModel): void {
  const header = document.createElement('div')
  header.className = 'dashboard-activity-feed__entry-header'
  const eventBadge = document.createElement('span')
  eventBadge.className = 'dashboard-activity-feed__event-badge'
  eventBadge.textContent = entry.eventLabel
  header.append(eventBadge, categoryBadge(entry.category))

  const title = document.createElement('h3')
  title.className = 'dashboard-activity-feed__entry-title'
  title.textContent = entry.title

  const summary = text(entry.summary, 'dashboard-activity-feed__entry-summary')

  const metadata = document.createElement('dl')
  metadata.className = 'dashboard-activity-feed__metadata'
  metadata.setAttribute('aria-label', `Activity metadata for ${entry.title}`)
  metadata.append(
    metadataItem('Actor', entry.actorHandle),
    metadataItem('Project', entry.projectId),
    metadataItem('Time', `${entry.relativeTimeLabel} · ${entry.absoluteTimeLabel}`),
    metadataItem('Source', entry.sourceLabel)
  )

  root.append(header, title, summary, metadata)
}

function entryLabel(entry: ActivityEntryViewModel): string {
  return `${entry.title} — ${entry.category} — ${entry.actorHandle} — ${entry.projectId} — ${entry.timeLabel}`
}

function renderEntry(entry: ActivityEntryViewModel): HTMLElement {
  const row = document.createElement('article')
  row.className = 'dashboard-activity-feed__entry-card'
  row.setAttribute('data-activity-entry-static', '')
  row.setAttribute('role', 'listitem')
  row.setAttribute('aria-label', entryLabel(entry))
  renderEntryContent(row, entry)
  return row
}

function metadataItem(label: string, value: string): HTMLElement {
  const wrapper = document.createElement('div')
  wrapper.className = 'dashboard-activity-feed__metadata-item'
  const term = document.createElement('dt')
  term.textContent = label
  const description = document.createElement('dd')
  description.textContent = value
  wrapper.append(term, description)
  return wrapper
}

export function renderActivityFeed(
  state: ViewState<ActivityFeedViewModel>,
  deps: ActivityFeedDeps
): ActivityFeedHandle {
  const card = activityFeedRoot()

  if (state.status === 'loading') {
    const loading = text('Loading recent memory lifecycle activity…')
    loading.setAttribute('role', 'status')
    append(card, loading)
    return { element: card, dispose: () => {} }
  }

  if (state.status === 'error') {
    error(card, state.message)
    card.querySelector('.dashboard-state')?.setAttribute('role', 'alert')
    return { element: card, dispose: () => {} }
  }

  const { groups } = state.data
  if (groups.length === 0) {
    append(card, emptyState('No recent memory lifecycle activity is available.'))
    return { element: card, dispose: () => {} }
  }

  const timeline = document.createElement('div')
  timeline.className = 'dashboard-activity-feed__timeline'
  timeline.setAttribute('role', 'list')
  timeline.setAttribute('aria-label', 'Recent memory lifecycle activity')

  for (const group of groups) {
    const section = document.createElement('section')
    section.className = 'dashboard-activity-feed__group'

    const header = document.createElement('h3')
    header.setAttribute('data-activity-group-header', '')
    header.textContent = group.dateLabel
    section.append(header)

    const entryList = document.createElement('div')
    entryList.className = 'dashboard-activity-feed__entries'
    entryList.setAttribute('data-activity-group-entries', '')
    for (const entry of group.entries) {
      entryList.append(renderEntry(entry))
    }
    section.append(entryList)
    timeline.append(section)
  }
  card.append(timeline)

  if (state.data.paginationError) {
    const paginationError = text(state.data.paginationError, 'dashboard-state state')
    paginationError.setAttribute('role', 'alert')
    card.append(paginationError)
  }

  if (state.data.nextCursor) {
    const button = control(state.data.loadingMore ? '' : 'Load older activity ↓', { disabled: state.data.loadingMore })
    button.classList.add('dashboard-activity-feed__load-more')
    button.setAttribute('data-load-more-activity', '')
    if (state.data.loadingMore) {
      button.setAttribute('aria-busy', 'true')
      button.setAttribute('aria-label', 'Loading older activity…')
      const progress = document.createElement('span')
      progress.className = 'dashboard-activity-feed__load-more-progress'
      progress.setAttribute('role', 'progressbar')
      progress.setAttribute('aria-label', 'Loading older activity')
      const label = document.createElement('span')
      label.textContent = 'Loading older activity…'
      button.append(progress, label)
    }
    button.addEventListener('click', () => {
      if (button.disabled) return
      button.disabled = true
      button.setAttribute('aria-disabled', 'true')
      deps.onLoadMore?.()
    })
    card.append(button)
  }

  return { element: card, dispose: () => {} }
}

function activityFeedRoot(): HTMLElement {
  const root = document.createElement('section')
  root.className = 'dashboard-activity-feed'
  root.dataset.dashboardView = 'activity-feed'
  root.setAttribute('aria-labelledby', 'dashboard-activity-feed-title')

  const header = document.createElement('div')
  header.className = 'dashboard-activity-feed__header'

  const title = document.createElement('h2')
  title.id = 'dashboard-activity-feed-title'
  title.className = 'dashboard-activity-feed__title'
  title.textContent = 'Activity Feed'

  header.append(title)
  root.append(header, sourceNote())
  return root
}

function sourceNote(): HTMLElement {
  const note = text('Recent memory lifecycle activity from the live Activity API. This is not an audit log or notification inbox.', 'dashboard-activity-feed__source-note')
  note.setAttribute('role', 'note')
  return note
}
