import { describe, expect, it, vi } from 'vitest'
import { activityFeedFixture } from '../fixtures/hive-dashboard/explore'
import type { ActivityFeedFixtureViewModel } from '../domain/dashboard'
import type { ViewState } from './Overview'
import { renderActivityFeed, type ActivityFeedDeps, type ActivityFeedHandle } from './ActivityFeed'

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function makeDeps(overrides: Partial<ActivityFeedDeps> = {}): ActivityFeedDeps {
  return { onNavigate: vi.fn(), ...overrides }
}

function fakeScheduler() {
  let cb: (() => void) | null = null
  const setIntervalSpy = vi.fn((fn: () => void, _ms: number) => {
    cb = fn
    return 42 as unknown as ReturnType<typeof setInterval>
  })
  const clearIntervalSpy = vi.fn()
  return {
    scheduler: {
      setInterval: setIntervalSpy as unknown as typeof setInterval,
      clearInterval: clearIntervalSpy as unknown as typeof clearInterval
    },
    tick: () => { cb?.() },
    setIntervalSpy,
    clearIntervalSpy
  }
}

function ready(data: ActivityFeedFixtureViewModel = activityFeedFixture): ViewState<ActivityFeedFixtureViewModel> {
  return { status: 'ready', data }
}

function twoGroups(): ActivityFeedFixtureViewModel {
  return {
    ...activityFeedFixture,
    groups: [
      { dateLabel: 'Today', entries: [entry('e1', 'architecture'), entry('e2', 'bugfix')] },
      { dateLabel: 'Yesterday', entries: [entry('e3', 'decision')] }
    ]
  }
}

function entry(
  id: string,
  category: 'architecture' | 'bugfix' | 'decision' | 'discovery' | 'pattern' | 'config' | 'preference' | 'session_summary' = 'discovery',
  overrides: Partial<{ title: string; actorHandle: string; projectId: string; timeLabel: string }> = {}
) {
  return {
    id,
    title: overrides.title ?? `Title ${id}`,
    actorHandle: overrides.actorHandle ?? '@actor',
    projectId: overrides.projectId ?? 'proj-1',
    category,
    timeLabel: overrides.timeLabel ?? '1m ago'
  }
}

// ---------------------------------------------------------------------------
// T1 — Types and function signature
// ---------------------------------------------------------------------------

describe('ActivityFeed — T1: types and function signature', () => {
  it('renderActivityFeed returns { element: HTMLElement, dispose: function }', () => {
    const result: ActivityFeedHandle = renderActivityFeed({ status: 'loading' }, makeDeps())
    expect(result.element).toBeInstanceOf(HTMLElement)
    expect(typeof result.dispose).toBe('function')
  })
})

// ---------------------------------------------------------------------------
// T2 — Loading and error states
// ---------------------------------------------------------------------------

describe('ActivityFeed — T2: loading and error states', () => {
  it('shows loading text when status is loading', () => {
    const { element } = renderActivityFeed({ status: 'loading' }, makeDeps())
    expect(element.textContent).toContain('Loading activity feed')
  })

  it('shows error message when status is error', () => {
    const { element } = renderActivityFeed({ status: 'error', message: 'feed unavailable' }, makeDeps())
    expect(element.textContent).toContain('feed unavailable')
  })
})

// ---------------------------------------------------------------------------
// T3 — Groups and date headers
// ---------------------------------------------------------------------------

describe('ActivityFeed — T3: groups and date headers', () => {
  it('renders both group date headers in DOM order', () => {
    const { element } = renderActivityFeed(ready(twoGroups()), makeDeps())
    const headers = Array.from(element.querySelectorAll<HTMLElement>('[data-activity-group-header]'))
    expect(headers).toHaveLength(2)
    expect(headers[0].textContent).toContain('Today')
    expect(headers[1].textContent).toContain('Yesterday')
  })

  it('renders entry containers with data-activity-group-entries', () => {
    const { element } = renderActivityFeed(ready(twoGroups()), makeDeps())
    expect(element.querySelectorAll('[data-activity-group-entries]')).toHaveLength(2)
  })
})

// ---------------------------------------------------------------------------
// T4 — Entry rows (button, 5 fields, aria-label)
// ---------------------------------------------------------------------------

