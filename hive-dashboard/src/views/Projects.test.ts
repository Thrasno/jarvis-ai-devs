import { describe, expect, it } from 'vitest'
import { projectsFromApi } from '../domain/dashboard'
import type { ProjectListResponse } from '../api/client'
import { renderProjects } from './Projects'

describe('projects view', () => {
  it('renders live project cards with backend fields and project-name Knowledge Browser links', () => {
    const view = renderProjects({ status: 'ready', data: projectsFromApi(projectResponse([healthyProject(), unknownProject(), degradedProject()])) })
    const cards = Array.from(view.querySelectorAll<HTMLElement>('[role="listitem"]'))
    const browseLinks = Array.from(view.querySelectorAll<HTMLAnchorElement>('a'))

    expect(cards).toHaveLength(3)
    expect(cards.map((card) => card.getAttribute('aria-label'))).toEqual([
      'Core API project: Healthy health, 4,821 memories, 17 sessions, last activity Jun 27, 2026, 09:30',
      'Search Index project: Unknown health, 2,104 memories, 4 sessions, last activity unavailable',
      'Billing Worker project: Degraded health, 1,633 memories, 3 sessions, last activity Jun 26, 2026, 08:15'
    ])
    expect(cards[0].textContent).toContain('Core API')
    expect(cards[0].textContent).toContain('4,821 memories')
    expect(cards[0].textContent).toContain('17 sessions')
    expect(cards[0].textContent).toContain('Last activity: Jun 27, 2026, 09:30')
    expect(cards[0].textContent).toContain('Health: Healthy')
    expect(cards[0].querySelector<HTMLAnchorElement>('a')?.textContent).toBe('Open in Knowledge Browser')
    expect(browseLinks.map((link) => link.getAttribute('aria-label'))).toEqual([
      'Open Core API in Knowledge Browser',
      'Open Search Index in Knowledge Browser',
      'Open Billing Worker in Knowledge Browser'
    ])
    expect(cards[0].querySelector<HTMLAnchorElement>('a')?.getAttribute('href')).toBe('/dashboard/knowledgeBrowser?project=Core+API')
  })

  it('normalizes missing and unsupported sync health without stale-date inference', () => {
    const view = renderProjects({ status: 'ready', data: projectsFromApi(projectResponse([
      { name: 'Missing Health', memoryCount: 8, sessionCount: 1, lastActivityAt: '2020-01-01T00:00:00Z', syncHealth: null },
      { name: 'Unsupported Health', memoryCount: 12, sessionCount: 2, lastActivityAt: '2026-06-25T00:00:00Z', syncHealth: 'paused' }
    ])) })
    const cards = Array.from(view.querySelectorAll<HTMLElement>('[role="listitem"]'))

    expect(cards).toHaveLength(2)
    expect(cards[0].textContent).toContain('Health: Unknown')
    expect(cards[1].textContent).toContain('Health: Unknown')
    expect(cards[0].textContent).not.toContain('Health: Degraded')
  })

  it('renders loading, error, and empty states without fixture fallback cards', () => {
    const loading = renderProjects({ status: 'loading' })
    expect(loading.querySelector('[role="status"]')?.textContent).toBe('Loading live project summaries…')
    expect(loading.querySelectorAll('[role="listitem"]')).toHaveLength(0)

    const failed = renderProjects({ status: 'error', message: 'projects API unavailable' })
    expect(failed.querySelector('[role="alert"]')?.textContent).toContain('projects API unavailable')
    expect(failed.querySelectorAll('[role="listitem"]')).toHaveLength(0)

    const empty = renderProjects({ status: 'ready', data: projectsFromApi(projectResponse([])) })
    expect(empty.querySelector('[role="status"]')?.textContent).toBe('No live project summaries found.')
    expect(empty.querySelector('[role="alert"]')).toBeNull()
    expect(empty.querySelectorAll('[role="listitem"]')).toHaveLength(0)
  })

  it('does not render fixture-only region, contributor, developer, card, or last-sync claims', () => {
    const view = renderProjects({ status: 'ready', data: projectsFromApi(projectResponse([healthyProject()])) })
    const text = view.textContent ?? ''

    expect(text).not.toContain('eu-west-1')
    expect(text).not.toContain('contributors')
    expect(text).not.toContain('developers')
    expect(text).not.toContain('Last sync')
    expect(text).not.toContain('Demo fixture data')
  })

  it('encodes special project names in Knowledge Browser browse links', () => {
    const view = renderProjects({ status: 'ready', data: projectsFromApi(projectResponse([
      { name: 'team/alpha project', memoryCount: 1, sessionCount: 1, lastActivityAt: null, syncHealth: 'healthy' }
    ])) })

    expect(view.querySelector<HTMLAnchorElement>('a')?.getAttribute('href')).toBe('/dashboard/knowledgeBrowser?project=team%2Falpha+project')
  })
})

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
