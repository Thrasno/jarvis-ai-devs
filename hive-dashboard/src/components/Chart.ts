import type { ChartPointViewModel, ChartSeriesViewModel } from '../domain/dashboard'
import { emptyState } from './dom'

export type ChartKind = 'time-series' | 'categorical'

export type ChartInput =
  | {
      readonly kind: 'time-series'
      readonly title: string
      readonly series: ChartSeriesViewModel
    }
  | {
      readonly kind: 'categorical'
      readonly title: string
      readonly points: readonly ChartPointViewModel[]
    }

const SVG_NS = 'http://www.w3.org/2000/svg'

// Chart layout constants
const DEFAULT_CHART_WIDTH = 600
const MIN_CHART_HEIGHT = 180
const MAX_CHART_HEIGHT = 280
const CHART_ASPECT_HEIGHT = 0.4
const PAD_L = 42
const PAD_B = 22
const PAD_T = 12
const PAD_R = 12
const VIEW_W = 600

interface TimeSeriesChartPoint {
  readonly label: string
  readonly value: number
  readonly x: number
  readonly y: number
}

export interface TimeSeriesChartModel {
  readonly width: number
  readonly height: number
  readonly plotBottom: number
  readonly plotRight: number
  readonly yMax: number
  readonly yTicks: readonly number[]
  readonly points: readonly TimeSeriesChartPoint[]
}

export function renderChart(input: ChartInput): HTMLElement {
  if (!isSupportedChartInput(input)) return fallbackChart()

  const chart = chartRoot(input.title, input.kind)

  if (input.kind === 'time-series') {
    const points = input.series.points
    if (points.length === 0) {
      chart.append(emptyState(`No chart data is available for ${input.title}.`))
      return chart
    }
    chart.append(renderLineChart(input.title, points, DEFAULT_CHART_WIDTH))
    observeTimeSeriesChart(chart, input.title, points)
  } else {
    const points = input.points
    if (points.length === 0) {
      chart.append(emptyState(`No chart data is available for ${input.title}.`))
      return chart
    }
    chart.append(renderBarChart(points))
  }

  return chart
}

function isSupportedChartInput(input: unknown): input is ChartInput {
  if (!input || typeof input !== 'object') return false
  const candidate = input as Record<string, unknown>

  if (candidate.kind !== 'time-series' && candidate.kind !== 'categorical') return false
  if (typeof candidate.title !== 'string') return false

  if (candidate.kind === 'time-series') {
    const series = candidate.series as Partial<ChartSeriesViewModel> | undefined
    return Boolean(series && Array.isArray(series.points))
  }

  return Array.isArray(candidate.points)
}

function chartRoot(label: string, kind: ChartKind | 'unsupported'): HTMLElement {
  const chart = document.createElement('section')
  chart.className = 'chart'
  chart.dataset.chartKind = kind
  chart.setAttribute('role', 'figure')
  chart.setAttribute('aria-label', label)
  return chart
}

export function createTimeSeriesChartModel(width: number, points: readonly ChartPointViewModel[]): TimeSeriesChartModel {
  const safeWidth = Math.max(1, Math.round(width))
  const height = Math.round(Math.min(MAX_CHART_HEIGHT, Math.max(MIN_CHART_HEIGHT, safeWidth * CHART_ASPECT_HEIGHT)))
  const n = points.length
  const innerW = Math.max(1, safeWidth - PAD_L - PAD_R)
  const innerH = height - PAD_B - PAD_T
  const maxValue = Math.max(...points.map((point) => point.value), 0)
  const yMax = niceCumulativeCeiling(maxValue)

  function xOf(index: number): number {
    return n === 1 ? PAD_L + innerW / 2 : PAD_L + (index / (n - 1)) * innerW
  }
  function yOf(value: number): number {
    return PAD_T + innerH - (Math.max(0, value) / yMax) * innerH
  }

  return {
    width: safeWidth,
    height,
    plotBottom: PAD_T + innerH,
    plotRight: PAD_L + innerW,
    yMax,
    yTicks: [yMax, yMax / 2, 0],
    points: points.map((point, index) => ({ ...point, x: xOf(index), y: yOf(point.value) }))
  }
}

function niceCumulativeCeiling(value: number): number {
  if (value <= 1) return 1
  const magnitude = 10 ** Math.floor(Math.log10(value))
  const normalized = value / magnitude
  const multiplier = normalized <= 1 ? 1 : normalized <= 2 ? 2 : normalized <= 5 ? 5 : 10
  return multiplier * magnitude
}

function observeTimeSeriesChart(chart: HTMLElement, title: string, points: readonly ChartPointViewModel[]): void {
  if (typeof ResizeObserver === 'undefined') return
  let renderedWidth = DEFAULT_CHART_WIDTH
  const observer = new ResizeObserver((entries) => {
    if (!chart.isConnected) {
      observer.disconnect()
      return
    }
    const width = Math.round(entries[0]?.contentRect.width ?? 0)
    if (width <= 0 || width === renderedWidth) return
    renderedWidth = width
    chart.querySelector('svg')?.replaceWith(renderLineChart(title, points, width))
  })
  observer.observe(chart)
}

