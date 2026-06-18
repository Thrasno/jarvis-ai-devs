import { describe, expect, it } from 'vitest'
import type { ProjectListFixtureViewModel, ProjectPrimitiveViewModel } from '../domain/dashboard'
import { renderProjects } from './Projects'

describe('projects view', () => {
  it('renders sorted project cards with required fields and project-scoped browse links', () => {
    const view = renderProjects({ status: 'ready', data: projectList([healthyProject(), unknownProject(), degradedProject()]) })
    const cards = Array.from(view.querySelectorAll<HTMLElement>('[role="listitem"]'))
    const browseLinks = Array.from(view.querySelectorAll<HTMLAnchorElement>('a'))

    expect(cards).toHaveLength(3)
    expect(cards.map((card) => card.getAttribute('aria-label'))).toEqual([
      'Billing Worker project: Degraded health, 1,633 memories, 3 contributors, last synced 38m ago',
      'Search Index project: Unknown health, 2,104 memories, 4 contributors, last synced 1d ago',
      'Core API project: Healthy health, 4,821 memories, 6 contributors, last synced 2m ago'
    ])
    expect(cards[0].textContent).toContain('Billing Worker')
    expect(cards[0].textContent).toContain('us-east-1')
    expect(cards[0].textContent).toContain('1,633 memories')
    expect(cards[0].textContent).toContain('3 contributors')
    expect(cards[0].textContent).toContain('Last sync: 38m ago')
    expect(cards[0].textContent).toContain('Health: Degraded')
    expect(cards[0].querySelector<HTMLAnchorElement>('a')?.textContent).toBe('Browse memories')
    expect(browseLinks.map((link) => link.getAttribute('aria-label'))).toEqual([
      'Browse memories for Billing Worker',
      'Browse memories for Search Index',
      'Browse memories for Core API'
    ])
    expect(cards[0].querySelector<HTMLAnchorElement>('a')?.getAttribute('href')).toBe('/dashboard/memories?project=billing-worker')
  })

  it('uses the project list reference date for health derivation and ordering', () => {
    const view = renderProjects({
      status: 'ready',
      data: projectList(
        [
          project('almost-stale', 'Almost Stale', 'eu-west-1', 'healthy', 12, 2, '1d ago', '2024-06-10T00:00:00.000Z'),
          project('older-stale', 'Older Stale', 'eu-west-1', 'healthy', 8, 1, '2d ago', '2024-06-09T00:00:00.000Z')
        ],
        '2026-06-09T00:00:00.000Z'
      )
    })
    const cards = Array.from(view.querySelectorAll<HTMLElement>('[role="listitem"]'))

    expect(cards.map((card) => card.querySelector('h3')?.textContent)).toEqual(['Older Stale', 'Almost Stale'])
    expect(cards[0].textContent).toContain('Health: Degraded')
    expect(cards[1].textContent).toContain('Health: Healthy')
  })

  it('renders a non-live source note without claiming fixture cards are production data', () => {
    const view = renderProjects({ status: 'ready', data: projectList([healthyProject()]) })
    const note = view.querySelector('[role="note"]')

    expect(note?.textContent).toBe('Demo fixture data — live project summaries are unavailable.')
    expect(note?.textContent).toMatch(/fixture data/i)
    expect(note?.textContent).toMatch(/live project summaries are unavailable/i)
    expect(view.textContent).not.toMatch(/live production|production data/i)
  })

  it('renders unknown health distinctly from degraded health', () => {
    const view = renderProjects({ status: 'ready', data: projectList([unknownProject(), degradedProject()]) })
    const [degraded, unknown] = Array.from(view.querySelectorAll<HTMLElement>('[role="listitem"]'))

    expect(degraded.textContent).toContain('Health: Degraded')
    expect(degraded.textContent).not.toContain('Health: Unknown')
    expect(unknown.textContent).toContain('Health: Unknown')
    expect(unknown.textContent).not.toContain('Health: Degraded')
  })

  it('renders a non-error empty state when no project summaries are available', () => {
    const view = renderProjects({ status: 'ready', data: projectList([]) })

    expect(view.getAttribute('role')).toBe('region')
    expect(view.querySelector('[role="alert"]')).toBeNull()
    expect(view.querySelector('[role="status"]')?.textContent).toBe('No project summaries are available. Demo fixture data — live project summaries are unavailable.')
    expect(view.querySelectorAll('[role="listitem"]')).toHaveLength(0)
    expect(view.textContent).not.toContain('0 projects')
  })
})

function projectList(projects: readonly ProjectPrimitiveViewModel[], healthEvaluationDate = '2026-06-18T00:00:00.000Z'): ProjectListFixtureViewModel {
  return {
    screen: 'projects',
    totalProjects: projects.length,
    sourceLabel: 'Demo fixture data — live project summaries are unavailable.',
    healthEvaluationDate,
    projects
  }
}

function healthyProject(): ProjectPrimitiveViewModel {
  return project('core-api', 'Core API', 'eu-west-1', 'healthy', 4821, 6, '2m ago', '2026-06-06T01:37:00.000Z')
}

function degradedProject(): ProjectPrimitiveViewModel {
  return project('billing-worker', 'Billing Worker', 'us-east-1', 'degraded', 1633, 3, '38m ago', '2026-06-04T09:10:00.000Z')
}

function unknownProject(): ProjectPrimitiveViewModel {
  return project('search-index', 'Search Index', 'us-east-1', 'unknown', 2104, 4, '1d ago', null)
}

function project(
  id: string,
  name: string,
  region: string,
  status: ProjectPrimitiveViewModel['status'],
  memoryCount: number,
  contributorCount: number,
  lastSyncLabel: string,
  lastMemoryAt: string | null
): ProjectPrimitiveViewModel {
  return { id, name, region, status, memoryCount, contributorCount, lastSyncLabel, lastMemoryAt }
}
