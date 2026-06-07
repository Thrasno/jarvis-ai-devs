import { describe, expect, it } from 'vitest'
import { renderAuditSync } from './AuditSync'

describe('audit and sync view', () => {
  it('renders sync audit events without daemon management copy', () => {
    const view = renderAuditSync({ status: 'ready', data: { audit_logs: [{ id: 'audit-1', occurred_at: '2026-06-06T20:00:00Z', action: 'sync_push', outcome: 'success', entry_count: 3, metadata: { pushed_count: 3 } }], total: 1, limit: 10, offset: 0 } })

    expect(view.textContent).toContain('sync_push')
    expect(view.textContent).toContain('success · 3 entries')
    expect(view.textContent?.toLowerCase()).not.toMatch(/daemon|start|stop|configure/)
  })

  it('renders empty and error states', () => {
    expect(renderAuditSync({ status: 'ready', data: { audit_logs: [], total: 0, limit: 10, offset: 0 } }).textContent).toContain('No sync audit events found')
    expect(renderAuditSync({ status: 'error', message: 'audit unavailable' }).textContent).toContain('audit unavailable')
  })
})
