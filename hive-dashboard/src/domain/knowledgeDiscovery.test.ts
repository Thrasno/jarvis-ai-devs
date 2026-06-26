import { describe, expect, it } from 'vitest'
import type { MemoryList, MemorySearch } from '../api/client'
import { memoryListToDiscoveryData, memorySearchToDiscoveryData } from './knowledgeDiscovery'

describe('knowledge discovery API mapping', () => {
  it('maps browse memory rows to discovery cards using created_at as savedAt with no highlights', () => {
    const page: MemoryList = {
      memories: [memory({ id: 'mem-1', created_at: '2026-06-06T20:00:00Z' })],
      total: 1,
      limit: 10,
      offset: 0
    }

    expect(memoryListToDiscoveryData(page)).toMatchObject({
      total: 1,
      limit: 10,
      offset: 0,
      previousOffset: null,
      nextOffset: null,
      items: [{
        id: 'mem-1',
        title: 'Dashboard scope',
        content: 'No daemon controls',
        category: 'decision',
        projectId: 'jarvis-dev',
        authorId: 'admin-1',
        authorLabel: 'admin-1',
        savedAt: '2026-06-06T20:00:00Z',
        savedAtLabel: '06 Jun 2026',
        highlights: []
      }]
    })
  })

  it('maps search responses as memory-only cards without search highlights', () => {
    const page: MemorySearch = {
      memories: [memory({ id: 'mem-2', title: 'Auth boundary', content: 'Gateway owns auth', tags: ['security'] })],
      total: 20,
      query: 'auth',
      limit: 5,
      offset: 10
    }

    const data = memorySearchToDiscoveryData(page)

    expect(data.items.map((item) => ({ id: item.id, title: item.title, highlights: item.highlights }))).toEqual([
      { id: 'mem-2', title: 'Auth boundary', highlights: [] }
    ])
    expect(data).toMatchObject({ total: 20, limit: 5, offset: 10, previousOffset: 5, nextOffset: 15 })
  })
})

function memory(overrides: Partial<MemoryList['memories'][number]> = {}): MemoryList['memories'][number] {
  return {
    id: 'mem-1',
    sync_id: 'sync-1',
    project: 'jarvis-dev',
    category: 'decision',
    title: 'Dashboard scope',
    content: 'No daemon controls',
    tags: [],
    files_affected: [],
    created_by: 'admin-1',
    created_at: '2026-06-06T20:00:00Z',
    updated_at: '2026-06-06T20:01:00Z',
    synced_at: '2026-06-06T20:02:00Z',
    ...overrides
  }
}
