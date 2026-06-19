import { describe, expect, it, vi } from 'vitest'
import type { UserLevel } from '../api/client'
import { renderUsers } from './Users'

const adminUser = { id: 'user-1', username: 'admin', email: 'admin@example.com', level: 'admin' as const, is_active: true, created_at: '2026-06-06T20:00:00Z' }
const memberUser = { id: 'user-2', username: 'member', email: 'member@example.com', level: 'member' as const, is_active: true, created_at: '2026-06-06T20:00:00Z' }
const viewerUser = { id: 'user-3', username: 'viewer', email: 'viewer@example.com', level: 'viewer' as const, is_active: false, created_at: '2026-06-06T20:00:00Z' }

describe('users view', () => {
  it('renders admin users from /admin/users', () => {
    const view = renderUsers({ status: 'ready', data: { users: [adminUser, viewerUser] } })

    expect(view.textContent).toContain('admin')
    expect(view.textContent).toContain('admin@example.com')
    expect(view.textContent).toContain('Level: admin')
    expect(view.textContent).toContain('Admin seat: yes')
    expect(view.textContent).toContain('State: active')
    expect(view.textContent).toContain('viewer')
    expect(view.textContent).toContain('Level: viewer')
    expect(view.textContent).toContain('Admin seat: no')
    expect(view.textContent).toContain('State: inactive')
  })

  it('renders an empty state', () => {
    const view = renderUsers({ status: 'ready', data: { users: [] } })

    expect(view.textContent).toContain('No users found')
  })

  it('renders a failed list state without fake data', () => {
    const view = renderUsers({ status: 'error', message: 'users unavailable' })

    expect(view.getAttribute('role')).toBe('alert')
    expect(view.textContent).toContain('users unavailable')
    expect(view.textContent).not.toContain('admin@example.com')
  })

  it('does not render deferred filters, pagination, or reactivation controls', () => {
    const view = renderUsers({ status: 'ready', data: { users: [viewerUser] } })

    expect(view.textContent).not.toMatch(/filter|pagination|next page|previous page|reactivate/i)
    expect(actionButton(view, 'Reactivate viewer')).toBeNull()
  })

  it('does not expose management controls to non-admin sessions', () => {
    const view = renderUsers({ status: 'ready', data: { users: [memberUser] } }, { currentUsername: 'viewer', currentLevel: 'viewer' })

    expect(actionButton(view, 'Set member level to viewer')).toBeNull()
    expect(actionButton(view, 'Set member level to member')).toBeNull()
    expect(actionButton(view, 'Grant admin to member')).toBeNull()
    expect(actionButton(view, 'Deactivate member')).toBeNull()
  })

  it('renders role-level controls for supported non-admin level changes', () => {
    const view = renderUsers({ status: 'ready', data: { users: [adminUser, viewerUser] } }, { currentUsername: 'owner', currentLevel: 'admin', actions: userActions() })

    expect(actionButton(view, 'Set admin level to member')).not.toBeNull()
    expect(actionButton(view, 'Set admin level to viewer')).not.toBeNull()
    expect(actionButton(view, 'Set viewer level to member')).not.toBeNull()
  })

  it('requires confirmation before changing a user level and skips the mutation when cancelled', async () => {
    const actions = userActions()
    const view = renderUsers({ status: 'ready', data: { users: [memberUser] } }, { currentUsername: 'admin', currentLevel: 'admin', actions })

    actionButton(view, 'Set member level to viewer')!.click()

    expect(view.querySelector('[role="dialog"]')?.textContent).toContain('Change member level to viewer?')
    actionButton(view, 'Cancel')!.click()
    await Promise.resolve()

    expect(actions.onSetUserLevel).not.toHaveBeenCalled()
    expect(view.querySelector('[role="dialog"]')).toBeNull()
  })

  it('calls the injected level action after visible confirmation', async () => {
    const actions = userActions()
    const view = renderUsers({ status: 'ready', data: { users: [viewerUser] } }, { currentUsername: 'admin', currentLevel: 'admin', actions })

    actionButton(view, 'Set viewer level to member')!.click()
    actionButton(view, 'Confirm')!.click()
    await Promise.resolve()

    expect(actions.onSetUserLevel).toHaveBeenCalledWith('viewer', 'member')
  })

  it('calls the injected level action for admin to member changes', async () => {
    const actions = userActions()
    const view = renderUsers({ status: 'ready', data: { users: [adminUser] } }, { currentUsername: 'owner', currentLevel: 'admin', actions })

    actionButton(view, 'Set admin level to member')!.click()
    actionButton(view, 'Confirm')!.click()
    await Promise.resolve()

    expect(actions.onSetUserLevel).toHaveBeenCalledWith('admin', 'member')
  })

  it('requires confirmation before granting an admin seat', async () => {
    const actions = userActions()
    const view = renderUsers({ status: 'ready', data: { users: [memberUser] } }, { currentUsername: 'admin', currentLevel: 'admin', actions })

    actionButton(view, 'Grant admin to member')!.click()
    expect(view.querySelector('[role="dialog"]')?.textContent).toContain('Grant admin seat to member?')
    actionButton(view, 'Confirm')!.click()
    await Promise.resolve()

    expect(actions.onGrantAdmin).toHaveBeenCalledWith('member')
  })

  it('requires confirmation before deactivating a user', async () => {
    const actions = userActions()
    const view = renderUsers({ status: 'ready', data: { users: [memberUser] } }, { currentUsername: 'admin', currentLevel: 'admin', actions })

    actionButton(view, 'Deactivate member')!.click()
    expect(view.querySelector('[role="dialog"]')?.textContent).toContain('Deactivate member?')
    actionButton(view, 'Confirm')!.click()
    await Promise.resolve()

    expect(actions.onDeactivateUser).toHaveBeenCalledWith('member')
  })

  it('disables self management controls', () => {
    const view = renderUsers({ status: 'ready', data: { users: [adminUser] } }, { currentUsername: 'admin', currentLevel: 'admin', actions: userActions() })

    expect(actionButton(view, 'Set admin level to member')?.disabled).toBe(true)
    expect(actionButton(view, 'Set admin level to viewer')?.disabled).toBe(true)
    expect(actionButton(view, 'Deactivate admin')?.disabled).toBe(true)
    expect(view.textContent).toContain('You cannot manage your own account.')
  })

  it('disables self admin-seat controls for non-admin accounts', () => {
    const view = renderUsers({ status: 'ready', data: { users: [memberUser] } }, { currentUsername: 'member', currentLevel: 'admin', actions: userActions() })

    expect(actionButton(view, 'Set member level to viewer')?.disabled).toBe(true)
    expect(actionButton(view, 'Grant admin to member')?.disabled).toBe(true)
    expect(actionButton(view, 'Deactivate member')?.disabled).toBe(true)
  })

  it('disables duplicate submissions while the same action is pending', () => {
    const view = renderUsers({ status: 'ready', data: { users: [memberUser] } }, { currentUsername: 'admin', currentLevel: 'admin', actions: userActions(), pendingAction: { username: 'member', type: 'deactivate' } })

    expect(actionButton(view, 'Deactivating member')?.disabled).toBe(true)
    expect(actionButton(view, 'Set member level to viewer')?.disabled).toBe(true)
    expect(actionButton(view, 'Grant admin to member')?.disabled).toBe(true)
  })

  it('shows mutation errors while keeping the user visible', () => {
    const view = renderUsers({ status: 'ready', data: { users: [memberUser] } }, { currentUsername: 'admin', currentLevel: 'admin', actions: userActions(), mutationError: 'admin limit reached' })

    expect(view.querySelector('[role="alert"]')?.textContent).toContain('admin limit reached')
    expect(view.textContent).toContain('member@example.com')
  })
})

function actionButton(root: HTMLElement, label: string): HTMLButtonElement | null {
  return Array.from(root.querySelectorAll('button')).find((button) => button.textContent === label) ?? null
}

function userActions() {
  return {
    onSetUserLevel: vi.fn<(username: string, level: UserLevel) => Promise<void>>().mockResolvedValue(undefined),
    onGrantAdmin: vi.fn<(username: string) => Promise<void>>().mockResolvedValue(undefined),
    onDeactivateUser: vi.fn<(username: string) => Promise<void>>().mockResolvedValue(undefined)
  }
}
