import { describe, expect, it, vi } from 'vitest'
import { renderMemories } from './Memories'

const memory = { id: 'mem-1', sync_id: 'sync-1', project: 'jarvis-dev', category: 'decision', title: 'Dashboard scope', content: 'Observe API state only', tags: ['dashboard'], files_affected: [], created_by: 'admin-1', created_at: '2026-06-06T20:00:00Z', updated_at: '2026-06-06T20:01:00Z', synced_at: '2026-06-06T20:02:00Z' }
const listState = { status: 'ready' as const, data: { recent: { memories: [], total: 0, limit: 5, offset: 0 }, search: { memories: [], total: 0, query: 'dashboard', limit: 5, offset: 0 } } }

describe('memories view', () => {
  it('renders recent and searched memories from Hive API', () => {
    const view = renderMemories({ status: 'ready', data: { recent: { memories: [memory], total: 1, limit: 5, offset: 0 }, search: { memories: [{ ...memory, id: 'mem-2', title: 'Search result' }], total: 1, query: 'dashboard', limit: 5, offset: 0 } } })

    expect(view.textContent).toContain('Dashboard scope')
    expect(view.textContent).toContain('Search result')
    expect(view.textContent).toContain('decision · jarvis-dev')
  })

  it('renders empty recent and search states', () => {
    const view = renderMemories({ status: 'ready', data: { recent: { memories: [], total: 0, limit: 5, offset: 0 }, search: { memories: [], total: 0, query: 'missing', limit: 5, offset: 0 } } })

    expect(view.textContent).toContain('No recent memories found')
    expect(view.textContent).toContain('No memories matched "missing"')
  })

  it('renders ready memory detail fields from the loaded memory', () => {
    const richMemory = { ...memory, tags: ['dashboard', 'sdd'], files_affected: ['hive-dashboard/src/main.ts', 'hive-dashboard/src/views/Memories.ts'] }

    const view = renderMemories(listState, {
      detailRoute: { kind: 'valid', id: richMemory.id, routeKey: richMemory.id },
      detail: { status: 'ready', data: { routeId: richMemory.id, memory: richMemory } }
    })

    expect(view.querySelector('h2')?.textContent).toBe('Dashboard scope')
    expect(view.textContent).toContain('Observe API state only')
    expect(view.textContent).toContain('Project: jarvis-dev')
    expect(view.textContent).toContain('Category: decision')
    expect(view.textContent).toContain('Tag: dashboard')
    expect(view.textContent).toContain('Tag: sdd')
    expect(view.textContent).toContain('File: hive-dashboard/src/main.ts')
    expect(view.textContent).toContain('Created by: admin-1')
    expect(view.textContent).toContain('Created: 2026-06-06T20:00:00Z')
    expect(view.textContent).toContain('Updated: 2026-06-06T20:01:00Z')
    expect(view.textContent).toContain('Synced: 2026-06-06T20:02:00Z')
    expect(view.textContent).toContain('Sync ID: sync-1')
    expect(view.textContent).toContain('Memory ID: mem-1')
  })

  it('renders detail state controls without misleading optional placeholders', () => {
    const emptyOptionalMemory = { ...memory, tags: [], files_affected: [], synced_at: '' }

    const ready = renderMemories(listState, {
      detailRoute: { kind: 'valid', id: emptyOptionalMemory.id, routeKey: emptyOptionalMemory.id },
      detail: { status: 'ready', data: { routeId: emptyOptionalMemory.id, memory: emptyOptionalMemory } }
    })
    expect(ready.textContent).toContain('Dashboard scope')
    expect(ready.textContent).not.toContain('Tag:')
    expect(ready.textContent).not.toContain('File:')
    expect(ready.textContent).not.toContain('Synced:')
    expect(ready.textContent).not.toContain('No tags')
    expect(ready.textContent).not.toContain('No files')

    const loading = renderMemories(listState, { detailRoute: { kind: 'valid', id: 'mem-loading', routeKey: 'mem-loading' }, detail: { status: 'loading' } })
    expect(loading.querySelector('[role="status"]')?.textContent).toBe('Loading memory mem-loading…')

    const malformed = renderMemories(listState, { detailRoute: { kind: 'malformed', raw: '%E0%A4%A' } })
    expect(malformed.querySelector('[role="alert"]')?.textContent).toContain('Malformed memory ID')

    const failed = renderMemories(listState, { detailRoute: { kind: 'valid', id: 'missing-memory', routeKey: 'missing-memory' }, detail: { status: 'error', message: 'memory missing' } })
    expect(failed.querySelector('[role="alert"]')?.textContent).toContain('memory missing')
    expect(failed.textContent).not.toContain('sync-ID lookup')
  })

  it('wires Back to memories from detail mode and leaves list rendering unchanged', () => {
    const onBackToMemories = vi.fn()
    const detail = renderMemories(listState, {
      detailRoute: { kind: 'valid', id: memory.id, routeKey: memory.id },
      detail: { status: 'ready', data: { routeId: memory.id, memory } },
      onBackToMemories
    })

    detail.querySelector<HTMLButtonElement>('button')?.click()

    expect(onBackToMemories).toHaveBeenCalledTimes(1)

    const list = renderMemories({ status: 'ready', data: { recent: { memories: [memory], total: 1, limit: 5, offset: 0 }, search: { memories: [], total: 0, query: 'missing', limit: 5, offset: 0 } } })
    expect(list.querySelector('h2')?.textContent).toBe('Memories')
    expect(list.textContent).toContain('Recent memories')
    expect(list.textContent).toContain('Dashboard scope')
  })
})
