import { describe, expect, it } from 'vitest'
import { hiveOverviewFixture } from '../fixtures/hive-dashboard/overview'
import { insightsScreenFixtures } from '../fixtures/hive-dashboard/insights'
import { renderChart, type ChartInput } from './Chart'

describe('dashboard chart foundation', () => {
  it('renders time-series fixture data with accessible summary and ordered marks', () => {
    const chart = renderChart({
      kind: 'time-series',
      title: 'Knowledge Growth',
      summary: 'Total memories grew from February through June.',
      series: hiveOverviewFixture.knowledgeGrowth
    })

    expect(chart.getAttribute('role')).toBe('figure')
    expect(chart.getAttribute('data-chart-kind')).toBe('time-series')
    expect(chart.getAttribute('aria-label')).toBe('Knowledge Growth')
    expect(chart.querySelector('h2')?.textContent).toBe('Knowledge Growth')
    expect(chart.querySelector('.chart-summary')?.textContent).toBe('Total memories grew from February through June.')

    const marks = Array.from(chart.querySelectorAll('[data-chart-point]'))
    expect(marks.map((mark) => mark.textContent)).toEqual([
      'Feb16720',
      'Mar18140',
      'Apr19680',
      'May21110',
      'Jun22375'
    ])
    expect(marks.map((mark) => mark.getAttribute('aria-label'))).toEqual([
      'Knowledge Growth point Feb: 16720',
      'Knowledge Growth point Mar: 18140',
      'Knowledge Growth point Apr: 19680',
      'Knowledge Growth point May: 21110',
      'Knowledge Growth point Jun: 22375'
    ])
  })

  it('renders categorical fixture data with deterministic labels and category semantics', () => {
    const chart = renderChart({
      kind: 'categorical',
      title: 'Memories by category',
      summary: 'Memory totals grouped by category.',
      points: insightsScreenFixtures.analytics.memoriesByCategory
    })

    expect(chart.getAttribute('data-chart-kind')).toBe('categorical')
    expect(chart.querySelector('.chart-summary')?.textContent).toBe('Memory totals grouped by category.')

    const categories = Array.from(chart.querySelectorAll('[data-chart-category]'))
    expect(categories.map((category) => category.getAttribute('data-chart-category'))).toEqual([
      'architecture',
      'bugfix',
      'decision',
      'discovery',
      'pattern',
      'config',
      'preference',
      'session_summary'
    ])
    expect(categories.map((category) => category.getAttribute('aria-label'))).toEqual([
      'Memories by category category architecture: 4200',
      'Memories by category category bugfix: 3600',
      'Memories by category category decision: 3100',
      'Memories by category category discovery: 2800',
      'Memories by category category pattern: 2500',
      'Memories by category category config: 2200',
      'Memories by category category preference: 1800',
      'Memories by category category session_summary: 2200'
    ])
  })

  it('renders an accessible empty state without misleading marks', () => {
    const chart = renderChart({
      kind: 'categorical',
      title: 'Empty categories',
      summary: 'No category values are available.',
      points: []
    })

    expect(chart.getAttribute('role')).toBe('figure')
    expect(chart.querySelectorAll('[data-chart-point]')).toHaveLength(0)
    expect(chart.querySelector('.state')?.getAttribute('role')).toBe('status')
    expect(chart.querySelector('.state')?.getAttribute('data-state')).toBe('empty')
    expect(chart.querySelector('.state')?.textContent).toBe('No chart data is available for Empty categories.')
  })

  it('renders a safe fallback for unsupported chart input', () => {
    const chart = renderChart({ kind: 'radar', title: 'Unsupported analytics' } as unknown as ChartInput)

    expect(chart.getAttribute('role')).toBe('figure')
    expect(chart.getAttribute('data-chart-kind')).toBe('unsupported')
    expect(chart.getAttribute('aria-label')).toBe('Unsupported chart')
    expect(chart.querySelector('.state')?.getAttribute('role')).toBe('status')
    expect(chart.querySelector('.state')?.textContent).toBe('Unsupported chart data. Chart cannot be rendered.')
  })
})
