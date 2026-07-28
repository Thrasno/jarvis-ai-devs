// @ts-expect-error Node types are not installed; Vitest still runs tests in Node.
import { readFileSync } from 'node:fs'
import { describe, expect, it, vi } from 'vitest'
import { projectsFromApi } from '../domain/dashboard'
import type { ProjectListResponse } from '../api/client'
import { renderProjects } from './Projects'

const styles = readFileSync('src/styles.css', 'utf8')

describe('projects view', () => {
  it('renders a bespoke Projects root with heading, repository count, and no generic panel chrome', () => {
    const view = renderProjects({ status: 'ready', data: projectsFromApi(projectResponse([healthyProject(), unknownProject(), degradedProject()])) })

    expect(view.matches('section[data-dashboard-view="projects"]')).toBe(true)
    expect(view.getAttribute('aria-labelledby')).toBe('dashboard-projects-title')
    expect(view.querySelector('#dashboard-projects-title')?.textContent).toBe('Projects')
    expect(view.querySelector('.dashboard-projects__eyebrow')?.textContent).toBe('ACCESSIBLE REPOSITORIES · 3')
    expect(view.getAttribute('data-dashboard-primitive')).toBeNull()
    expect(view.classList.contains('dashboard-panel')).toBe(false)
    expect(view.classList.contains('panel')).toBe(false)
  })

  it('renders segmented project cards with honest live labels and project-name Knowledge Browser links', () => {
    const view = renderProjects({ status: 'ready', data: projectsFromApi(projectResponse([healthyProject(), unknownProject(), degradedProject()])) })
    const cards = Array.from(view.querySelectorAll<HTMLElement>('[role="listitem"]'))
    const browseLinks = Array.from(view.querySelectorAll<HTMLAnchorElement>('a'))

    expect(cards).toHaveLength(3)
    expect(cards.map((card) => card.getAttribute('aria-label'))).toEqual([
      'Core API project: HEALTHY health, 4,821 memories, 17 sessions, last activity 27/06/26',
      'Search Index project: UNKNOWN health, 2,104 memories, 4 sessions, last activity unavailable',
      'Billing Worker project: DEGRADED health, 1,633 memories, 3 sessions, last activity 26/06/26'
    ])
    expect(cards[0].querySelector('.dashboard-project-card__identity h3')?.textContent).toBe('Core API')
    expect(cards[0].querySelector('.dashboard-project-card__identity .dashboard-project-card__health')?.textContent).toContain('HEALTHY')
    expect(cards[0].querySelector('.dashboard-project-card__metrics')).not.toBeNull()
    expect(cards[0].querySelector('.dashboard-project-card__actions')).not.toBeNull()
    expect(cards[0].textContent).toContain('Core API')
    expect(metricValue(cards[0], 'MEMORIES')).toBe('4,821')
    expect(metricValue(cards[0], 'SESSIONS')).toBe('17')
    expect(metricValue(cards[0], 'LAST ACTIVITY')).toBe('27/06/26')
    expect(cards[0].querySelector<HTMLAnchorElement>('a')?.textContent).toBe('browse memories →')
    expect(browseLinks.map((link) => link.getAttribute('aria-label'))).toEqual([
      'Browse memories for Core API',
      'Browse memories for Search Index',
      'Browse memories for Billing Worker'
    ])
    expect(cards[0].querySelector<HTMLAnchorElement>('a')?.getAttribute('href')).toBe('/dashboard/knowledgeBrowser?project=Core+API')
  })

  it('renders sync health uppercase and keeps decorative rails non-semantic', () => {
    const view = renderProjects({ status: 'ready', data: projectsFromApi(projectResponse([
      { name: 'Missing Health', memoryCount: 8, sessionCount: 1, lastActivityAt: '2020-01-01T00:00:00Z', syncHealth: null },
      { name: 'Unsupported Health', memoryCount: 12, sessionCount: 2, lastActivityAt: '2026-06-25T00:00:00Z', syncHealth: 'paused' }
    ])) })
    const cards = Array.from(view.querySelectorAll<HTMLElement>('[role="listitem"]'))

    expect(cards).toHaveLength(2)
    expect(cards.map((card) => card.querySelector('.dashboard-project-card__health')?.textContent)).toEqual(['UNKNOWN', 'UNKNOWN'])
    expect(cards[0].textContent).not.toContain('DEGRADED')
    for (const rail of view.querySelectorAll<HTMLElement>('.dashboard-project-card__rail')) {
      expect(rail.getAttribute('aria-hidden')).toBe('true')
      expect(rail.getAttribute('role')).not.toBe('progressbar')
      expect(rail.hasAttribute('aria-valuenow')).toBe(false)
      expect(rail.childElementCount).toBe(0)
    }
  })

  it('renders loading, error, and empty states without fixture fallback cards', () => {
    const loading = renderProjects({ status: 'loading' })
    expect(loading.matches('section[data-dashboard-view="projects"]')).toBe(true)
    expect(loading.querySelector('[role="status"]')?.textContent).toBe('Loading live project summaries…')
    expect(loading.querySelectorAll('[role="listitem"]')).toHaveLength(0)

    const failed = renderProjects({ status: 'error', message: 'projects API unavailable' })
    expect(failed.matches('section[data-dashboard-view="projects"]')).toBe(true)
    expect(failed.querySelector('[role="alert"]')?.textContent).toContain('projects API unavailable')
    expect(failed.querySelectorAll('[role="listitem"]')).toHaveLength(0)

    const empty = renderProjects({ status: 'ready', data: projectsFromApi(projectResponse([])) })
    expect(empty.matches('section[data-dashboard-view="projects"]')).toBe(true)
    expect(empty.querySelector('.dashboard-projects__eyebrow')?.textContent).toBe('ACCESSIBLE REPOSITORIES · 0')
    expect(empty.querySelector('[role="status"]')?.textContent).toBe('No live project summaries found.')
    expect(empty.querySelector('[role="alert"]')).toBeNull()
    expect(empty.querySelectorAll('[role="listitem"]')).toHaveLength(0)
  })

  it('does not render fixture-only region, contributor, developer, card, or last-sync claims', () => {
    const view = renderProjects({ status: 'ready', data: projectsFromApi(projectResponse([healthyProject()])) })
    const text = view.textContent ?? ''

    expect(text).not.toContain('eu-west-1')
    expect(text).not.toContain('contributors')
    expect(text).not.toContain('Contributors')
    expect(text).not.toContain('developers')
    expect(text).not.toContain('DEVS')
    expect(text).not.toContain('LAST SYNC')
    expect(text).not.toContain('Last sync')
    expect(text).not.toContain('developer count')
    expect(text).not.toContain('contributor count')
    expect(text).not.toContain('Demo fixture data')
  })

  it('encodes special project names in Knowledge Browser browse links', () => {
    const view = renderProjects({ status: 'ready', data: projectsFromApi(projectResponse([
      { name: 'team/alpha project', memoryCount: 1, sessionCount: 1, lastActivityAt: null, syncHealth: 'healthy' }
    ])) })

    expect(view.querySelector<HTMLAnchorElement>('a')?.getAttribute('href')).toBe('/dashboard/knowledgeBrowser?project=team%2Falpha+project')
  })

  it('keeps long unbroken project names accessible and allows the title flex item to wrap', () => {
    const name = 'project-with-an-extremely-long-unbroken-name-that-must-not-hide-health'
    const view = renderProjects({ status: 'ready', data: projectsFromApi(projectResponse([
      { name, memoryCount: 1, sessionCount: 1, lastActivityAt: null, syncHealth: 'healthy' }
    ])) })
    const card = view.querySelector<HTMLElement>('.dashboard-project-card')!

    expect(card.getAttribute('aria-label')).toContain(name)
    expect(card.querySelector('.dashboard-project-card__identity h3')?.textContent).toBe(name)
    expect(styles).toMatch(/\.dashboard-project-card__identity h3\s*{[^}]*min-width:\s*0;[^}]*overflow-wrap:\s*anywhere;/s)
  })

  it('renders blocked badges, status, reason, export marker, and admin quarantine form guard copy', () => {
    const view = renderProjects({ status: 'ready', data: projectsFromApi(projectResponse([
      { name: 'Blocked Project', memoryCount: 0, sessionCount: 0, lastActivityAt: null, syncHealth: 'degraded', blocked: true, canonicalProjectKey: 'blocked-project', blockReason: 'duplicate import', exportMarker: 'export-123', blockAckStatus: 'applied' }
    ])) }, { currentUserLevel: 'admin' })

    expect(view.textContent).toContain('BLOCKED')
    expect(view.textContent).toContain('Status: ACK applied')
    expect(view.textContent).toContain('Reason: duplicate import')
    expect(view.textContent).toContain('Export marker: export-123')
    expect(view.querySelector('form[aria-label="Block Blocked Project"]')).not.toBeNull()
    expect(view.querySelector('input[name="confirmation"]')?.getAttribute('placeholder')).toBe('blocked-project')
    expect(view.textContent).toContain('Type blocked-project exactly')
  })

  it('shows non-admin rejection guidance instead of block controls', () => {
    const view = renderProjects({ status: 'ready', data: projectsFromApi(projectResponse([healthyProject()])) }, { currentUserLevel: 'member' })

    expect(view.textContent).toContain('Admin access required to block or quarantine projects.')
    expect(view.querySelector('form[aria-label^="Block "]')).toBeNull()
  })

  it('rejects admin block submission until reason, export marker, and exact canonical confirmation are present', () => {
    const onBlockProject = vi.fn()
    const view = renderProjects({ status: 'ready', data: projectsFromApi(projectResponse([
      { name: 'Blocked Project', memoryCount: 0, sessionCount: 0, lastActivityAt: null, syncHealth: 'degraded', canonicalProjectKey: 'blocked-project' }
    ])) }, { currentUserLevel: 'admin', onBlockProject })
    const form = view.querySelector<HTMLFormElement>('form[aria-label="Block Blocked Project"]')!

    form.querySelector<HTMLInputElement>('input[name="reason"]')!.value = 'duplicate import'
    form.querySelector<HTMLInputElement>('input[name="export_marker"]')!.value = 'export-123'
    form.querySelector<HTMLInputElement>('input[name="confirmation"]')!.value = 'wrong-project'
    form.dispatchEvent(new SubmitEvent('submit', { bubbles: true, cancelable: true }))

    expect(onBlockProject).not.toHaveBeenCalled()
    expect(form.querySelector('[role="alert"]')?.textContent).toBe('Reason, export marker, and exact canonical confirmation are required.')
  })

  it('rejects padded canonical confirmation instead of trimming it before validation', () => {
    const onBlockProject = vi.fn()
    const view = renderProjects({ status: 'ready', data: projectsFromApi(projectResponse([
      { name: 'Blocked Project', memoryCount: 0, sessionCount: 0, lastActivityAt: null, syncHealth: 'degraded', canonicalProjectKey: 'blocked-project' }
    ])) }, { currentUserLevel: 'admin', onBlockProject })
    const form = view.querySelector<HTMLFormElement>('form[aria-label="Block Blocked Project"]')!

    form.querySelector<HTMLInputElement>('input[name="reason"]')!.value = 'duplicate import'
    form.querySelector<HTMLInputElement>('input[name="export_marker"]')!.value = 'export-123'
    form.querySelector<HTMLInputElement>('input[name="confirmation"]')!.value = ' blocked-project '
    form.dispatchEvent(new SubmitEvent('submit', { bubbles: true, cancelable: true }))

    expect(onBlockProject).not.toHaveBeenCalled()
    expect(form.querySelector('[role="alert"]')?.textContent).toBe('Reason, export marker, and exact canonical confirmation are required.')
  })

  it('preserves exact confirmation input in the admin block request and disables duplicate submits while pending', async () => {
    const onBlockProject = vi.fn(() => Promise.resolve())
    const view = renderProjects({ status: 'ready', data: projectsFromApi(projectResponse([
      { name: 'Blocked Project', memoryCount: 0, sessionCount: 0, lastActivityAt: null, syncHealth: 'degraded', canonicalProjectKey: 'blocked-project' }
    ])) }, { currentUserLevel: 'admin', onBlockProject, pendingBlockProject: 'Blocked Project' })
    const form = view.querySelector<HTMLFormElement>('form[aria-label="Block Blocked Project"]')!

    expect(form.querySelector<HTMLButtonElement>('button[type="submit"]')?.disabled).toBe(true)
    expect(form.querySelector('[role="status"]')?.textContent).toBe('Quarantine request in progress…')
  })

  it('shows project block mutation and refresh errors in the governance form', () => {
    const view = renderProjects({ status: 'ready', data: projectsFromApi(projectResponse([
      { name: 'Blocked Project', memoryCount: 0, sessionCount: 0, lastActivityAt: null, syncHealth: 'degraded', canonicalProjectKey: 'blocked-project' }
    ])) }, { currentUserLevel: 'admin', mutationError: 'Project block failed: forbidden.', refreshError: 'Block succeeded, but Projects could not be refreshed: timeout.' })

    const alerts = Array.from(view.querySelectorAll('[role="alert"]')).map((node) => node.textContent)
    expect(alerts).toContain('Project block failed: forbidden.')
    expect(alerts).toContain('Block succeeded, but Projects could not be refreshed: timeout.')
  })
})

function metricValue(card: HTMLElement, label: string): string | undefined {
  const terms = Array.from(card.querySelectorAll('dt'))
  const term = terms.find((node) => node.textContent === label)
  return term?.nextElementSibling?.textContent ?? undefined
}

function projectResponse(projects: ProjectListResponse['projects']): ProjectListResponse {
  return { projects, total: projects.length }
}

function healthyProject(): ProjectListResponse['projects'][number] {
  return project('Core API', 'healthy', 4821, 17, '2026-06-27T09:30:00Z')
}

function degradedProject(): ProjectListResponse['projects'][number] {
  return project('Billing Worker', 'degraded', 1633, 3, '2026-06-26T08:15:00Z')
}

function unknownProject(): ProjectListResponse['projects'][number] {
  return project('Search Index', 'unknown', 2104, 4, null)
}

function project(
  name: string,
  syncHealth: ProjectListResponse['projects'][number]['syncHealth'],
  memoryCount: number,
  sessionCount: number,
  lastActivityAt: string | null
): ProjectListResponse['projects'][number] {
  return { name, memoryCount, sessionCount, lastActivityAt, syncHealth }
}
