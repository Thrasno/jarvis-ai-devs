import { describe, expect, it } from 'vitest'
import { knowledgeBrowserFixture } from '../fixtures/hive-dashboard'
import { renderKnowledgeBrowser } from './KnowledgeBrowser'

describe('Knowledge Browser view', () => {
  it('composes Browse mode over the knowledge browser fixture with URL filters', () => {
    const view = renderKnowledgeBrowser({ status: 'ready', data: { items: knowledgeBrowserFixture.memories.slice(0, 2).map((memory) => ({ ...memory, highlights: [] })), total: 2, limit: 2, offset: 0, previousOffset: null, nextOffset: null } }, '?query=auth&limit=2')

    expect(view.querySelector('h2')?.textContent).toBe('Knowledge Browser')
    expect(view.querySelector('[role="note"]')?.textContent).toBe('Live Hive API data')
    expect(view.querySelector('input[name="query"]')).toBeNull()
    expect(Array.from(view.querySelectorAll('article[role="listitem"]')).map((card) => card.textContent)).toEqual([
      expect.stringContaining('Gateway owns the auth boundary, not services'),
      expect.stringContaining('Vector store is single-writer, replicas read-only')
    ])
  })

  it('keeps Browse mode source-limited instead of exposing search-only highlight markup', () => {
    const view = renderKnowledgeBrowser({ status: 'ready', data: { items: knowledgeBrowserFixture.memories.slice(0, 1).map((memory) => ({ ...memory, highlights: [] })), total: 1, limit: 1, offset: 0, previousOffset: null, nextOffset: null } }, '?query=auth&limit=1')

    expect(view.querySelector('mark')).toBeNull()
    expect(view.querySelector('a[href="/dashboard/memories/gateway-auth-boundary"]')?.textContent).toBe('Open memory')
  })
})