function renderLineChart(title: string, points: readonly ChartPointViewModel[], width: number): SVGSVGElement {
  const model = createTimeSeriesChartModel(width, points)

  const svg = document.createElementNS(SVG_NS, 'svg')
  svg.setAttribute('viewBox', `0 0 ${model.width} ${model.height}`)
  svg.setAttribute('width', '100%')
  svg.setAttribute('height', String(model.height))
  svg.setAttribute('preserveAspectRatio', 'xMidYMid meet')
  svg.setAttribute('role', 'img')
  svg.setAttribute('aria-label', `${title} chart`)

  // Defs — vertical area gradient
  const defs = document.createElementNS(SVG_NS, 'defs')
  const gradId = `chart-area-gradient-${Math.random().toString(36).slice(2, 7)}`
  const grad = document.createElementNS(SVG_NS, 'linearGradient')
  grad.setAttribute('id', gradId)
  grad.setAttribute('x1', '0')
  grad.setAttribute('y1', '0')
  grad.setAttribute('x2', '0')
  grad.setAttribute('y2', '1')
  const stop0 = document.createElementNS(SVG_NS, 'stop')
  stop0.setAttribute('offset', '0%')
  stop0.setAttribute('stop-color', '#3B82E8')
  stop0.setAttribute('stop-opacity', '0.3')
  const stop60 = document.createElementNS(SVG_NS, 'stop')
  stop60.setAttribute('offset', '60%')
  stop60.setAttribute('stop-color', '#3B82E8')
  stop60.setAttribute('stop-opacity', '0.08')
  const stop100 = document.createElementNS(SVG_NS, 'stop')
  stop100.setAttribute('offset', '100%')
  stop100.setAttribute('stop-color', '#3B82E8')
  stop100.setAttribute('stop-opacity', '0')
  grad.append(stop0, stop60, stop100)
  defs.append(grad)
  svg.append(defs)

  // Three stable, zero-based cumulative ticks.
  const gridYs = [PAD_T, PAD_T + (model.plotBottom - PAD_T) / 2, model.plotBottom]
  for (let gi = 0; gi < 3; gi++) {
    const gLine = document.createElementNS(SVG_NS, 'line')
    gLine.setAttribute('x1', String(PAD_L))
    gLine.setAttribute('y1', String(gridYs[gi]))
    gLine.setAttribute('x2', String(model.plotRight))
    gLine.setAttribute('y2', String(gridYs[gi]))
    gLine.setAttribute('stroke', 'rgba(255,255,255,0.05)')
    gLine.setAttribute('stroke-width', '1')
    svg.append(gLine)

    const gText = document.createElementNS(SVG_NS, 'text')
    gText.setAttribute('x', String(PAD_L - 4))
    gText.setAttribute('y', String(gridYs[gi] + 4))
    gText.setAttribute('text-anchor', 'end')
    gText.setAttribute('fill', '#5A6472')
    gText.setAttribute('font-size', '9')
    gText.setAttribute('font-family', 'monospace')
    gText.textContent = String(model.yTicks[gi])
    svg.append(gText)
  }

  // Build point coordinates
  const linePts = model.points.map((point) => `${point.x},${point.y}`).join(' ')

  // Area polygon (filled with gradient)
  const area = document.createElementNS(SVG_NS, 'polygon')
  area.setAttribute('points', `${PAD_L},${model.plotBottom} ${linePts} ${model.plotRight},${model.plotBottom}`)
  area.setAttribute('fill', `url(#${gradId})`)
  svg.append(area)

  // Line polyline
  const line = document.createElementNS(SVG_NS, 'polyline')
  line.setAttribute('points', linePts)
  line.setAttribute('fill', 'none')
  line.setAttribute('stroke', '#3B82E8')
  line.setAttribute('stroke-width', '1.6')
  line.setAttribute('stroke-linejoin', 'round')
  line.setAttribute('stroke-linecap', 'round')
  svg.append(line)

  for (const point of model.points) {
    const marker = document.createElementNS(SVG_NS, 'circle')
    marker.dataset.chartPoint = ''
    marker.dataset.label = point.label
    marker.dataset.value = String(point.value)
    marker.setAttribute('cx', String(point.x))
    marker.setAttribute('cy', String(point.y))
    marker.setAttribute('r', '2.5')
    marker.setAttribute('fill', '#3B82E8')
    const pointTitle = document.createElementNS(SVG_NS, 'title')
    pointTitle.textContent = `${point.label}: ${point.value}`
    marker.append(pointTitle)
    svg.append(marker)
  }

  // X-axis labels every ~6 points
  const labelStep = Math.max(1, Math.floor(points.length / 6))
  for (let i = 0; i < points.length; i += labelStep) {
    const xLabel = document.createElementNS(SVG_NS, 'text')
    xLabel.setAttribute('x', String(model.points[i].x))
    xLabel.setAttribute('y', String(model.height - 4))
    xLabel.setAttribute('text-anchor', 'middle')
    xLabel.setAttribute('fill', '#5A6472')
    xLabel.setAttribute('font-size', '9')
    xLabel.setAttribute('font-family', 'monospace')
    xLabel.textContent = points[i].label
    svg.append(xLabel)
  }

  return svg
}

