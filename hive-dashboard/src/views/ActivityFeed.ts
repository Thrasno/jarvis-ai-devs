import { append, control, emptyState, error, panel, text } from '../components/dom'
import type { ActivityEntryViewModel, ActivityFeedViewModel, MemoryCategory } from '../domain/dashboard'
import type { ViewState } from './Overview'

export type ActivityFeedDeps = {
  onNavigate: (path: string) => void
  onLoadMore?: () => void
}

export type ActivityFeedHandle = { element: HTMLElement; dispose: () => void }

function categoryBadge(category: MemoryCategory): HTMLElement {
  const badge = document.createElement('span')
  badge.className = 'dashboard-status'
  badge.setAttribute('data-activity-category', category)
  badge.textContent = category.replaceAll('_', ' ')
  return badge
}

function renderEntryContent(root: HTMLElement, entry: ActivityEntryViewModel): void {
  root.append(
    text(entry.title),
    categoryBadge(entry.category),
    text(entry.actorHandle),
    text(entry.projectId),
    text(entry.timeLabel)
  )
}

function entryLabel(entry: ActivityEntryViewModel): string {
  return `${entry.title} — ${entry.category} — ${entry.actorHandle} — ${entry.projectId} — ${entry.timeLabel}`
}

function renderEntry(entry: ActivityEntryViewModel): HTMLElement {
  const row = document.createElement('article')
  row.className = 'dashboard-notification-card'
  row.setAttribute('data-activity-entry-static', '')
  row.setAttribute('aria-label', entryLabel(entry))
  renderEntryContent(row, entry)
  return row
}

export function renderActivityFeed(
  state: ViewState<ActivityFeedViewModel>,
  deps: ActivityFeedDeps
): ActivityFeedHandle {
  const card = panel('Activity Feed')

  if (state.status === 'loading') {
    append(card, text('Loading activity feed…'))
    return { element: card, dispose: () => {} }
  }

  if (state.status === 'error') {
    error(card, state.message)
    card.querySelector('.dashboard-state')?.setAttribute('role', 'alert')
    return { element: card, dispose: () => {} }
  }

  const { groups } = state.data
  if (groups.length === 0) {
    append(card, emptyState('No activity entries found.'))
    return { element: card, dispose: () => {} }
  }

  for (const group of groups) {
    const section = document.createElement('section')
    section.className = 'dashboard-activity-group'

    const header = document.createElement('h3')
    header.setAttribute('data-activity-group-header', '')
    header.textContent = group.dateLabel
    section.append(header)

    const entryList = document.createElement('div')
    entryList.setAttribute('data-activity-group-entries', '')
    for (const entry of group.entries) {
      entryList.append(renderEntry(entry))
    }
    section.append(entryList)
    card.append(section)
  }

  if (state.data.paginationError) {
    const paginationError = text(state.data.paginationError, 'dashboard-state state')
    paginationError.setAttribute('role', 'alert')
    card.append(paginationError)
  }

  if (state.data.nextCursor) {
    const button = control(state.data.loadingMore ? 'Loading more…' : 'Load More', { disabled: state.data.loadingMore })
    button.setAttribute('data-load-more-activity', '')
    button.addEventListener('click', () => deps.onLoadMore?.())
    card.append(button)
  }

  return { element: card, dispose: () => {} }
}
