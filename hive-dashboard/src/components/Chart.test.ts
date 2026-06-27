import { describe, expect, it } from 'vitest'
import { hiveOverviewFixture } from '../fixtures/hive-dashboard/overview'
import { insightsScreenFixtures } from '../fixtures/hive-dashboard/insights'
import { renderChart, type ChartInput } from './Chart'

describe('dashboard chart foundation', () => {
  it('renders time-series as SVG with accessible wrapper and SVG elements', () => {
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

    // SVG must be present with structural elements
    const svg = chart.querySelector('svg')
    expect(svg).not.toBeNull()
    expect(svg?.querySelector('polyline')).not.toBeNull()
    expect(svg?.querySelector('polygon')).not.toBeNull()
    expect(svg?.querySelector('line')).not.toBeNull()
  })

  it('renders categorical fixture data as horizontal bar chart SVG', () => {
    const chart = renderChart({
      kind: 'categorical',
      title: 'Memories by category',
      summary: 'Memory totals grouped by category.',
      points: insightsScreenFixtures.analytics.memoriesByCategory
    })

    expect(chart.getAttribute('data-chart-kind')).toBe('categorical')
    expect(chart.querySelector('.chart-summary')?.textContent).toBe('Memory totals grouped by category.')

    // SVG must be present with rect bar elements
    const svg = chart.querySelector('svg')
    expect(svg).not.toBeNull()
    expect(svg?.querySelector('rect')).not.toBeNull()

    // Top 6 bars only (sorted by value desc, categorical input has 8 items)
    const rects = Array.from(svg?.querySelectorAll('rect') ?? [])
    // At least 6 fill rects present (each bar has track + fill = 2 rects per row)
    expect(rects.length).toBeGreaterThanOrEqual(6)
  })

  it('renders time-series x-axis labels from point labels', () => {
    const chart = renderChart({
      kind: 'time-series',
      title: 'Knowledge Growth',
      summary: 'Growth over time.',
      series: hiveOverviewFixture.knowledgeGrowth
    })

    const svg = chart.querySelector('svg')
    const texts = Array.from(svg?.querySelectorAll('text') ?? []).map((t) => t.textContent)
    // At least one x-axis label from the data should appear
    expect(texts.some((t) => hiveOverviewFixture.knowledgeGrowth.points.some((p) => t?.includes(p.label)))).toBe(true)
  })

  it('renders categorical bar labels from point labels', () => {
    const points = [
      { label: 'alpha', value: 100 },
      { label: 'beta', value: 80 }
    ]
    const chart = renderChart({
      kind: 'categorical',
      title: 'Projects',
      summary: 'Most active.',
      points
    })

    const svg = chart.querySelector('svg')
    const texts = Array.from(svg?.querySelectorAll('text') ?? []).map((t) => t.textContent)
    expect(texts.some((t) => t?.includes('alpha'))).toBe(true)
    expect(texts.some((t) => t?.includes('beta'))).toBe(true)
  })

  it('renders an accessible empty state without SVG when data is empty', () => {
    const chart = renderChart({
      kind: 'categorical',
      title: 'Empty categories',
      summary: 'No category values are available.',
      points: []
    })

    expect(chart.getAttribute('role')).toBe('figure')
    expect(chart.querySelector('svg')).toBeNull()
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

  it('renders bar chart sorted descending by value (top 6)', () => {
    const points = [
      { label: 'low', value: 1 },
      { label: 'high', value: 100 },
      { label: 'mid', value: 50 }
    ]
    const chart = renderChart({
      kind: 'categorical',
      title: 'Active projects',
      summary: 'Project rankings.',
      points
    })

    const svg = chart.querySelector('svg')
    const texts = Array.from(svg?.querySelectorAll('text') ?? []).map((t) => t.textContent ?? '')
    // First label text should be the highest value item
    const labelTexts = texts.filter((t) => ['low', 'high', 'mid'].includes(t))
    expect(labelTexts[0]).toBe('high')
  })
})
