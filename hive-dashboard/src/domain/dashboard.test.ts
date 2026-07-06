import { describe, expect, it } from 'vitest'

import { projectsFromApi, relativeActivityAgeLabel } from './dashboard'

const currentTime = new Date('2026-07-04T12:00:00.000Z')

describe('relativeActivityAgeLabel', () => {
  it('returns unavailable when activity time is missing or invalid', () => {
    expect(relativeActivityAgeLabel(null, currentTime)).toBe('activity unavailable')
    expect(relativeActivityAgeLabel(undefined, currentTime)).toBe('activity unavailable')
    expect(relativeActivityAgeLabel('', currentTime)).toBe('activity unavailable')
    expect(relativeActivityAgeLabel('not-a-date', currentTime)).toBe('activity unavailable')
  })

  it('reports current or future activity as just now', () => {
    expect(relativeActivityAgeLabel('2026-07-04T12:00:00.000Z', currentTime)).toBe('just now')
    expect(relativeActivityAgeLabel('2026-07-04T12:05:00.000Z', currentTime)).toBe('just now')
  })

  it('formats elapsed activity age with minute, hour, and day buckets', () => {
    expect(relativeActivityAgeLabel('2026-07-04T11:59:01.000Z', currentTime)).toBe('just now')
    expect(relativeActivityAgeLabel('2026-07-04T11:59:00.000Z', currentTime)).toBe('1m ago')
    expect(relativeActivityAgeLabel('2026-07-04T11:01:00.000Z', currentTime)).toBe('59m ago')
    expect(relativeActivityAgeLabel('2026-07-04T11:00:00.000Z', currentTime)).toBe('1h ago')
    expect(relativeActivityAgeLabel('2026-07-03T13:00:00.000Z', currentTime)).toBe('23h ago')
    expect(relativeActivityAgeLabel('2026-07-03T12:00:00.000Z', currentTime)).toBe('1d ago')
  })
})

describe('projectsFromApi', () => {
  it('maps blocked project governance fields without changing active project defaults', () => {
    const view = projectsFromApi({
      total: 2,
      projects: [
        { name: 'Blocked Project', memoryCount: 0, sessionCount: 0, lastActivityAt: null, syncHealth: 'degraded', blocked: true, canonicalProjectKey: 'blocked-project', blockReason: 'duplicate import', exportMarker: 'export-123', blockAckStatus: 'applied' },
        { name: 'Active Project', memoryCount: 1, sessionCount: 1, lastActivityAt: null, syncHealth: 'healthy' }
      ]
    })

    expect(view.projects[0]).toMatchObject({ blocked: true, canonicalProjectKey: 'blocked-project', blockReason: 'duplicate import', exportMarker: 'export-123', blockAckStatus: 'applied' })
    expect(view.projects[1]).toMatchObject({ blocked: false, canonicalProjectKey: 'Active Project', blockReason: undefined, exportMarker: undefined, blockAckStatus: undefined })
  })
})
