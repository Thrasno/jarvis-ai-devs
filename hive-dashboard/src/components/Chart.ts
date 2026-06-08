import type { ChartPointViewModel, ChartSeriesViewModel } from '../domain/dashboard'
import { emptyState } from './dom'

export type ChartKind = 'time-series' | 'categorical'

export type ChartInput =
  | {
      readonly kind: 'time-series'
      readonly title: string
      readonly summary: string
      readonly series: ChartSeriesViewModel
    }
  | {
      readonly kind: 'categorical'
      readonly title: string
      readonly summary: string
      readonly points: readonly ChartPointViewModel[]
    }

export function renderChart(input: ChartInput): HTMLElement {
  if (!isSupportedChartInput(input)) return fallbackChart()

  const points = chartPoints(input)
  const chart = chartRoot(input.title, input.kind)
  chart.append(chartTitle(input.title), chartSummary(input.summary))

  if (points.length === 0) {
    chart.append(emptyState(`No chart data is available for ${input.title}.`))
    return chart
  }

  chart.append(pointList(input, points))
  return chart
}

function isSupportedChartInput(input: unknown): input is ChartInput {
  if (!input || typeof input !== 'object') return false
  const candidate = input as Record<string, unknown>

  if (candidate.kind !== 'time-series' && candidate.kind !== 'categorical') return false
  if (typeof candidate.title !== 'string' || typeof candidate.summary !== 'string') return false

  if (candidate.kind === 'time-series') {
    const series = candidate.series as Partial<ChartSeriesViewModel> | undefined
    return Boolean(series && Array.isArray(series.points))
  }

  return Array.isArray(candidate.points)
}

function chartPoints(input: ChartInput): readonly ChartPointViewModel[] {
  return input.kind === 'time-series' ? input.series.points : input.points
}

function chartRoot(label: string, kind: ChartKind | 'unsupported'): HTMLElement {
  const chart = document.createElement('section')
  chart.className = 'chart'
  chart.dataset.chartKind = kind
  chart.setAttribute('role', 'figure')
  chart.setAttribute('aria-label', label)
  return chart
}

function chartTitle(title: string): HTMLHeadingElement {
  const heading = document.createElement('h2')
  heading.textContent = title
  return heading
}

function chartSummary(summary: string): HTMLParagraphElement {
  const text = document.createElement('p')
  text.className = 'chart-summary'
  text.textContent = summary
  return text
}

function pointList(input: ChartInput, points: readonly ChartPointViewModel[]): HTMLOListElement {
  const list = document.createElement('ol')
  list.className = 'chart-points'

  for (const point of points) {
    list.append(pointMark(input, point))
  }

  return list
}

function pointMark(input: ChartInput, point: ChartPointViewModel): HTMLLIElement {
  const mark = document.createElement('li')
  mark.className = input.kind === 'time-series' ? 'chart-point' : 'chart-point chart-category'
  mark.dataset.chartPoint = point.label
  mark.setAttribute('aria-label', pointAriaLabel(input, point))

  if (input.kind === 'categorical') mark.dataset.chartCategory = point.label

  const label = document.createElement('span')
  label.className = 'chart-point-label'
  label.textContent = point.label

  const value = document.createElement('span')
  value.className = 'chart-point-value'
  value.textContent = String(point.value)

  mark.append(label, value)
  return mark
}

function pointAriaLabel(input: ChartInput, point: ChartPointViewModel): string {
  const subject = input.kind === 'time-series' ? 'point' : 'category'
  return `${input.title} ${subject} ${point.label}: ${point.value}`
}

function fallbackChart(): HTMLElement {
  const chart = chartRoot('Unsupported chart', 'unsupported')
  chart.append(emptyState('Unsupported chart data. Chart cannot be rendered.'))
  return chart
}
