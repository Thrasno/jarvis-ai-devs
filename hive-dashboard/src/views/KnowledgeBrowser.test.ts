import { describe, expect, it } from 'vitest'
import { knowledgeBrowserFixture } from '../fixtures/hive-dashboard'
import { renderKnowledgeBrowser } from './KnowledgeBrowser'

describe('Knowledge Browser view', () => {
  it('composes Browse mode over the knowledge browser fixture with URL filters', () => {
    const view = renderKnowledgeBrowser('?query=auth&limit=2')

    expect(view.querySelector('h2')?.textContent).toBe('Knowledge Browser')
    expect(view.querySelector('[role="note"]')?.textContent).toBe(knowledgeBrowserFixture.sourceLabel)
    expect(view.querySelector('input[name="query"]')?.getAttribute('value')).toBe('auth')
    expect(Array.from(view.querySelectorAll('article[role="listitem"]')).map((card) => card.textContent)).toEqual([
      expect.stringContaining('Gateway owns the auth boundary, not services'),
      expect.stringContaining('Race condition in token refresh on cold start')
    ])
  })

  it('keeps Browse mode source-limited instead of exposing search-only highlight markup', () => {
    const view = renderKnowledgeBrowser('?query=auth&limit=1')

    expect(view.querySelector('mark')).toBeNull()
    expect(view.querySelector('a[href="/dashboard/memories/gateway-auth-boundary"]')?.textContent).toBe('Open memory')
  })
})
