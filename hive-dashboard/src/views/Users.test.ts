import { describe, expect, it } from 'vitest'
import { renderUsers } from './Users'

const adminUser = { id: 'user-1', username: 'admin', email: 'admin@example.com', level: 'admin' as const, is_active: true, created_at: '2026-06-06T20:00:00Z' }

describe('users view', () => {
  it('renders admin users from /admin/users', () => {
    const view = renderUsers({ status: 'ready', data: { users: [adminUser, { ...adminUser, id: 'user-2', username: 'viewer', email: 'viewer@example.com', level: 'viewer', is_active: false }] } })

    expect(view.textContent).toContain('admin@example.com')
    expect(view.textContent).toContain('admin · active')
    expect(view.textContent).toContain('viewer · inactive')
  })

  it('renders an empty state', () => {
    const view = renderUsers({ status: 'ready', data: { users: [] } })

    expect(view.textContent).toContain('No users found')
  })
})