describe('ActivityFeed — T4: entry rows', () => {
  it('renders a button.dashboard-notification-card with all 5 fields and aria-label', () => {
    const state = ready({
      ...activityFeedFixture,
      groups: [{
        dateLabel: 'Today',
        entries: [entry('mem-1', 'discovery', { title: 'A title', actorHandle: '@alice', projectId: 'core-api', timeLabel: '2m ago' })]
      }]
    })
    const { element } = renderActivityFeed(state, makeDeps())
    const btn = element.querySelector<HTMLButtonElement>('button.dashboard-notification-card')
    expect(btn).not.toBeNull()
    expect(btn?.getAttribute('type')).toBe('button')
    expect(btn?.textContent).toContain('A title')
    expect(btn?.textContent).toContain('@alice')
    expect(btn?.textContent).toContain('core-api')
    expect(btn?.textContent).toContain('2m ago')
    const label = btn?.getAttribute('aria-label') ?? ''
    expect(label).toContain('A title')
    expect(label).toContain('@alice')
    expect(label).toContain('core-api')
    expect(label).toContain('2m ago')
    expect(label).toContain('discovery')
  })
})

// ---------------------------------------------------------------------------
// T5 — Category badges
// ---------------------------------------------------------------------------

describe('ActivityFeed — T5: category badges', () => {
  const categories = ['architecture', 'bugfix', 'decision', 'discovery', 'pattern', 'config', 'preference', 'session_summary'] as const

  for (const cat of categories) {
    it(`renders badge for category "${cat}"`, () => {
      const state = ready({ ...activityFeedFixture, groups: [{ dateLabel: 'Today', entries: [entry('x1', cat)] }] })
      const { element } = renderActivityFeed(state, makeDeps())
      const badge = element.querySelector<HTMLElement>(`[data-activity-category="${cat}"]`)
      expect(badge).not.toBeNull()
      expect(badge?.className).toContain('dashboard-status')
      expect(badge?.textContent).toBe(cat.replaceAll('_', ' '))
      expect(badge?.hasAttribute('data-dashboard-status')).toBe(false)
    })
  }
})

// ---------------------------------------------------------------------------
// T6 — Click navigation
// ---------------------------------------------------------------------------

describe('ActivityFeed — T6: click navigation', () => {
  it('calls onNavigate with the correct memory path when an entry is clicked', () => {
    const onNavigate = vi.fn()
    const state = ready({ ...activityFeedFixture, groups: [{ dateLabel: 'Today', entries: [entry('mem-42', 'discovery')] }] })
    const { element } = renderActivityFeed(state, { onNavigate })
    const btn = element.querySelector<HTMLButtonElement>('button.dashboard-notification-card')
    btn?.click()
    expect(onNavigate).toHaveBeenCalledWith('/dashboard/memories?id=mem-42')
  })
})

// ---------------------------------------------------------------------------
// T7 — Live indicator
// ---------------------------------------------------------------------------

describe('ActivityFeed — T7: live indicator', () => {
  it('shows data-live-indicator="on" with text "Live" when scheduler is provided', () => {
    const { scheduler } = fakeScheduler()
    const { element } = renderActivityFeed(ready(), { onNavigate: vi.fn(), scheduler })
    const indicator = element.querySelector<HTMLElement>('[data-live-indicator]')
    expect(indicator).not.toBeNull()
    expect(indicator?.getAttribute('data-live-indicator')).toBe('on')
    expect(indicator?.textContent).toContain('Live')
  })

  it('shows data-live-indicator="off" with text "Paused" when no scheduler provided and polling disabled', () => {
    const { element } = renderActivityFeed(
      ready({ ...activityFeedFixture, livePolling: { enabled: false, intervalSeconds: 5 } }),
      { onNavigate: vi.fn() }
    )
    const indicator = element.querySelector<HTMLElement>('[data-live-indicator]')
    expect(indicator).not.toBeNull()
    expect(indicator?.getAttribute('data-live-indicator')).toBe('off')
    expect(indicator?.textContent).toContain('Paused')
  })
})

// ---------------------------------------------------------------------------
// T8 — Polling: setInterval + prepend after 1 tick
// ---------------------------------------------------------------------------

