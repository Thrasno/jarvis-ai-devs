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
    expect(rowByName(view, 'admin')?.textContent).toContain('Active')
    expect(rowByName(view, 'admin')?.textContent).not.toContain('State: active')
    expect(view.textContent).toContain('viewer')
    expect(view.textContent).toContain('Level: viewer')
    expect(view.textContent).toContain('Admin seat: no')
    expect(rowByName(view, 'viewer')?.textContent).toContain('Inactive')
    expect(rowByName(view, 'viewer')?.textContent).not.toContain('State: inactive')
  })

  it('renders account status, sync status, and Never last sync independently', () => {
    const user = {
      ...viewerUser,
      sync_status: 'never' as const,
      last_sync_at: null
    }
    const view = renderUsers({ status: 'ready', data: { users: [user] } })

    expect(columnHeaders(view)).toEqual(['User', 'Email', 'Role', 'Admin seat', 'Account status', 'Sync status', 'Last sync', 'Actions'])
    expect(rowByName(view, 'viewer')?.textContent).toContain('Inactive')
    expect(rowByName(view, 'viewer')?.textContent).toContain('Never')
  })

  it('renders unavailable projection status truthfully', () => {
    const view = renderUsers({ status: 'ready', data: { users: [{ ...adminUser, sync_status: 'unknown' as const }] } })

    expect(rowByName(view, 'admin')?.textContent).toContain('Unavailable')
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

  it('does not render deferred filters or pagination controls', () => {
    const view = renderUsers({ status: 'ready', data: { users: [viewerUser] } })

    expect(view.textContent).not.toMatch(/filter|pagination|next page|previous page/i)
  })

  it('renders read-only management controls to non-admin sessions', () => {
    const view = renderUsers({ status: 'ready', data: { users: [memberUser] } }, { currentUsername: 'viewer', currentLevel: 'viewer', actions: userActions() })

    expect(view.textContent).toContain('Admin access is required to change users.')
    expect(buttonByText(roleSwitcher(view, 'member'), 'VIEWER')?.disabled).toBe(true)
    expect(buttonByText(roleSwitcher(view, 'member'), 'ADMIN')?.disabled).toBe(true)
    expect(statusButton(view, 'Mark member inactive')?.disabled).toBe(true)
  })

  it('renders role-level controls for supported non-admin level changes', () => {
    const view = renderUsers({ status: 'ready', data: { users: [adminUser, viewerUser] } }, { currentUsername: 'owner', currentLevel: 'admin', actions: userActions() })

    expect(buttonByText(roleSwitcher(view, 'admin'), 'MEMBER')).not.toBeNull()
    expect(buttonByText(roleSwitcher(view, 'admin'), 'VIEWER')).not.toBeNull()
    expect(buttonByText(roleSwitcher(view, 'viewer'), 'MEMBER')).not.toBeNull()
  })

  it('requires confirmation before changing a user level and skips the mutation when cancelled', async () => {
    const actions = userActions()
    const view = renderUsers({ status: 'ready', data: { users: [memberUser] } }, { currentUsername: 'admin', currentLevel: 'admin', actions })

    buttonByText(roleSwitcher(view, 'member'), 'VIEWER')!.click()

    expect(view.querySelector('[role="dialog"]')?.textContent).toContain('Change member level to viewer?')
    actionButton(view, 'Cancel')!.click()
    await Promise.resolve()

    expect(actions.onSetUserLevel).not.toHaveBeenCalled()
    expect(view.querySelector('[role="dialog"]')).toBeNull()
  })

  it('calls the injected level action after visible confirmation', async () => {
    const actions = userActions()
    const view = renderUsers({ status: 'ready', data: { users: [viewerUser] } }, { currentUsername: 'admin', currentLevel: 'admin', actions })

    buttonByText(roleSwitcher(view, 'viewer'), 'MEMBER')!.click()
    actionButton(view, 'Confirm')!.click()
    await Promise.resolve()

    expect(actions.onSetUserLevel).toHaveBeenCalledWith('viewer', 'member')
  })

  it('calls the injected level action for admin to member changes', async () => {
    const actions = userActions()
    const view = renderUsers({ status: 'ready', data: { users: [adminUser] } }, { currentUsername: 'owner', currentLevel: 'admin', actions })

    buttonByText(roleSwitcher(view, 'admin'), 'MEMBER')!.click()
    actionButton(view, 'Confirm')!.click()
    await Promise.resolve()

    expect(actions.onSetUserLevel).toHaveBeenCalledWith('admin', 'member')
  })

  it('requires confirmation before changing a user to admin', async () => {
    const actions = userActions()
    const view = renderUsers({ status: 'ready', data: { users: [memberUser] } }, { currentUsername: 'admin', currentLevel: 'admin', actions })

    buttonByText(roleSwitcher(view, 'member'), 'ADMIN')!.click()
    expect(view.querySelector('[role="dialog"]')?.textContent).toContain('Change member level to admin?')
    actionButton(view, 'Confirm')!.click()
    await Promise.resolve()

    expect(actions.onSetUserLevel).toHaveBeenCalledWith('member', 'admin')
  })

  it('requires confirmation before deactivating a user', async () => {
    const actions = userActions()
    const view = renderUsers({ status: 'ready', data: { users: [memberUser] } }, { currentUsername: 'admin', currentLevel: 'admin', actions })

    statusButton(view, 'Mark member inactive')!.click()
    expect(view.querySelector('[role="dialog"]')?.textContent).toContain('Mark member inactive?')
    actionButton(view, 'Confirm')!.click()
    await Promise.resolve()

    expect(actions.onDeactivateUser).toHaveBeenCalledWith('member')
  })

  it('disables self management controls', () => {
    const view = renderUsers({ status: 'ready', data: { users: [adminUser] } }, { currentUsername: 'admin', currentLevel: 'admin', actions: userActions() })

    expect(buttonByText(roleSwitcher(view, 'admin'), 'MEMBER')?.disabled).toBe(true)
    expect(buttonByText(roleSwitcher(view, 'admin'), 'VIEWER')?.disabled).toBe(true)
    expect(statusButton(view, 'Mark admin inactive')?.disabled).toBe(true)
    expect(view.textContent).toContain('You cannot manage your own account.')
    expect(rowByName(view, 'admin')?.textContent).not.toContain('You cannot manage your own account.')
  })

  it('disables self admin-seat controls for non-admin accounts', () => {
    const view = renderUsers({ status: 'ready', data: { users: [memberUser] } }, { currentUsername: 'member', currentLevel: 'admin', actions: userActions() })

    expect(buttonByText(roleSwitcher(view, 'member'), 'VIEWER')?.disabled).toBe(true)
    expect(buttonByText(roleSwitcher(view, 'member'), 'ADMIN')?.disabled).toBe(true)
    expect(statusButton(view, 'Mark member inactive')?.disabled).toBe(true)
  })

  it('disables duplicate submissions while the same action is pending', () => {
    const view = renderUsers({ status: 'ready', data: { users: [memberUser] } }, { currentUsername: 'admin', currentLevel: 'admin', actions: userActions(), pendingAction: { username: 'member', type: 'deactivate' } })

    expect(statusButton(view, 'Marking member inactive…')?.disabled).toBe(true)
    expect(buttonByText(roleSwitcher(view, 'member'), 'VIEWER')?.disabled).toBe(true)
    expect(buttonByText(roleSwitcher(view, 'member'), 'ADMIN')?.disabled).toBe(true)
  })

  it('shows mutation errors while keeping the user visible', () => {
    const view = renderUsers({ status: 'ready', data: { users: [memberUser] } }, { currentUsername: 'admin', currentLevel: 'admin', actions: userActions(), mutationError: 'admin limit reached' })

    expect(view.querySelector('[role="alert"]')?.textContent).toContain('admin limit reached')
    expect(view.textContent).toContain('member@example.com')
  })

  it('renders a screen-specific table layout without panel chrome or fabricated last-sync values', () => {
    const view = renderUsers({ status: 'ready', data: { users: [adminUser, memberUser] } }, { currentUsername: 'admin', currentLevel: 'admin', actions: userActions() })

    expect(view.dataset.dashboardView).toBe('users')
    expect(view.dataset.dashboardPrimitive).not.toBe('panel')
    expect(view.querySelector('[data-dashboard-primitive="panel"]')).toBeNull()
    expect(view.querySelector('article[role="listitem"]')).toBeNull()
    expect(view.querySelector('[role="table"]')?.getAttribute('aria-label')).toBe('Managed users')
    expect(columnHeaders(view)).toEqual(['User', 'Email', 'Role', 'Admin seat', 'Account status', 'Sync status', 'Last sync', 'Actions'])
    expect(rowByName(view, 'admin')?.textContent).toContain('admin@example.com')
    expect(rowByName(view, 'member')?.textContent).toContain('member@example.com')
    expect(view.textContent).toContain('Last sync')
  })

  it('shows active admin-seat usage in a banner for admins', () => {
    const inactiveAdmin = { ...adminUser, id: 'inactive-admin', username: 'inactive-admin', email: 'inactive-admin@example.com', is_active: false }
    const view = renderUsers({ status: 'ready', data: { users: [adminUser, inactiveAdmin, memberUser] } }, { currentUsername: 'admin', currentLevel: 'admin', actions: userActions() })

    const banner = view.querySelector('[role="status"]')
    expect(banner?.textContent).toContain('Admin seats')
    expect(banner?.textContent).toContain('1 of 3 active admin seats used')
    expect(banner?.textContent).not.toContain('inactive-admin@example.com')
  })

  it('renders one segmented role switcher per user instead of separate grant-admin controls', () => {
    const view = renderUsers({ status: 'ready', data: { users: [memberUser] } }, { currentUsername: 'admin', currentLevel: 'admin', actions: userActions() })
    const switcher = view.querySelector('[role="group"][aria-label="member role"]')

    expect(buttonTexts(switcher)).toEqual(['VIEWER', 'MEMBER', 'ADMIN'])
    expect(buttonByText(switcher, 'MEMBER')?.getAttribute('aria-pressed')).toBe('true')
    expect(buttonByText(switcher, 'VIEWER')?.disabled).toBe(false)
    expect(buttonByText(switcher, 'ADMIN')?.disabled).toBe(false)
    expect(actionButton(view, 'Grant admin to member')).toBeNull()
  })

  it('keeps visible user-management controls disabled with permission context for non-admin sessions', () => {
    const view = renderUsers({ status: 'ready', data: { users: [memberUser] } }, { currentUsername: 'viewer', currentLevel: 'viewer', actions: userActions() })

    expect(view.textContent).toContain('Admin access is required to change users.')
    expect(buttonByText(view, 'VIEWER')?.disabled).toBe(true)
    expect(buttonByText(view, 'MEMBER')?.disabled).toBe(true)
    expect(buttonByText(view, 'ADMIN')?.disabled).toBe(true)
    expect(actionButton(view, 'Reset password for member')?.disabled).toBe(true)
    expect(statusButton(view, 'Mark member inactive')?.disabled).toBe(true)
  })

  it('uses one status toggle button and does not call the mutation when confirmation is cancelled', async () => {
    const actions = userActions()
    const view = renderUsers({ status: 'ready', data: { users: [memberUser, viewerUser] } }, { currentUsername: 'admin', currentLevel: 'admin', actions })

    expect(actionButton(view, 'Deactivate member')).toBeNull()
    expect(actionButton(view, 'Activate viewer')).toBeNull()
    expect(actionButton(view, 'Mark member inactive')).toBeNull()
    expect(statusButton(view, 'Mark member inactive')?.textContent).toBe('Active')
    statusButton(view, 'Mark member inactive')!.click()
    expect(view.querySelector('[role="dialog"]')?.textContent).toContain('Mark member inactive?')
    actionButton(view, 'Cancel')!.click()
    await Promise.resolve()

    expect(actions.onDeactivateUser).not.toHaveBeenCalled()
    expect(actions.onActivateUser).not.toHaveBeenCalled()
    expect(view.querySelector('[role="dialog"]')).toBeNull()
    expect(statusButton(view, 'Mark viewer active')?.textContent).toBe('Inactive')
  })

  it('blocks empty and short temporary password resets before calling the action', async () => {
    const actions = userActions()
    const view = renderUsers({ status: 'ready', data: { users: [memberUser] } }, { currentUsername: 'admin', currentLevel: 'admin', actions })

    actionButton(view, 'Reset password for member')!.click()
    actionButton(view, 'Confirm')!.click()
    await Promise.resolve()

    expect(actions.onResetTemporaryPassword).not.toHaveBeenCalled()
    expect(view.querySelector('[role="alert"]')?.textContent).toContain('Temporary password must be at least 8 characters.')

    view.querySelector<HTMLInputElement>('input[name="temporary_password"]')!.value = 'short'
    actionButton(view, 'Confirm')!.click()
    await Promise.resolve()

    expect(actions.onResetTemporaryPassword).not.toHaveBeenCalled()
  })

  it('disables inactive-user activation when no activate action is injected', async () => {
    const baseActions = userActions()
    const actions = {
      onCreateUser: baseActions.onCreateUser,
      onSetUserLevel: baseActions.onSetUserLevel,
      onDeactivateUser: baseActions.onDeactivateUser,
      onResetTemporaryPassword: baseActions.onResetTemporaryPassword
    }
    const view = renderUsers({ status: 'ready', data: { users: [viewerUser] } }, { currentUsername: 'admin', currentLevel: 'admin', actions })

    const activate = statusButton(view, 'Mark viewer active')
    expect(activate).not.toBeNull()
    expect(activate?.disabled).toBe(true)

    activate!.click()
    await Promise.resolve()

    expect(view.querySelector('[role="dialog"]')).toBeNull()
    expect(actions.onDeactivateUser).not.toHaveBeenCalled()
  })

  it('blocks empty and short create-user temporary passwords before calling the action', async () => {
    const actions = userActions()
    const view = renderUsers({ status: 'ready', data: { users: [memberUser] } }, { currentUsername: 'admin', currentLevel: 'admin', actions })
    const form = view.querySelector<HTMLFormElement>('form[aria-label="Create user"]')!
    form.querySelector<HTMLInputElement>('input[name="username"]')!.value = 'created'
    form.querySelector<HTMLInputElement>('input[name="email"]')!.value = 'created@example.com'

    form.dispatchEvent(new SubmitEvent('submit', { bubbles: true, cancelable: true }))
    await Promise.resolve()

    expect(actions.onCreateUser).not.toHaveBeenCalled()
    expect(form.querySelector('[role="alert"]')?.textContent).toContain('Temporary password must be at least 8 characters.')

    form.querySelector<HTMLInputElement>('input[name="temporary_password"]')!.value = 'short'
    form.dispatchEvent(new SubmitEvent('submit', { bubbles: true, cancelable: true }))
    await Promise.resolve()

    expect(actions.onCreateUser).not.toHaveBeenCalled()
  })
})

