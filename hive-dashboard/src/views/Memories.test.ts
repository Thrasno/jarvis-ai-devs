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
    const richMemory = { ...memory, title: '  Complete stored title: API_v2 -- unchanged  ', content: '# Decision\n\nUse **safe Markdown**.', tags: ['dashboard', 'sdd'], files_affected: ['hive-dashboard/src/main.ts', 'hive-dashboard/src/views/Memories.ts'] }

    const view = renderMemories(listState, {
      detailRoute: { kind: 'valid', id: richMemory.id, routeKey: richMemory.id },
      detail: { status: 'ready', data: { routeId: richMemory.id, memory: richMemory } }
    })

    expect(view.querySelector('header h1')?.textContent).toBe(richMemory.title)
    expect(view.querySelector('main .dashboard-markdown h1')?.textContent).toBe('Decision')
    expect(view.querySelector('main .dashboard-markdown strong')?.textContent).toBe('safe Markdown')
    expect(view.querySelector('aside[aria-labelledby="memory-details-title"]')).not.toBeNull()
    expect(view.textContent).toContain('Projectjarvis-dev')
    expect(view.textContent).toContain('Categorydecision')
    expect(view.querySelectorAll('.memory-detail__tag')).toHaveLength(2)
    expect(view.querySelectorAll('.memory-detail__file')).toHaveLength(2)
    expect(view.querySelector('time[datetime="2026-06-06T20:00:00Z"]')?.textContent).not.toBe('2026-06-06T20:00:00Z')
    const identifiers = view.querySelector('details')
    expect(identifiers?.open).toBe(false)
    expect(identifiers?.querySelector('summary')?.textContent).toBe('Technical identifiers')
    expect(identifiers?.textContent).toContain('sync-1')
    expect(identifiers?.textContent).toContain('mem-1')
  })

  it('renders detail state controls without misleading optional placeholders', () => {
    const emptyOptionalMemory = { ...memory, tags: [], files_affected: [], synced_at: '' }

    const ready = renderMemories(listState, {
      detailRoute: { kind: 'valid', id: emptyOptionalMemory.id, routeKey: emptyOptionalMemory.id },
      detail: { status: 'ready', data: { routeId: emptyOptionalMemory.id, memory: emptyOptionalMemory } }
    })
    expect(ready.textContent).toContain('Dashboard scope')
    expect(ready.textContent).toContain('No tags')
    expect(ready.textContent).toContain('No affected files')
    expect(ready.textContent).toContain('Not synced')

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

    expect(detail.querySelector<HTMLButtonElement>('.memory-detail__back')?.textContent).toBe('← Back to memories')
    detail.querySelector<HTMLButtonElement>('.memory-detail__back')?.click()

    expect(onBackToMemories).toHaveBeenCalledTimes(1)

    const list = renderMemories({ status: 'ready', data: { recent: { memories: [memory], total: 1, limit: 5, offset: 0 }, search: { memories: [], total: 0, query: 'missing', limit: 5, offset: 0 } } })
    expect(list.querySelector('h2')?.textContent).toBe('Memories')
    expect(list.textContent).toContain('Recent memories')
    expect(list.textContent).toContain('Dashboard scope')
  })

  it('renders empty content as an accessible state through the document area', () => {
    const empty = { ...memory, content: '' }
    const view = renderMemories(listState, {
      detailRoute: { kind: 'valid', id: empty.id, routeKey: empty.id },
      detail: { status: 'ready', data: { routeId: empty.id, memory: empty } }
    })

    expect(view.querySelector('main [role="status"]')?.textContent).toBe('This memory has no content.')
  })

  it('copies original Markdown and the stable detail link with accessible success feedback', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined)
    Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText } })
    history.replaceState(null, '', '/dashboard/memories/mem-1?returnTo=%2Fdashboard%2Fknowledge%3Fquery%3Dauth')
    const view = renderMemories(listState, {
      detailRoute: { kind: 'valid', id: memory.id, routeKey: memory.id },
      detail: { status: 'ready', data: { routeId: memory.id, memory } }
    })

    buttonNamed(view, 'Copy Markdown').click()
    await vi.waitFor(() => expect(writeText).toHaveBeenCalledWith(memory.content))
    await vi.waitFor(() => expect(view.querySelector('[role="status"]')?.textContent).toBe('Markdown copied.'))

    buttonNamed(view, 'Copy link').click()
    await vi.waitFor(() => expect(writeText).toHaveBeenLastCalledWith(window.location.href))
    await vi.waitFor(() => expect(view.querySelector('[role="status"]')?.textContent).toBe('Link copied.'))
  })

  it.each([
    ['Clipboard rejection', { writeText: vi.fn().mockRejectedValue(new Error('denied')) }],
    ['Clipboard unavailable', undefined]
  ])('falls back when %s', async (_scenario, clipboard) => {
    Object.defineProperty(navigator, 'clipboard', { configurable: true, value: clipboard })
    const execCommand = vi.fn().mockReturnValue(true)
    Object.defineProperty(document, 'execCommand', { configurable: true, value: execCommand })
    const view = renderMemories(listState, {
      detailRoute: { kind: 'valid', id: memory.id, routeKey: memory.id },
      detail: { status: 'ready', data: { routeId: memory.id, memory } }
    })

    buttonNamed(view, 'Copy Markdown').click()

    await vi.waitFor(() => expect(execCommand).toHaveBeenCalledWith('copy'))
    expect(view.querySelector('[role="status"]')?.textContent).toBe('Markdown copied.')
  })

  it('announces final copy failure when Clipboard API and fallback both fail', async () => {
    Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText: vi.fn().mockRejectedValue(new Error('denied')) } })
    Object.defineProperty(document, 'execCommand', { configurable: true, value: vi.fn().mockReturnValue(false) })
    const view = renderMemories(listState, {
      detailRoute: { kind: 'valid', id: memory.id, routeKey: memory.id },
      detail: { status: 'ready', data: { routeId: memory.id, memory } }
    })

    buttonNamed(view, 'Copy link').click()

    await vi.waitFor(() => expect(view.querySelector('[role="alert"]')?.textContent).toBe('Could not copy link.'))
  })
})

function buttonNamed(root: HTMLElement, name: string): HTMLButtonElement {
  const button = Array.from(root.querySelectorAll('button')).find((candidate) => candidate.textContent === name)
  if (!button) throw new Error(`Missing button: ${name}`)
  return button
}
