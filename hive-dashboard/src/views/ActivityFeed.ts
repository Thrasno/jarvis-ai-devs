import { append, emptyState, error, panel, text } from '../components/dom'
import type { ActivityEntryViewModel, ActivityFeedFixtureViewModel, MemoryCategory } from '../domain/dashboard'
import type { ViewState } from './Overview'

export type ActivityFeedDeps = {
  onNavigate: (path: string) => void
  scheduler?: { setInterval: typeof setInterval; clearInterval: typeof clearInterval }
}

export type ActivityFeedHandle = { element: HTMLElement; dispose: () => void }

function makeSyntheticEntry(seq: number): ActivityEntryViewModel {
  return {
    id: `live-${seq}`,
    title: `New memory saved (#${seq})`,
    actorHandle: '@live',
    projectId: 'live-feed',
    category: 'discovery',
    timeLabel: 'just now'
  }
}

function categoryBadge(category: MemoryCategory): HTMLElement {
  const badge = document.createElement('span')
  badge.className = 'dashboard-status'
  badge.setAttribute('data-activity-category', category)
  badge.textContent = category.replaceAll('_', ' ')
  return badge
}

function renderEntry(entry: ActivityEntryViewModel, onNavigate: (path: string) => void): HTMLButtonElement {
  const btn = document.createElement('button')
  btn.type = 'button'
  btn.className = 'dashboard-notification-card'
  btn.setAttribute(
    'aria-label',
    `${entry.title} — ${entry.category} — ${entry.actorHandle} — ${entry.projectId} — ${entry.timeLabel}`
  )
  btn.append(
    text(entry.title),
    categoryBadge(entry.category),
    text(entry.actorHandle),
    text(entry.projectId),
    text(entry.timeLabel)
  )
  btn.addEventListener('click', () => onNavigate(`/dashboard/memories?id=${encodeURIComponent(entry.id)}`))
  return btn
}

function renderLiveIndicator(active: boolean): HTMLElement {
  const badge = document.createElement('span')
  badge.setAttribute('data-live-indicator', active ? 'on' : 'off')
  badge.className = 'dashboard-live-indicator'
  badge.textContent = active ? 'Live' : 'Paused'
  return badge
}

function sourceNote(): HTMLElement {
  const note = text('Demo fixture data — live activity feed is unavailable.', 'dashboard-source-note')
  note.setAttribute('role', 'note')
  return note
}

export function renderActivityFeed(
  state: ViewState<ActivityFeedFixtureViewModel>,
  deps: ActivityFeedDeps
): ActivityFeedHandle {
  const card = panel('Activity Feed')

  if (state.status === 'loading') {
    append(card, text('Loading activity feed…'))
    return { element: card, dispose: () => {} }
  }

  if (state.status === 'error') {
    error(card, state.message)
    return { element: card, dispose: () => {} }
  }

  const { groups, livePolling } = state.data
  const sched = deps.scheduler ?? { setInterval, clearInterval }
  const liveActive = livePolling.enabled

  // Live indicator near header
  card.querySelector('h2')?.after(renderLiveIndicator(liveActive))

  if (groups.length === 0) {
    append(card, emptyState('No activity entries found.'))
    // Polling guard: empty groups → no interval registered
    let disposed = false
    return {
      element: card,
      dispose: () => {
        if (disposed) return
        disposed = true
      }
    }
  }

  // Seed dedup set with all existing entry ids
  const seenIds = new Set<string>(groups.flatMap((g) => g.entries.map((e) => e.id)))
  let seq = 0

  // Render groups
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
      entryList.append(renderEntry(entry, deps.onNavigate))
    }
    section.append(entryList)
    card.append(section)
  }

  // Source note
  card.append(sourceNote())

  // Polling
  let disposed = false
  let timerId: ReturnType<typeof setInterval> | undefined

  if (livePolling.enabled) {
    timerId = sched.setInterval(() => {
      if (disposed) return
      const firstList = card.querySelector<HTMLElement>('[data-activity-group-entries]')
      if (!firstList) return
      const synthetic = makeSyntheticEntry(++seq)
      if (seenIds.has(synthetic.id)) return
      seenIds.add(synthetic.id)
      firstList.prepend(renderEntry(synthetic, deps.onNavigate))
    }, livePolling.intervalSeconds * 1000)
  }

  return {
    element: card,
    dispose: () => {
      if (disposed) return
      disposed = true
      if (timerId !== undefined) sched.clearInterval(timerId)
    }
  }
}
