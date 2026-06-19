import type { User, UserLevel } from '../api/client'
import { append, control, emptyState, error, panel, stack, statusBadge, text } from '../components/dom'
import type { ViewState } from './Overview'

export type UserManagementActions = {
  onSetUserLevel(username: string, level: UserLevel): Promise<void>
  onGrantAdmin(username: string): Promise<void>
  onDeactivateUser(username: string): Promise<void>
}

export type UserManagementActionType = 'level' | 'grant-admin' | 'deactivate'

export type UserManagementOptions = {
  currentUsername?: string
  currentLevel?: UserLevel
  pendingAction?: { username: string; type: UserManagementActionType }
  mutationError?: string
  actions?: UserManagementActions
}

export function renderUsers(state: ViewState<{ users: User[] }>, options: UserManagementOptions = {}): HTMLElement {
  const card = panel('Users')
  if (state.status === 'loading') return append(card, text('Loading users…'))
  if (state.status === 'error') return error(card, state.message)
  if (options.mutationError) card.append(inlineAlert(options.mutationError))
  if (state.data.users.length === 0) return append(card, emptyState('No users found'))

  const users = document.createElement('div')
  users.setAttribute('role', 'list')
  users.setAttribute('aria-label', 'Managed users')
  for (const user of state.data.users) {
    users.append(userRow(user, options))
  }
  return append(card, users)
}

function userRow(user: User, options: UserManagementOptions): HTMLElement {
  const row = document.createElement('article')
  row.setAttribute('role', 'listitem')
  row.setAttribute('aria-label', `${user.username} user account`)

  row.append(
    text(user.username),
    text(user.email),
    text(`${user.level} · ${user.is_active ? 'active' : 'inactive'}`),
    text(`Level: ${user.level}`),
    text(`Admin seat: ${isAdminSeat(user) ? 'yes' : 'no'}`),
    stateLine(user)
  )

  const actions = managementControls(user, options)
  if (actions) row.append(actions)
  return row
}

function stateLine(user: User): HTMLElement {
  const line = document.createElement('p')
  line.append(`State: ${user.is_active ? 'active' : 'inactive'} `, statusBadge(user.is_active ? 'active' : 'inactive'))
  return line
}

function managementControls(user: User, options: UserManagementOptions): HTMLElement | null {
  if (!options.actions || options.currentLevel !== 'admin') return null

  const group = stack([])
  group.setAttribute('aria-label', `${user.username} management actions`)
  const disabled = isUserPending(user, options)
  const isSelf = options.currentUsername === user.username

  for (const nextLevel of roleChangeTargets(user.level)) {
    group.append(actionButton(`Set ${user.username} level to ${nextLevel}`, disabled || isSelf, () => {
      showConfirmation(group, `Change ${user.username} level to ${nextLevel}?`, () => options.actions!.onSetUserLevel(user.username, nextLevel))
    }))
  }

  if (!isAdminSeat(user)) {
    group.append(actionButton(`Grant admin to ${user.username}`, disabled || isSelf, () => {
      showConfirmation(group, `Grant admin seat to ${user.username}?`, () => options.actions!.onGrantAdmin(user.username))
    }))
  }

  const deactivationPending = options.pendingAction?.username === user.username && options.pendingAction.type === 'deactivate'
  group.append(actionButton(`${deactivationPending ? 'Deactivating' : 'Deactivate'} ${user.username}`, disabled || isSelf || !user.is_active, () => {
    showConfirmation(group, `Deactivate ${user.username}?`, () => options.actions!.onDeactivateUser(user.username))
  }))
  if (isSelf) group.append(text('You cannot manage your own account.'))

  return group
}

function actionButton(label: string, disabled: boolean, onClick: () => void): HTMLButtonElement {
  const button = control(label, { disabled })
  button.setAttribute('aria-label', label)
  button.addEventListener('click', onClick)
  return button
}

function showConfirmation(root: HTMLElement, message: string, run: () => Promise<void>): void {
  root.querySelector('[role="dialog"]')?.remove()

  const dialog = document.createElement('div')
  dialog.setAttribute('role', 'dialog')
  dialog.setAttribute('aria-label', message)
  dialog.append(text(message))

  const confirm = control('Confirm')
  confirm.addEventListener('click', () => {
    confirm.disabled = true
    run().catch((caught) => {
      dialog.append(inlineAlert(caught instanceof Error ? caught.message : 'User management action failed'))
    })
  })

  const cancel = control('Cancel')
  cancel.addEventListener('click', () => dialog.remove())
  dialog.append(confirm, cancel)
  root.append(dialog)
}

function inlineAlert(message: string): HTMLElement {
  const alert = text(message)
  alert.setAttribute('role', 'alert')
  return alert
}

function isUserPending(user: User, options: UserManagementOptions): boolean {
  return options.pendingAction?.username === user.username
}

function isAdminSeat(user: User): boolean {
  return user.level === 'admin'
}

function roleChangeTargets(level: UserLevel): UserLevel[] {
  if (level === 'admin') return ['member', 'viewer']
  if (level === 'member') return ['viewer']
  return ['member']
}
