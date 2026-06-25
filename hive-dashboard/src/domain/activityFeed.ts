import type { ActivityFeedEntry, ActivityFeedResponse } from '../api/client'
import { memoryCategories, type ActivityEntryViewModel, type ActivityFeedViewModel, type ActivityGroupViewModel, type MemoryCategory } from './dashboard'

type ActivityEntryWithDate = ActivityEntryViewModel & { occurredAt: string }

export function activityFeedFromApi(response: ActivityFeedResponse, now = new Date()): ActivityFeedViewModel {
  return {
    screen: 'activityFeed',
    groups: groupEntries(response.entries, now),
    livePolling: disabledLivePolling(),
    nextCursor: response.next_cursor || undefined,
    loadingMore: false
  }
}

export function appendActivityPage(current: ActivityFeedViewModel, response: ActivityFeedResponse, now = new Date()): ActivityFeedViewModel {
  const seen = new Set(current.groups.flatMap((group) => group.entries.map((entry) => entry.id)))
  const currentEntries = current.groups.flatMap((group) => group.entries) as ActivityEntryWithDate[]
  const appendedEntries = response.entries
    .map(toEntryViewModel)
    .filter((entry) => {
      if (seen.has(entry.id)) return false
      seen.add(entry.id)
      return true
    })

  return {
    ...current,
    groups: groupViewEntries([...currentEntries, ...appendedEntries], now),
    nextCursor: response.next_cursor || undefined,
    loadingMore: false,
    paginationError: undefined
  }
}

function disabledLivePolling(): ActivityFeedViewModel['livePolling'] {
  return { enabled: false, intervalSeconds: 0 }
}

function groupEntries(entries: readonly ActivityFeedEntry[], now: Date): readonly ActivityGroupViewModel[] {
  return groupViewEntries(entries.map(toEntryViewModel), now)
}

function groupViewEntries(entries: readonly ActivityEntryWithDate[], now: Date): readonly ActivityGroupViewModel[] {
  const groups = new Map<string, ActivityEntryViewModel[]>()
  for (const entry of entries) {
    const dateLabel = dateGroupLabel(entry.occurredAt, now)
    const groupEntries = groups.get(dateLabel) ?? []
    groupEntries.push(entry)
    groups.set(dateLabel, groupEntries)
  }
  return Array.from(groups, ([dateLabel, groupEntries]) => ({ dateLabel, entries: groupEntries }))
}

function toEntryViewModel(entry: ActivityFeedEntry): ActivityEntryWithDate {
  return {
    id: entry.id,
    title: entry.title,
    actorHandle: entry.actor,
    projectId: entry.project,
    category: categoryFor(entry.category),
    timeLabel: timeLabel(entry.occurred_at),
    memorySyncId: entry.memory_sync_id || undefined,
    occurredAt: entry.occurred_at
  }
}

function dateGroupLabel(value: string, now: Date): string {
  const date = new Date(value)
  if (sameUtcDate(date, now)) return 'Today'
  const yesterday = new Date(Date.UTC(now.getUTCFullYear(), now.getUTCMonth(), now.getUTCDate() - 1))
  if (sameUtcDate(date, yesterday)) return 'Yesterday'
  return new Intl.DateTimeFormat('en-GB', { day: '2-digit', month: 'short', timeZone: 'UTC' }).format(date)
}

function sameUtcDate(left: Date, right: Date): boolean {
  return left.getUTCFullYear() === right.getUTCFullYear()
    && left.getUTCMonth() === right.getUTCMonth()
    && left.getUTCDate() === right.getUTCDate()
}

function timeLabel(value: string): string {
  const date = new Date(value)
  return new Intl.DateTimeFormat('en-GB', { hour: '2-digit', minute: '2-digit', timeZone: 'UTC' }).format(date)
}

function categoryFor(value: string): MemoryCategory {
  return memoryCategories.includes(value as MemoryCategory) ? value as MemoryCategory : 'discovery'
}