describe('ActivityFeed — T8: polling — setInterval + prepend', () => {
  it('calls setInterval when scheduler is provided', () => {
    const { scheduler, setIntervalSpy } = fakeScheduler()
    renderActivityFeed(ready(), { onNavigate: vi.fn(), scheduler })
    expect(setIntervalSpy).toHaveBeenCalledTimes(1)
  })

  it('prepends a new entry to the first group on tick and entry text includes live id', () => {
    const { scheduler, tick } = fakeScheduler()
    const { element } = renderActivityFeed(ready(twoGroups()), { onNavigate: vi.fn(), scheduler })
    const firstList = element.querySelector('[data-activity-group-entries]')
    const initialCount = firstList?.querySelectorAll('button.dashboard-notification-card').length ?? 0
    tick()
    const afterCount = firstList?.querySelectorAll('button.dashboard-notification-card').length ?? 0
    expect(afterCount).toBe(initialCount + 1)
    const firstBtn = firstList?.querySelector('button.dashboard-notification-card')
    expect(firstBtn?.textContent).toContain('New memory saved (#1)')
  })
})

// ---------------------------------------------------------------------------
// T9 — Polling: dedup + cleanup
// ---------------------------------------------------------------------------

describe('ActivityFeed — T9: polling — dedup + cleanup', () => {
  it('does not prepend an entry with a duplicate id', () => {
    const { scheduler, tick } = fakeScheduler()
    // 'live-1' is already in the entries — the Set seeds from all entries, so first tick
    // makeSyntheticEntry(1) → id='live-1' which is already seeded, should skip
    const data: ActivityFeedFixtureViewModel = {
      ...activityFeedFixture,
      groups: [{ dateLabel: 'Today', entries: [entry('live-1', 'discovery'), entry('other', 'bugfix')] }]
    }
    const { element } = renderActivityFeed(ready(data), { onNavigate: vi.fn(), scheduler })
    const firstList = element.querySelector('[data-activity-group-entries]')
    const countBefore = firstList?.querySelectorAll('button.dashboard-notification-card').length ?? 0
    tick()
    const countAfter = firstList?.querySelectorAll('button.dashboard-notification-card').length ?? 0
    expect(countAfter).toBe(countBefore)
  })

  it('calls clearInterval when dispose() is called', () => {
    const { scheduler, clearIntervalSpy } = fakeScheduler()
    const { dispose } = renderActivityFeed(ready(), { onNavigate: vi.fn(), scheduler })
    dispose()
    expect(clearIntervalSpy).toHaveBeenCalledTimes(1)
  })

  it('does not mutate the DOM after dispose()', () => {
    const { scheduler, tick } = fakeScheduler()
    const { element, dispose } = renderActivityFeed(ready(twoGroups()), { onNavigate: vi.fn(), scheduler })
    const firstList = element.querySelector('[data-activity-group-entries]')
    const countBefore = firstList?.querySelectorAll('button.dashboard-notification-card').length ?? 0
    dispose()
    tick()
    const countAfter = firstList?.querySelectorAll('button.dashboard-notification-card').length ?? 0
    expect(countAfter).toBe(countBefore)
  })
})

// ---------------------------------------------------------------------------
// T10 — Empty groups
// ---------------------------------------------------------------------------

describe('ActivityFeed — T10: empty groups', () => {
  it('renders empty state when groups array is empty', () => {
    const data: ActivityFeedFixtureViewModel = { ...activityFeedFixture, groups: [] }
    const { element } = renderActivityFeed(ready(data), makeDeps())
    expect(element.querySelector('[role="status"]')).not.toBeNull()
    expect(element.textContent).toContain('No activity')
  })

  it('does not register a polling interval when groups is empty', () => {
    const { setIntervalSpy } = fakeScheduler()
    renderActivityFeed(
      { status: 'ready', data: { ...activityFeedFixture, groups: [] } },
      { onNavigate: vi.fn(), scheduler: { setInterval: setIntervalSpy as unknown as typeof setInterval, clearInterval: vi.fn() as unknown as typeof clearInterval } }
    )
    expect(setIntervalSpy).not.toHaveBeenCalled()
  })
})

// ---------------------------------------------------------------------------
// T11 — Source note
// ---------------------------------------------------------------------------

