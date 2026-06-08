import type { AuditLogList } from '../api/client'
import { append, error, list, panel, text } from '../components/dom'
import type { ViewState } from './Overview'

export function renderAuditSync(state: ViewState<AuditLogList>): HTMLElement {
  const card = panel('Audit and sync')
  if (state.status === 'loading') return append(card, text('Loading audit activity…'))
  if (state.status === 'error') return error(card, state.message)
  return append(card, list(state.data.audit_logs.map((entry) => `${entry.action} — ${entry.outcome} · ${entry.entry_count} entries`), 'No sync audit events found'))
}
