import { describe, expect, it } from 'vitest'
import type { ActivityFeedEntry, ActivityFeedResponse } from '../api/client'
import { activityFeedFromApi, appendActivityPage } from './activityFeed'

const now = new Date('2026-06-25T12:00:00Z')

describe('activity feed mapper', () => {
  it('groups backend entries under Today, Yesterday, and DD Mon labels', () => {
    const viewModel = activityFeedFromApi({
      entries: [
        entry('event-today', '2026-06-25T08:30:00Z', { title: 'Today memory' }),
        entry('event-yesterday', '2026-06-24T19:15:00Z', { title: 'Yesterday memory' }),
        entry('event-older', '2026-06-21T10:45:00Z', { title: 'Older memory' })
      ],
      next_cursor: 'next-page'
    }, now)

    expect(viewModel.groups.map((group) => group.dateLabel)).toEqual(['Today', 'Yesterday', '21 Jun'])
    expect(viewModel.groups[0].entries[0]).toMatchObject({
      id: 'event-today',
      title: 'Today memory',
      actorHandle: 'ada@example.com',
      projectId: 'jarvis-dev',
      category: 'decision',
      memorySyncId: 'sync-event-today'
    })
    expect(viewModel.nextCursor).toBe('next-page')
  })

  it('keeps entries without memory_sync_id visible but non-navigable', () => {
    const viewModel = activityFeedFromApi({ entries: [entry('delete-event', '2026-06-25T08:30:00Z', { memory_sync_id: null })] }, now)

    expect(viewModel.groups).toHaveLength(1)
    expect(viewModel.groups[0].entries[0]).toMatchObject({
      id: 'delete-event',
      title: 'Captured activity delete-event',
      memorySyncId: undefined
    })
  })

  it('appends a cursor page without duplicating existing event ids', () => {
    const firstPage = activityFeedFromApi({
      entries: [entry('event-1', '2026-06-25T08:30:00Z'), entry('event-2', '2026-06-24T08:30:00Z')],
      next_cursor: 'cursor-2'
    }, now)
    const appended = appendActivityPage(firstPage, {
      entries: [entry('event-2', '2026-06-24T08:30:00Z'), entry('event-3', '2026-06-21T08:30:00Z')],
      next_cursor: null
    }, now)

    expect(appended.groups.map((group) => [group.dateLabel, group.entries.map((item) => item.id)])).toEqual([
      ['Today', ['event-1']],
      ['Yesterday', ['event-2']],
      ['21 Jun', ['event-3']]
    ])
    expect(appended.nextCursor).toBeUndefined()
    expect(appended.loadingMore).toBe(false)
    expect(appended.paginationError).toBeUndefined()
  })
})

function entry(id: string, occurredAt: string, overrides: Partial<ActivityFeedEntry> = {}): ActivityFeedEntry {
  return {
    id,
    event_type: 'create',
    occurred_at: occurredAt,
    actor: 'ada@example.com',
    project: 'jarvis-dev',
    category: 'decision',
    title: `Captured activity ${id}`,
    summary: `Summary for ${id}`,
    memory_sync_id: `sync-${id}`,
    ...overrides
  }
}