function tint(hex: string, alpha: number): string {
  const r = parseInt(hex.slice(1, 3), 16)
  const g = parseInt(hex.slice(3, 5), 16)
  const b = parseInt(hex.slice(5, 7), 16)
  return `rgba(${r},${g},${b},${alpha})`
}

function renderBarChart(rawPoints: readonly ChartPointViewModel[]): SVGSVGElement {
  // Sort descending, take top 6
  const points = [...rawPoints].sort((a, b) => b.value - a.value).slice(0, 6)
  const maxV = Math.max(...points.map((p) => p.value), 1)

  const rowH = 24
  const rowGap = 6
  const labelW = 96
  const valueW = 56
  const trackX = labelW + 8
  const trackW = VIEW_W - labelW - valueW - 16
  const totalH = points.length * (rowH + rowGap) + 4

  const svg = document.createElementNS(SVG_NS, 'svg')
  svg.setAttribute('viewBox', `0 0 ${VIEW_W} ${totalH}`)
  svg.setAttribute('width', '100%')
  svg.setAttribute('height', String(totalH))
  svg.setAttribute('aria-hidden', 'true')

  const defs = document.createElementNS(SVG_NS, 'defs')

  for (let i = 0; i < points.length; i++) {
    const p = points[i]
    const color = '#3B82E8'
    const y = i * (rowH + rowGap)
    const barW = Math.max(3, (p.value / maxV) * trackW)

    // Gradient for this bar
    const gradId = `bar-grad-${i}-${Math.random().toString(36).slice(2, 7)}`
    const grad = document.createElementNS(SVG_NS, 'linearGradient')
    grad.setAttribute('id', gradId)
    grad.setAttribute('x1', '0')
    grad.setAttribute('y1', '0')
    grad.setAttribute('x2', '1')
    grad.setAttribute('y2', '0')
    const stop0 = document.createElementNS(SVG_NS, 'stop')
    stop0.setAttribute('offset', '0%')
    stop0.setAttribute('stop-color', tint(color, 0.25))
    const stop100 = document.createElementNS(SVG_NS, 'stop')
    stop100.setAttribute('offset', '100%')
    stop100.setAttribute('stop-color', tint(color, 0.55))
    grad.append(stop0, stop100)
    defs.append(grad)

    // Label text
    const label = document.createElementNS(SVG_NS, 'text')
    label.setAttribute('x', String(labelW - 4))
    label.setAttribute('y', String(y + rowH / 2 + 4))
    label.setAttribute('text-anchor', 'end')
    label.setAttribute('fill', '#8B97A8')
    label.setAttribute('font-size', '11.5')
    label.setAttribute('font-family', 'monospace')
    label.textContent = p.label
    svg.append(label)

    // Track rect (background)
    const track = document.createElementNS(SVG_NS, 'rect')
    track.setAttribute('x', String(trackX))
    track.setAttribute('y', String(y))
    track.setAttribute('width', String(trackW))
    track.setAttribute('height', String(rowH))
    track.setAttribute('fill', 'rgba(255,255,255,0.03)')
    track.setAttribute('rx', '3')
    svg.append(track)

    // Fill rect
    const fill = document.createElementNS(SVG_NS, 'rect')
    fill.setAttribute('x', String(trackX))
    fill.setAttribute('y', String(y))
    fill.setAttribute('width', String(barW))
    fill.setAttribute('height', String(rowH))
    fill.setAttribute('fill', `url(#${gradId})`)
    fill.setAttribute('rx', '3')
    svg.append(fill)

    // Value text
    const valueText = document.createElementNS(SVG_NS, 'text')
    valueText.setAttribute('x', String(trackX + trackW + 6))
    valueText.setAttribute('y', String(y + rowH / 2 + 4))
    valueText.setAttribute('fill', '#E6EDF3')
    valueText.setAttribute('font-size', '12')
    valueText.setAttribute('font-family', 'monospace')
    valueText.setAttribute('font-variant-numeric', 'tabular-nums')
    valueText.textContent = String(p.value)
    svg.append(valueText)
  }

  svg.prepend(defs)
  return svg
}

function fallbackChart(): HTMLElement {
  const chart = chartRoot('Unsupported chart', 'unsupported')
  chart.append(emptyState('Unsupported chart data. Chart cannot be rendered.'))
  return chart
}
