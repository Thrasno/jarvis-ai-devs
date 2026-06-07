import { describe, expect, it } from 'vitest'
import { renderMemories } from './Memories'

const memory = { id: 'mem-1', sync_id: 'sync-1', project: 'jarvis-dev', category: 'decision', title: 'Dashboard scope', content: 'Observe API state only', tags: ['dashboard'], files_affected: [], created_by: 'admin-1', created_at: '2026-06-06T20:00:00Z', updated_at: '2026-06-06T20:01:00Z', synced_at: '2026-06-06T20:02:00Z' }

describe('memories view', () => {
  it('renders recent and searched memories from Hive API', () => {
    const view = renderMemories({ status: 'ready', data: { recent: { memories: [memory], total: 1, limit: 5, offset: 0 }, search: { memories: [{ ...memory, id: 'mem-2', title: 'Search result' }], total: 1, query: 'dashboard', limit: 5 } } })

    expect(view.textContent).toContain('Dashboard scope')
    expect(view.textContent).toContain('Search result')
    expect(view.textContent).toContain('decision · jarvis-dev')
  })

  it('renders empty recent and search states', () => {
    const view = renderMemories({ status: 'ready', data: { recent: { memories: [], total: 0, limit: 5, offset: 0 }, search: { memories: [], total: 0, query: 'missing', limit: 5 } } })

    expect(view.textContent).toContain('No recent memories found')
    expect(view.textContent).toContain('No memories matched "missing"')
  })
})
