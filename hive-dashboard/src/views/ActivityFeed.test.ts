import { describe, expect, it, vi } from 'vitest'
import type { ActivityFeedViewModel, MemoryCategory } from '../domain/dashboard'
import type { ViewState } from './Overview'
import { renderActivityFeed } from './ActivityFeed'

function ready(data: ActivityFeedViewModel = feed()): ViewState<ActivityFeedViewModel> {
  return { status: 'ready', data }
}

describe('ActivityFeed', () => {
  it('renders a bespoke Activity Feed root with heading and no generic panel chrome', () => {
    const view = renderActivityFeed(ready(), { onNavigate: vi.fn() }).element

    expect(view.matches('section[data-dashboard-view="activity-feed"]')).toBe(true)
    expect(view.getAttribute('aria-labelledby')).toBe('dashboard-activity-feed-title')
    expect(view.querySelector('#dashboard-activity-feed-title')?.textContent).toBe('Activity Feed')
    expect(view.getAttribute('data-dashboard-primitive')).toBeNull()
    expect(view.classList.contains('dashboard-panel')).toBe(false)
    expect(view.classList.contains('panel')).toBe(false)
  })

  it('renders accessible loading, initial error, and empty states without fixture source notes', () => {
    const loading = renderActivityFeed({ status: 'loading' }, { onNavigate: vi.fn() }).element
    expect(loading.querySelector('[role="status"]')?.textContent).toContain('Loading recent memory lifecycle activity')

    const error = renderActivityFeed({ status: 'error', message: 'activity unavailable' }, { onNavigate: vi.fn() }).element
    expect(error.querySelector('[role="alert"]')?.textContent).toContain('activity unavailable')

    const empty = renderActivityFeed(ready(feed({ groups: [] })), { onNavigate: vi.fn() }).element
    expect(empty.querySelector('[role="status"]')?.textContent).toContain('No recent memory lifecycle activity is available.')
    expect(empty.textContent).toContain('Recent memory lifecycle activity from the live Activity API.')
    expect(empty.textContent).not.toContain('Demo fixture data')
  })

  it('renders grouped backend entries as timeline cards with honest metadata and no unsupported controls', () => {
    const { element } = renderActivityFeed(ready(feed({
      groups: [
        { dateLabel: 'Today', entries: [entry('event-1', { title: 'Captured decision', eventLabel: 'Created', summary: 'Decision summary', category: 'decision', memorySyncId: 'sync-1' })] },
        { dateLabel: 'Yesterday', entries: [entry('event-2', { title: 'Deleted stale memory', eventLabel: 'Deleted', summary: 'Delete summary', category: 'bugfix' })] }
      ]
    })), { onNavigate: vi.fn() })

    expect(Array.from(element.querySelectorAll('[data-activity-group-header]')).map((header) => header.textContent)).toEqual(['Today', 'Yesterday'])
    expect(element.querySelectorAll('button.dashboard-notification-card')).toHaveLength(0)
    expect(element.querySelectorAll('[data-activity-entry-static]')).toHaveLength(2)
    expect(element.querySelectorAll('article[role="listitem"]')).toHaveLength(2)
    expect(element.querySelector('[data-activity-category="decision"]')?.textContent).toBe('decision')
    expect(element.textContent).toContain('Created')
    expect(element.textContent).toContain('Decision summary')
    expect(element.textContent).toContain('@ada')
    expect(element.textContent).toContain('jarvis-dev')
    expect(element.textContent).toContain('09:00')
    expect(element.querySelector('[role="note"]')?.textContent).toContain('Recent memory lifecycle activity')
    expect(element.querySelector('form')).toBeNull()
    expect(element.querySelector('a[href^="/dashboard/memories/"]')).toBeNull()
    expect(element.textContent).not.toContain('Audit log')
    expect(element.textContent).not.toContain('Unread')
    expect(element.querySelector('[data-live-indicator]')).toBeNull()
  })

  it('keeps memory sync ID entries visible but non-navigable until activity exposes server memory IDs', () => {
    const onNavigate = vi.fn()
    const { element } = renderActivityFeed(ready(feed({
      groups: [{
        dateLabel: 'Today',
        entries: [entry('event-1', { memorySyncId: 'sync-1' }), entry('event-2', { title: 'Deleted memory' })]
      }]
    })), { onNavigate })

    expect(element.querySelector('button.dashboard-notification-card')).toBeNull()
    expect(element.textContent).toContain('Activity event-1')
    expect(onNavigate).not.toHaveBeenCalled()
    expect(Array.from(element.querySelectorAll('[data-activity-entry-static]')).map((row) => row.textContent)).toEqual([
      expect.stringContaining('Activity event-1'),
      expect.stringContaining('Deleted memory')
    ])
  })

  it('renders Load More from nextCursor and exposes loading and pagination error states', () => {
    const onLoadMore = vi.fn()
    const { element } = renderActivityFeed(ready(feed({ nextCursor: 'cursor-2' })), { onNavigate: vi.fn(), onLoadMore })
    const button = element.querySelector<HTMLButtonElement>('button[data-load-more-activity]')

    expect(button?.textContent).toBe('Load More')
    button?.click()
    expect(onLoadMore).toHaveBeenCalledTimes(1)

    const loading = renderActivityFeed(ready(feed({ nextCursor: 'cursor-2', loadingMore: true })), { onNavigate: vi.fn(), onLoadMore }).element
    expect(loading.querySelector<HTMLButtonElement>('button[data-load-more-activity]')?.disabled).toBe(true)
    expect(loading.querySelector<HTMLButtonElement>('button[data-load-more-activity]')?.textContent).toBe('Loading more…')

    const failed = renderActivityFeed(ready(feed({ nextCursor: 'cursor-2', paginationError: 'next page failed' })), { onNavigate: vi.fn(), onLoadMore }).element
    expect(failed.querySelector('[role="alert"]')?.textContent).toContain('next page failed')
  })
})

function feed(overrides: Partial<ActivityFeedViewModel> = {}): ActivityFeedViewModel {
  return {
    screen: 'activityFeed',
    groups: [{ dateLabel: 'Today', entries: [entry('event-1', { memorySyncId: 'sync-1' })] }],
    ...overrides
  }
}

function entry(id: string, overrides: Partial<{ title: string; eventLabel: string; summary: string; category: MemoryCategory; memorySyncId: string; timeLabel: string }> = {}) {
  return {
    id,
    title: overrides.title ?? `Activity ${id}`,
    eventType: 'create',
    eventLabel: overrides.eventLabel ?? 'Created',
    summary: overrides.summary ?? `Summary for ${id}`,
    actorHandle: '@ada',
    projectId: 'jarvis-dev',
    category: overrides.category ?? 'discovery',
    sourceLabel: overrides.category ?? 'discovery',
    timeLabel: overrides.timeLabel ?? '09:00',
    absoluteTimeLabel: '25 Jun 2026 · 09:00',
    relativeTimeLabel: '3h ago',
    memorySyncId: overrides.memorySyncId
  }
}
