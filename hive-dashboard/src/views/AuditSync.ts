import type { SyncAttemptSummary, SyncAttemptSummaryWindow } from '../api/client'
import { append, emptyState, error, list, panel, stack, text } from '../components/dom'
import type { ViewState } from './Overview'

export function renderAuditSync(state: ViewState<SyncAttemptSummary>): HTMLElement {
  const card = panel('Sync attempt audit reliability')
  if (state.status === 'loading') return append(card, text('Loading sync attempt audit summary…'))
  if (state.status === 'error') return error(card, state.message)

  if (state.data.windows.every((window) => window.total === 0)) {
    return append(card, emptyState('No sync attempt history has been recorded for these audit windows.'))
  }

  return append(
    card,
    text('Audit-derived reliability summary from persisted sync attempts.'),
    stack(state.data.windows.map(renderWindowSummary))
  )
}

function renderWindowSummary(window: SyncAttemptSummaryWindow): HTMLElement {
  const rows = [
    `${window.window} audit window — ${window.total} attempts · ${window.successes} successes · ${window.failures} ${pluralize('failure', window.failures)} · ${formatPercent(window.failure_rate)} failure rate`,
    ...(window.last_success_at ? [`Last success: ${window.last_success_at}`] : []),
    ...(window.last_failure_at ? [`Last failure: ${window.last_failure_at}`] : []),
    ...(window.top_errors.length > 0 ? [`Top error: ${window.top_errors[0].key} (${window.top_errors[0].count})`] : []),
    ...dimensionRows('Developer', window.by_developer),
    ...dimensionRows('Project', window.by_project),
    ...dimensionRows('Client', window.by_client),
    ...dimensionRows('Daemon', window.by_daemon)
  ]
  return list(rows)
}

function dimensionRows(label: string, counts: readonly { key: string; count: number }[]): string[] {
  return counts.slice(0, 3).map((count) => `${label}: ${count.key} (${count.count})`)
}

function formatPercent(rate: number): string {
  return `${(rate * 100).toFixed(1)}%`
}

function pluralize(word: string, count: number): string {
  return count === 1 ? word : `${word}s`
}
