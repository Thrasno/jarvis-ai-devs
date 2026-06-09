import { describe, expect, it } from 'vitest'
import { appendDashboardFilters, parseDashboardFilters, serializeDashboardFilters } from './urlFilters'

describe('dashboard URL filters', () => {
  it('serializes supported filters in a stable shareable order', () => {
    expect(serializeDashboardFilters({
      offset: 20,
      limit: 10,
      outcome: 'success',
      action: 'sync_push',
      tags: ['api contract', 'dashboard'],
      until: '2026-06-07T00:00:00Z',
      from: '2026-06-01T00:00:00Z',
      author: 'dev-1',
      category: 'decision',
      project: 'jarvis-dev',
      query: 'dashboard filters'
    })).toBe('query=dashboard+filters&project=jarvis-dev&category=decision&author=dev-1&from=2026-06-01T00%3A00%3A00Z&until=2026-06-07T00%3A00%3A00Z&tag=api+contract&tag=dashboard&action=sync_push&outcome=success&limit=10&offset=20')
  })

  it('omits empty scalar values, empty tags, null, undefined, and invalid numbers', () => {
    expect(serializeDashboardFilters({
      query: '',
      project: null,
      category: undefined,
      author: '  ',
      tags: ['dashboard', '', '  '],
      limit: 0,
      offset: -1
    })).toBe('tag=dashboard')
  })

  it('parses scalar filters, repeated tags, and safe numbers from URLSearchParams', () => {
    const filters = parseDashboardFilters('?category=decision&category=bugfix&tag=dashboard&tag=api+contract&limit=25&offset=50&query=dashboard+filters')

    expect(filters).toEqual({
      query: 'dashboard filters',
      category: 'decision',
      tags: ['dashboard', 'api contract'],
      limit: 25,
      offset: 50
    })
  })

  it('omits invalid parsed numbers instead of leaking unsafe defaults', () => {
    expect(parseDashboardFilters('limit=zero&limit=0&offset=-1&query=dashboard')).toEqual({ query: 'dashboard' })
  })

  it('keeps offset zero but rejects limit zero to match production list validation', () => {
    expect(serializeDashboardFilters({ limit: 1, offset: 0 })).toBe('limit=1&offset=0')
    expect(parseDashboardFilters('limit=1&offset=0')).toEqual({ limit: 1, offset: 0 })
  })

  it('round-trips values safely through URLSearchParams encoding', () => {
    const original = { query: 'sync + dashboard?', project: 'jarvis/dev', tags: ['a&b', 'c=d'], limit: 5, offset: 0 }

    expect(parseDashboardFilters(serializeDashboardFilters(original))).toEqual(original)
  })

  it('appends filters only when serialized filters are present', () => {
    expect(appendDashboardFilters('/memories', { project: 'jarvis-dev', limit: 5 })).toBe('/memories?project=jarvis-dev&limit=5')
    expect(appendDashboardFilters('/memories', { query: '' })).toBe('/memories')
  })

  it('preserves existing query params when appending dashboard filters', () => {
    expect(appendDashboardFilters('/memories?view=cards', { category: 'decision', offset: 10 })).toBe('/memories?view=cards&category=decision&offset=10')
  })
})
