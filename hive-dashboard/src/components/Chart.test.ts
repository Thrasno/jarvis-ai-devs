import { beforeEach, describe, expect, it, vi } from 'vitest'
import { hiveOverviewFixture } from '../fixtures/hive-dashboard/overview'
import { insightsScreenFixtures } from '../fixtures/hive-dashboard/insights'
import { createTimeSeriesChartModel, renderChart, type ChartInput } from './Chart'

class ResizeObserverMock {
  static instances: ResizeObserverMock[] = []

  readonly disconnect = vi.fn()
  readonly observe = vi.fn()

  constructor(private readonly callback: ResizeObserverCallback) {
    ResizeObserverMock.instances.push(this)
  }

  resize(target: Element, width: number): void {
    this.callback([
      {
        target,
        contentRect: { width } as DOMRectReadOnly
      } as ResizeObserverEntry
    ], this as unknown as ResizeObserver)
  }
}

describe('dashboard chart foundation', () => {
  beforeEach(() => {
    ResizeObserverMock.instances = []
    vi.stubGlobal('ResizeObserver', ResizeObserverMock)
  })

  it('renders time-series as SVG with accessible wrapper and SVG elements', () => {
    const chart = renderChart({
      kind: 'time-series',
      title: 'Knowledge Growth',
      series: hiveOverviewFixture.knowledgeGrowth
    })

    expect(chart.getAttribute('role')).toBe('figure')
    expect(chart.getAttribute('data-chart-kind')).toBe('time-series')
    expect(chart.getAttribute('aria-label')).toBe('Knowledge Growth')

    // Chart content must NOT render an inner heading — the panel owns the title
    expect(chart.querySelector('h2')).toBeNull()
    // Chart content must NOT render a summary paragraph — removed per polish spec
    expect(chart.querySelector('.chart-summary')).toBeNull()

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
      points: insightsScreenFixtures.analytics.memoriesByCategory
    })

    expect(chart.getAttribute('data-chart-kind')).toBe('categorical')
    // No summary paragraph — removed per polish spec
    expect(chart.querySelector('.chart-summary')).toBeNull()

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
      series: hiveOverviewFixture.knowledgeGrowth
    })

    const svg = chart.querySelector('svg')
    const texts = Array.from(svg?.querySelectorAll('text') ?? []).map((t) => t.textContent)
    // At least one x-axis label from the data should appear
    expect(texts.some((t) => hiveOverviewFixture.knowledgeGrowth.points.some((p) => t?.includes(p.label)))).toBe(true)
  })

  it('computes proportional chart geometry with bounded mobile and desktop heights', () => {
    const points = [{ label: 'Jul 1', value: 40 }, { label: 'Jul 2', value: 80 }]

    expect(createTimeSeriesChartModel(280, points).height).toBe(180)
    expect(createTimeSeriesChartModel(600, points).height).toBe(240)
    expect(createTimeSeriesChartModel(900, points).height).toBe(280)
    expect(createTimeSeriesChartModel(900, points).width).toBe(900)
  })

  it('uses deterministic zero-based cumulative scale thresholds and truthful ticks', () => {
    const belowThreshold = createTimeSeriesChartModel(600, [
      { label: 'Jul 1', value: 40 },
      { label: 'Jul 2', value: 99 }
    ])
    const aboveThreshold = createTimeSeriesChartModel(600, [
      { label: 'Jul 1', value: 40 },
      { label: 'Jul 2', value: 101 }
    ])

    expect(belowThreshold.yMax).toBe(100)
    expect(belowThreshold.yTicks).toEqual([100, 50, 0])
    expect(aboveThreshold.yMax).toBe(200)
    expect(aboveThreshold.yTicks).toEqual([200, 100, 0])
  })

  it('keeps historical cumulative coordinates stable while growth remains within the scale threshold', () => {
    const history = [{ label: 'Jul 1', value: 40 }, { label: 'Jul 2', value: 60 }]
    const initial = createTimeSeriesChartModel(600, history)
    const grown = createTimeSeriesChartModel(600, [...history, { label: 'Jul 3', value: 90 }])

    expect(initial.yMax).toBe(grown.yMax)
    expect(grown.points.slice(0, 2).map((point) => point.y)).toEqual(initial.points.map((point) => point.y))
  })

  it('recomputes SVG geometry from ResizeObserver measurements and retains source point values', () => {
    const points = [{ label: 'Jul 1', value: 40 }, { label: 'Jul 2', value: 80 }]
    const chart = renderChart({
      kind: 'time-series',
      title: 'Knowledge Growth',
      series: { label: 'Memories', points }
    })
    document.body.append(chart)

    const observer = ResizeObserverMock.instances[0]
    expect(observer.observe).toHaveBeenCalledWith(chart)

    observer.resize(chart, 900)

    const svg = chart.querySelector('svg')
    expect(svg?.getAttribute('viewBox')).toBe('0 0 900 280')
    expect(svg?.getAttribute('height')).toBe('280')
    expect(svg?.getAttribute('preserveAspectRatio')).not.toBe('none')
    expect(svg?.getAttribute('aria-label')).toBe('Knowledge Growth chart')
    expect(Array.from(svg?.querySelectorAll('[data-chart-point]') ?? []).map((point) => ({
      label: point.getAttribute('data-label'),
      value: point.getAttribute('data-value')
    }))).toEqual([
      { label: 'Jul 1', value: '40' },
      { label: 'Jul 2', value: '80' }
    ])
  })

  it('disconnects its resize observer when notified after chart removal', () => {
    const chart = renderChart({
      kind: 'time-series',
      title: 'Knowledge Growth',
      series: { label: 'Memories', points: [{ label: 'Jul 1', value: 40 }] }
    })
    document.body.append(chart)
    const observer = ResizeObserverMock.instances[0]

    chart.remove()
    observer.resize(chart, 600)

    expect(observer.disconnect).toHaveBeenCalledOnce()
  })

  it('renders categorical bar labels from point labels', () => {
    const points = [
      { label: 'alpha', value: 100 },
      { label: 'beta', value: 80 }
    ]
    const chart = renderChart({
      kind: 'categorical',
      title: 'Projects',
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
      points
    })

    const svg = chart.querySelector('svg')
    const texts = Array.from(svg?.querySelectorAll('text') ?? []).map((t) => t.textContent ?? '')
    // First label text should be the highest value item
    const labelTexts = texts.filter((t) => ['low', 'high', 'mid'].includes(t))
    expect(labelTexts[0]).toBe('high')
  })
})
