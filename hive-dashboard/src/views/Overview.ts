import type { AdminStats, Health } from '../api/client'
import { append, error, grid, list, metricCard, panel, stack, statusBadge, text } from '../components/dom'

export type ViewState<T> = { status: 'loading' } | { status: 'error'; message: string } | { status: 'ready'; data: T }
export type OverviewData = { health: Health; stats: AdminStats }

export function renderOverview(state: ViewState<OverviewData>): HTMLElement {
  const card = panel('Overview')
  if (state.status === 'loading') return append(card, text('Loading dashboard data…'))
  if (state.status === 'error') return error(card, state.message)
  const { health, stats } = state.data
  return append(card,
    stack([
      append(text(`API status ${health.status}; database ${health.db}; version ${health.version}.`), statusBadge(health.status)),
      grid([
        metricCard({ label: 'Users', value: `${stats.users.total} users`, detail: `${stats.users.active} active` }),
        metricCard({ label: 'Memories', value: `${stats.memories.total} memories` })
      ])
    ]),
    list(stats.memories.by_project.map((item) => `${item.project ?? 'unknown'}: ${item.count}`))
  )
}