function actionButton(root: HTMLElement, label: string): HTMLButtonElement | null {
  return Array.from(root.querySelectorAll('button')).find((button) => button.textContent === label) ?? null
}

function statusButton(root: HTMLElement, label: string): HTMLButtonElement | null {
  return root.querySelector<HTMLButtonElement>(`button[data-user-status-action][aria-label="${label}"]`)
}

function columnHeaders(root: HTMLElement): string[] {
  return Array.from(root.querySelectorAll('[role="columnheader"]')).map((header) => header.textContent ?? '')
}

function rowByName(root: HTMLElement, username: string): HTMLElement | null {
  return root.querySelector<HTMLElement>(`[role="row"][aria-label="${username} user account"]`)
}

function roleSwitcher(root: HTMLElement, username: string): HTMLElement | null {
  return root.querySelector<HTMLElement>(`[role="group"][aria-label="${username} role"]`)
}

function buttonTexts(root: ParentNode | null): string[] {
  return Array.from(root?.querySelectorAll('button') ?? []).map((button) => button.textContent ?? '')
}

function buttonByText(root: ParentNode | null, label: string): HTMLButtonElement | null {
  return Array.from(root?.querySelectorAll('button') ?? []).find((button) => button.textContent === label) ?? null
}

function userActions() {
  return {
    onCreateUser: vi.fn().mockResolvedValue(undefined),
    onSetUserLevel: vi.fn<(username: string, level: UserLevel) => Promise<void>>().mockResolvedValue(undefined),
    onDeactivateUser: vi.fn<(username: string) => Promise<void>>().mockResolvedValue(undefined),
    onResetTemporaryPassword: vi.fn<(username: string, temporaryPassword: string) => Promise<void>>().mockResolvedValue(undefined),
    onActivateUser: vi.fn<(username: string) => Promise<void>>().mockResolvedValue(undefined)
  }
}