describe('ActivityFeed — T11: source note', () => {
  it('renders the source note with correct class, role, and exact text in ready state', () => {
    const { element } = renderActivityFeed(ready(), makeDeps())
    const note = element.querySelector('.dashboard-source-note')
    expect(note).not.toBeNull()
    expect(note?.getAttribute('role')).toBe('note')
    expect(note?.textContent).toBe('Demo fixture data — live activity feed is unavailable.')
  })
})

// ---------------------------------------------------------------------------
// T12 — Click isolation (S5)
// ---------------------------------------------------------------------------

describe('ActivityFeed — T12: click isolation', () => {
  it('calls onNavigate exactly once for the clicked entry, not for the other', () => {
    const onNavigate = vi.fn()
    const state = ready({
      ...activityFeedFixture,
      groups: [{
        dateLabel: 'Today',
        entries: [
          entry('mem-001', 'discovery'),
          entry('mem-002', 'bugfix')
        ]
      }]
    })
    const { element } = renderActivityFeed(state, { onNavigate })
    const buttons = element.querySelectorAll<HTMLButtonElement>('button.dashboard-notification-card')
    // Click the first button (mem-001)
    buttons[0].click()
    expect(onNavigate).toHaveBeenCalledTimes(1)
    expect(onNavigate).toHaveBeenCalledWith('/dashboard/memories?id=mem-001')
    expect(onNavigate).not.toHaveBeenCalledWith('/dashboard/memories?id=mem-002')
  })
})

// ---------------------------------------------------------------------------
// T13 — N-tick (3-tick) uniqueness + count (S11)
// ---------------------------------------------------------------------------

describe('ActivityFeed — T13: 3-tick uniqueness and count', () => {
  it('prepends 3 unique entries at the top after 3 ticks', () => {
    const { scheduler, tick } = fakeScheduler()
    const data: ActivityFeedFixtureViewModel = {
      ...activityFeedFixture,
      groups: [
        { dateLabel: 'Today', entries: [entry('orig-1', 'discovery'), entry('orig-2', 'bugfix')] }
      ]
    }
    const { element } = renderActivityFeed(ready(data), { onNavigate: vi.fn(), scheduler })
    const firstList = element.querySelector('[data-activity-group-entries]')!

    tick() // tick 1
    tick() // tick 2
    tick() // tick 3

    const buttons = firstList.querySelectorAll<HTMLButtonElement>('button.dashboard-notification-card')
    expect(buttons).toHaveLength(5)

    // All 5 ids are unique — verify via aria-labels (each encodes the entry id via title/category/handle)
    const ariaLabels = Array.from(buttons).map((btn) => btn.getAttribute('aria-label') ?? '')
    const uniqueLabels = new Set(ariaLabels)
    expect(uniqueLabels.size).toBe(5)

    // The 3 new entries are at indices 0, 1, 2 (prepended = newest first)
    expect(buttons[0].textContent).toContain('New memory saved (#3)')
    expect(buttons[1].textContent).toContain('New memory saved (#2)')
    expect(buttons[2].textContent).toContain('New memory saved (#1)')
  })
})

// ---------------------------------------------------------------------------
// T14 — Second group immutability during polling (S12)
// ---------------------------------------------------------------------------

describe('ActivityFeed — T14: second group immutability during polling', () => {
  it('does not mutate the second group when polling adds an entry to the first group', () => {
    const { scheduler, tick } = fakeScheduler()
    const data: ActivityFeedFixtureViewModel = {
      ...activityFeedFixture,
      groups: [
        { dateLabel: 'Today', entries: [entry('a1', 'discovery')] },
        { dateLabel: 'Yesterday', entries: [entry('b1', 'bugfix'), entry('b2', 'decision')] }
      ]
    }
    const { element } = renderActivityFeed(ready(data), { onNavigate: vi.fn(), scheduler })
    const allLists = element.querySelectorAll('[data-activity-group-entries]')
    const firstList = allLists[0]
    const secondList = allLists[1]

    tick()

    const firstCount = firstList.querySelectorAll('button.dashboard-notification-card').length
    const secondCount = secondList.querySelectorAll('button.dashboard-notification-card').length

    expect(firstCount).toBe(2)
    expect(secondCount).toBe(2)
  })
})
