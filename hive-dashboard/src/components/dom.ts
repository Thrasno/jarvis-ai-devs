const STATUS_CONTRACTS: Record<string, { token: string; label: string }> = {
  active: { token: 'active', label: 'Active' },
  degraded: { token: 'warning', label: 'Degraded' },
  error: { token: 'error', label: 'Error' },
  healthy: { token: 'healthy', label: 'Healthy' },
  inactive: { token: 'inactive', label: 'Inactive' },
  ok: { token: 'healthy', label: 'Healthy' },
  pending: { token: 'pending', label: 'Pending' },
  warning: { token: 'warning', label: 'Warning' }
}

let primitiveId = 0

export function panel(title: string, children: HTMLElement[] = []): HTMLElement {
  const section = document.createElement('section')
  const titleId = `dashboard-panel-${++primitiveId}`
  section.className = 'dashboard-panel panel'
  section.dataset.dashboardPrimitive = 'panel'
  section.setAttribute('role', 'region')
  section.setAttribute('aria-labelledby', titleId)
  const h2 = document.createElement('h2')
  h2.id = titleId
  h2.textContent = title
  section.append(h2, ...children)
  return section
}

export function stack(children: HTMLElement[]): HTMLElement {
  const node = document.createElement('div')
  node.className = 'dashboard-stack stack'
  node.dataset.dashboardPrimitive = 'stack'
  node.append(...children)
  return node
}

export function grid(children: HTMLElement[]): HTMLElement {
  const node = document.createElement('div')
  node.className = 'dashboard-grid grid'
  node.dataset.dashboardPrimitive = 'grid'
  node.append(...children)
  return node
}

export function control(label: string, options: { disabled?: boolean } = {}): HTMLButtonElement {
  const button = document.createElement('button')
  button.type = 'button'
  button.className = 'dashboard-control control'
  button.dataset.dashboardPrimitive = 'control'
  button.textContent = label
  if (options.disabled) {
    button.disabled = true
    button.setAttribute('aria-disabled', 'true')
  }
  return button
}

export function metricCard(input: { label: string; value: string; detail?: string }): HTMLElement {
  const metric = document.createElement('article')
  metric.className = 'dashboard-metric metric'
  metric.dataset.dashboardPrimitive = 'metric'
  metric.setAttribute('role', 'group')
  metric.setAttribute('aria-label', metricAriaLabel(input))
  metric.append(text(input.label, 'dashboard-metric-label'), text(input.value, 'dashboard-metric-value'))
  if (input.detail) metric.append(text(input.detail, 'dashboard-metric-detail'))
  return metric
}

function metricAriaLabel(input: { label: string; value: string; detail?: string }): string {
  return input.detail ? `${input.label}: ${input.value}, ${input.detail}` : `${input.label}: ${input.value}`
}

export function emptyState(message: string): HTMLElement {
  const state = text(message, 'dashboard-state state')
  state.dataset.dashboardPrimitive = 'state'
  state.dataset.state = 'empty'
  state.setAttribute('role', 'status')
  return state
}

function statusContract(status: string): { token: string; label: string } {
  return STATUS_CONTRACTS[status.toLowerCase()] ?? { token: 'neutral', label: 'Unknown' }
}

function statusAriaLabel(status: string): string {
  const contract = statusContract(status)
  return `${contract.label === 'Unknown' ? 'Neutral' : contract.label} status: ${status}`
}

export function statusBadge(status: string): HTMLElement {
  const contract = statusContract(status)
  const badge = document.createElement('span')
  badge.className = 'dashboard-status status'
  badge.dataset.dashboardPrimitive = 'status'
  badge.dataset.dashboardStatus = contract.token
  badge.setAttribute('aria-label', statusAriaLabel(status))
  badge.textContent = contract.label
  return badge
}

export function statusDot(status: string, options: { decorative?: boolean } = {}): HTMLElement {
  const contract = statusContract(status)
  const dot = document.createElement('span')
  dot.className = 'dashboard-status-dot'
  dot.dataset.dashboardPrimitive = 'status-dot'
  dot.dataset.dashboardStatus = contract.token
  dot.title = statusAriaLabel(status)
  if (options.decorative) {
    dot.setAttribute('aria-hidden', 'true')
  } else {
    dot.setAttribute('role', 'img')
    dot.setAttribute('aria-label', statusAriaLabel(status))
  }
  return dot
}

export function statusLabel(status: string): string {
  return statusContract(status).label
}

export function text(value: string, className?: string): HTMLParagraphElement {
  const p = document.createElement('p')
  if (className) p.className = className
  p.textContent = value
  return p
}

export function list(items: readonly string[], empty = ''): HTMLElement {
  if (items.length === 0) return emptyState(empty)
  const ul = document.createElement('ul')
  for (const item of items) {
    const li = document.createElement('li')
    li.textContent = item
    ul.append(li)
  }
  return ul
}

export function append<T extends HTMLElement>(root: T, ...children: HTMLElement[]): T {
  root.append(...children)
  return root
}

export function error<T extends HTMLElement>(root: T, message: string): T {
  root.setAttribute('role', 'alert')
  root.dataset.state = 'error'
  root.append(text(message, 'dashboard-state state'))
  return root
}
