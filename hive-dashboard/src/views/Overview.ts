import type { AdminStats, Health } from '../api/client'

export type ViewState<T> = { status: 'loading' } | { status: 'error'; message: string } | { status: 'ready'; data: T }
export type OverviewData = { health: Health; stats: AdminStats }

export function renderOverview(state: ViewState<OverviewData>): HTMLElement {
  const card = panel('Overview')
  if (state.status === 'loading') return append(card, text('Loading dashboard data…'))
  if (state.status === 'error') return error(card, state.message)
  const { health, stats } = state.data
  return append(card,
    text(`API status ${health.status}; database ${health.db}; version ${health.version}.`),
    text(`${stats.users.total} users; ${stats.users.active} active.`),
    text(`${stats.memories.total} memories.`),
    list(stats.memories.by_project.map((item) => `${item.project ?? 'unknown'}: ${item.count}`))
  )
}

export function panel(title: string): HTMLElement {
  const section = document.createElement('section')
  section.className = 'card view'
  const h2 = document.createElement('h2')
  h2.textContent = title
  section.append(h2)
  return section
}

export function text(value: string): HTMLParagraphElement {
  const p = document.createElement('p')
  p.textContent = value
  return p
}

export function list(items: string[], empty = ''): HTMLElement {
  if (items.length === 0) return text(empty)
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
  root.append(text(message))
  return root
}
