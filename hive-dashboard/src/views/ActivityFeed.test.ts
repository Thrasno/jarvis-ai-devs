import { describe, expect, it, vi } from 'vitest'
import type { ActivityFeedViewModel, MemoryCategory } from '../domain/dashboard'
import type { ViewState } from './Overview'
import { renderActivityFeed } from './ActivityFeed'

function ready(data: ActivityFeedViewModel = feed()): ViewState<ActivityFeedViewModel> {
  return { status: 'ready', data }
}

describe('ActivityFeed', () => {
  it('renders loading, initial error, and empty states without fixture source notes', () => {
    expect(renderActivityFeed({ status: 'loading' }, { onNavigate: vi.fn() }).element.textContent).toContain('Loading activity feed')

    const error = renderActivityFeed({ status: 'error', message: 'activity unavailable' }, { onNavigate: vi.fn() }).element
    expect(error.querySelector('[role="alert"]')?.textContent).toContain('activity unavailable')

    const empty = renderActivityFeed(ready(feed({ groups: [] })), { onNavigate: vi.fn() }).element
    expect(empty.querySelector('[role="status"]')?.textContent).toContain('No activity entries found')
    expect(empty.textContent).not.toContain('Demo fixture data')
  })

  it('renders grouped backend entries with category badges and no live polling indicator', () => {
    const { element } = renderActivityFeed(ready(feed({
      groups: [
        { dateLabel: 'Today', entries: [entry('event-1', { title: 'Captured decision', category: 'decision', memorySyncId: 'sync-1' })] },
        { dateLabel: 'Yesterday', entries: [entry('event-2', { title: 'Deleted stale memory', category: 'bugfix' })] }
      ]
    })), { onNavigate: vi.fn() })

    expect(Array.from(element.querySelectorAll('[data-activity-group-header]')).map((header) => header.textContent)).toEqual(['Today', 'Yesterday'])
    expect(element.querySelectorAll('button.dashboard-notification-card')).toHaveLength(1)
    expect(element.querySelectorAll('[data-activity-entry-static]')).toHaveLength(1)
    expect(element.querySelector('[data-activity-category="decision"]')?.textContent).toBe('decision')
    expect(element.textContent).not.toContain('Live')
    expect(element.querySelector('[data-live-indicator]')).toBeNull()
  })

  it('navigates only entries with memorySyncId and leaves delete-only rows static', () => {
    const onNavigate = vi.fn()
    const { element } = renderActivityFeed(ready(feed({
      groups: [{
        dateLabel: 'Today',
        entries: [entry('event-1', { memorySyncId: 'sync-1' }), entry('event-2', { title: 'Deleted memory' })]
      }]
    })), { onNavigate })

    element.querySelector<HTMLButtonElement>('button.dashboard-notification-card')!.click()
    expect(onNavigate).toHaveBeenCalledWith('/dashboard/memories/sync-1')
    expect(element.querySelector('[data-activity-entry-static]')?.textContent).toContain('Deleted memory')
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

function entry(id: string, overrides: Partial<{ title: string; category: MemoryCategory; memorySyncId: string; timeLabel: string }> = {}) {
  return {
    id,
    title: overrides.title ?? `Activity ${id}`,
    actorHandle: '@ada',
    projectId: 'jarvis-dev',
    category: overrides.category ?? 'discovery',
    timeLabel: overrides.timeLabel ?? '09:00',
    memorySyncId: overrides.memorySyncId
  }
}
