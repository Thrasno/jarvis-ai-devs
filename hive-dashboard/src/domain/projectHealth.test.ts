import { describe, expect, it } from 'vitest'

import { deriveProjectHealth, sortProjectSummaries, type ProjectHealthInput } from './projectHealth'

const evaluationDate = new Date('2026-06-18T00:00:00.000Z')

describe('deriveProjectHealth', () => {
  it('marks projects with no memories for at least two years as degraded', () => {
    expect(
      deriveProjectHealth(
        {
          status: 'healthy',
          memoryCount: 15,
          lastMemoryAt: '2024-06-18T00:00:00.000Z'
        },
        evaluationDate
      )
    ).toBe('degraded')
  })

  it('returns unknown when health inputs are missing or invalid without a usable status', () => {
    expect(deriveProjectHealth({ memoryCount: 0 }, evaluationDate)).toBe('unknown')
    expect(deriveProjectHealth({ status: null, memoryCount: 4, lastMemoryAt: 'not-a-date' }, evaluationDate)).toBe('unknown')
  })

  it('keeps explicit healthy and unknown statuses distinct when freshness does not override them', () => {
    expect(deriveProjectHealth({ status: 'healthy', memoryCount: 8, lastMemoryAt: '2026-06-17T10:00:00.000Z' }, evaluationDate)).toBe('healthy')
    expect(deriveProjectHealth({ status: 'unknown', memoryCount: 8, lastMemoryAt: '2026-06-17T10:00:00.000Z' }, evaluationDate)).toBe('unknown')
  })
})

describe('sortProjectSummaries', () => {
  it('orders degraded projects first, unknown projects second, and healthy projects last', () => {
    const projects = [
      project('web-client', { status: 'healthy', memoryCount: 12 }),
      project('billing-worker', { status: 'degraded', memoryCount: 9 }),
      project('search-index', { status: 'unknown', memoryCount: 7 })
    ]

    expect(sortProjectSummaries(projects, evaluationDate).map((summary) => summary.name)).toEqual([
      'billing-worker',
      'search-index',
      'web-client'
    ])
  })

  it('uses case-insensitive project names as the stable tie-breaker inside health buckets', () => {
    const projects = [
      project('mobile-sdk', { status: 'degraded', memoryCount: 4 }),
      project('Auth-Service', { status: 'degraded', memoryCount: 6 }),
      project('core-api', { status: 'healthy', memoryCount: 3 }),
      project('analytics', { status: 'unknown', memoryCount: 2 })
    ]

    expect(sortProjectSummaries(projects, evaluationDate).map((summary) => summary.name)).toEqual([
      'Auth-Service',
      'mobile-sdk',
      'analytics',
      'core-api'
    ])
  })
})

function project(name: string, health: ProjectHealthInput) {
  return {
    id: name.toLowerCase(),
    name,
    region: 'eu-west-1',
    contributorCount: 2,
    lastSyncLabel: '1m ago',
    ...health
  }
}
