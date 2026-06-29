import type { CreateUserRequest, User, UserLevel } from '../api/client'
import { append, emptyState, error, text } from '../components/dom'
import type { ViewState } from './Overview'

const ROLE_OPTIONS: UserLevel[] = ['viewer', 'member', 'admin']
const MAX_ACTIVE_ADMIN_SEATS = 3
const TEMPORARY_PASSWORD_MIN_LENGTH = 8

export type UserManagementActions = {
  onCreateUser?: (request: CreateUserRequest) => Promise<void>
  onSetUserLevel(username: string, level: UserLevel): Promise<void>
  onDeactivateUser(username: string): Promise<void>
  onResetTemporaryPassword?: (username: string, temporaryPassword: string) => Promise<void>
  onActivateUser?: (username: string) => Promise<void>
}

export type UserManagementActionType = 'create' | 'level' | 'grant-admin' | 'deactivate' | 'reset-password' | 'activate'

export type UserManagementOptions = {
  currentUsername?: string
  currentLevel?: UserLevel
  pendingAction?: { username: string; type: UserManagementActionType }
  mutationError?: string
  refreshError?: string
  actions?: UserManagementActions
}

export function renderUsers(state: ViewState<{ users: User[] }>, options: UserManagementOptions = {}): HTMLElement {
  const root = usersRoot()
  root.append(usersHeader())

  if (state.status === 'loading') return append(root, emptyState('Loading users…'))
  if (state.status === 'error') return error(root, state.message)

  root.append(adminSeatBanner(state.data.users))
  const selfManagementCopy = selfManagementNotice(state.data.users, options)
  if (selfManagementCopy) root.append(selfManagementCopy)
  if (options.currentLevel !== 'admin') root.append(permissionNotice())
  if (options.mutationError) root.append(inlineAlert(options.mutationError))
  if (options.refreshError) root.append(inlineAlert(options.refreshError))
  if (options.actions?.onCreateUser && options.currentLevel === 'admin') root.append(createUserForm(options))
  if (state.data.users.length === 0) return append(root, emptyState('No users found'))

  root.append(usersTable(state.data.users, options))
  return root
}

function usersRoot(): HTMLElement {
  const section = document.createElement('section')
  section.className = 'dashboard-users'
  section.dataset.dashboardView = 'users'
  section.setAttribute('role', 'region')
  section.setAttribute('aria-labelledby', 'dashboard-users-title')
  return section
}

function usersHeader(): HTMLElement {
  const header = document.createElement('div')
  header.className = 'dashboard-users__header'
  header.innerHTML = `
    <p class="dashboard-users__eyebrow">Access governance</p>
    <h2 id="dashboard-users-title" class="dashboard-users__title">Users</h2>
  `
  return header
}

function adminSeatBanner(users: User[]): HTMLElement {
  const activeAdmins = users.filter((user) => user.is_active && isAdminSeat(user)).length
  const banner = document.createElement('aside')
  banner.className = 'dashboard-users__seat-banner'
  banner.setAttribute('role', 'status')
  banner.textContent = `Admin seats · ${activeAdmins} of ${MAX_ACTIVE_ADMIN_SEATS} active admin seats used`
  return banner
}

function permissionNotice(): HTMLElement {
  const notice = document.createElement('p')
  notice.className = 'dashboard-users__permission-note'
  notice.textContent = 'Admin access is required to change users.'
  return notice
}

function selfManagementNotice(users: User[], options: UserManagementOptions): HTMLElement | null {
  if (options.currentLevel !== 'admin' || !options.currentUsername) return null
  if (!users.some((user) => user.username === options.currentUsername)) return null
  const notice = document.createElement('p')
  notice.className = 'dashboard-users__self-note'
  notice.textContent = 'You cannot manage your own account.'
  return notice
}

function createUserForm(options: UserManagementOptions): HTMLFormElement {
  const form = document.createElement('form')
  form.className = 'dashboard-users__create-form'
  form.setAttribute('aria-label', 'Create user')
  const pending = options.pendingAction?.type === 'create'
  form.innerHTML = `
    <label>Username<input name="username" autocomplete="username" required /></label>
    <label>Email<input name="email" type="email" autocomplete="email" required /></label>
    <label>Role
      <select name="level">
        <option value="member" selected>member</option>
        <option value="viewer">viewer</option>
        <option value="admin">admin</option>
      </select>
    </label>
    <label>Temporary password<input name="temporary_password" type="password" autocomplete="new-password" required /></label>
  `
  const submit = button(pending ? 'Creating user…' : 'Create user', { className: 'dashboard-users__button dashboard-users__button--primary', disabled: pending || Boolean(options.pendingAction) })
  submit.type = 'submit'
  form.append(submit)
  form.addEventListener('submit', (event) => {
    event.preventDefault()
    if (pending || options.pendingAction) return
    form.querySelector('[role="alert"]')?.remove()
    const data = new FormData(form)
    const temporaryPassword = String(data.get('temporary_password') ?? '')
    if (!isValidTemporaryPassword(temporaryPassword)) {
      form.append(inlineAlert(temporaryPasswordLengthMessage()))
      return
    }
    void options.actions?.onCreateUser?.({
      username: String(data.get('username') ?? '').trim(),
      email: String(data.get('email') ?? '').trim(),
      level: userLevelFromForm(data.get('level')),
      temporary_password: temporaryPassword
    })
  })
  return form
}

function usersTable(users: User[], options: UserManagementOptions): HTMLElement {
  const table = document.createElement('div')
  table.className = 'dashboard-users__table'
  table.setAttribute('role', 'table')
  table.setAttribute('aria-label', 'Managed users')
  table.append(tableHeader())
  const body = document.createElement('div')
  body.className = 'dashboard-users__body'
  body.setAttribute('role', 'rowgroup')
  for (const user of users) body.append(userRow(user, options))
  table.append(body)
  return table
}

function tableHeader(): HTMLElement {
  const header = document.createElement('div')
  header.className = 'dashboard-users__row dashboard-users__row--header'
  header.setAttribute('role', 'row')
  for (const label of ['User', 'Email', 'Role', 'Admin seat', 'Status', 'Actions']) {
    const cell = document.createElement('div')
    cell.setAttribute('role', 'columnheader')
    cell.textContent = label
    header.append(cell)
  }
  return header
}

function userRow(user: User, options: UserManagementOptions): HTMLElement {
  const row = document.createElement('div')
  row.className = 'dashboard-users__row'
  row.setAttribute('role', 'row')
  row.setAttribute('aria-label', `${user.username} user account`)

  row.append(
    cell(identityCell(user)),
    cell(text(user.email || 'Unavailable', 'dashboard-users__email')),
    cell(roleSwitcher(user, options)),
    cell(text(`Admin seat: ${isAdminSeat(user) ? 'yes' : 'no'}`, 'dashboard-users__seat-state')),
    cell(statusCell(user, options)),
    cell(managementControls(user, options))
  )
  return row
}

function identityCell(user: User): HTMLElement {
  const identity = document.createElement('div')
  identity.className = 'dashboard-users__identity'
  identity.append(text(user.username, 'dashboard-users__username'))
  return identity
}

function statusCell(user: User, options: UserManagementOptions): HTMLElement {
  const state = document.createElement('div')
  state.className = 'dashboard-users__status-cell'
  const statusPending = options.pendingAction?.username === user.username && (options.pendingAction.type === 'deactivate' || options.pendingAction.type === 'activate')
  const activateUnavailable = !user.is_active && !options.actions?.onActivateUser
  const status = user.is_active ? 'active' : 'inactive'
  const control = button(statusTagLabel(user, statusPending), {
    className: 'dashboard-users__status-toggle dashboard-status status',
    disabled: !canMutate(user, options) || activateUnavailable
  })
  control.dataset.dashboardPrimitive = 'status'
  control.dataset.dashboardStatus = status
  control.dataset.userStatusAction = status
  control.setAttribute('aria-label', statusButtonLabel(user, statusPending))
  if (!control.disabled) {
    control.addEventListener('click', () => {
      const nextState = user.is_active ? 'inactive' : 'active'
      showConfirmation(state, `Mark ${user.username} ${nextState}?`, () => user.is_active
        ? options.actions!.onDeactivateUser(user.username)
        : options.actions!.onActivateUser!(user.username))
    })
  }
  state.append(control)
  return state
}

function cell(child: HTMLElement): HTMLElement {
  const node = document.createElement('div')
  node.className = 'dashboard-users__cell'
  node.setAttribute('role', 'cell')
  node.append(child)
  return node
}

function roleSwitcher(user: User, options: UserManagementOptions): HTMLElement {
  const group = document.createElement('div')
  group.className = 'dashboard-users__role-switcher'
  group.setAttribute('role', 'group')
  group.setAttribute('aria-label', `${user.username} role`)
  group.append(text(`Level: ${user.level}`, 'dashboard-users__sr-label'))

  for (const level of ROLE_OPTIONS) {
    const pressed = user.level === level
    const roleButton = button(roleLabel(level), {
      className: 'dashboard-users__role-segment',
      disabled: !canMutate(user, options) || pressed,
      pressed
    })
    if (!pressed) {
      roleButton.addEventListener('click', () => {
        showConfirmation(group, `Change ${user.username} level to ${level}?`, () => roleMutation(user.username, level, options))
      })
    }
    group.append(roleButton)
  }
  return group
}

function roleMutation(username: string, level: UserLevel, options: UserManagementOptions): Promise<void> {
  if (level === 'admin' && options.actions?.onSetUserLevel) return options.actions.onSetUserLevel(username, 'admin')
  return options.actions?.onSetUserLevel?.(username, level) ?? Promise.resolve()
}

function roleLabel(level: UserLevel): string {
  return level.toUpperCase()
}

function managementControls(user: User, options: UserManagementOptions): HTMLElement {
  const group = document.createElement('div')
  group.className = 'dashboard-users__actions'
  group.setAttribute('aria-label', `${user.username} management actions`)

  const disabled = !canMutate(user, options)
  const resetPending = options.pendingAction?.username === user.username && options.pendingAction.type === 'reset-password'

  if (options.actions?.onResetTemporaryPassword || options.currentLevel !== 'admin') {
    group.append(actionButton(`${resetPending ? 'Resetting password for' : 'Reset password for'} ${user.username}`, disabled, () => {
      showPasswordResetConfirmation(group, user.username, (temporaryPassword) => options.actions!.onResetTemporaryPassword!(user.username, temporaryPassword))
    }))
  }

  return group
}

function statusButtonLabel(user: User, pending: boolean): string {
  if (pending) return user.is_active ? `Marking ${user.username} inactive…` : `Marking ${user.username} active…`
  return user.is_active ? `Mark ${user.username} inactive` : `Mark ${user.username} active`
}

function statusTagLabel(user: User, pending: boolean): string {
  const label = user.is_active ? 'Active' : 'Inactive'
  return pending ? `${label}…` : label
}

function userLevelFromForm(value: FormDataEntryValue | null): UserLevel {
  return value === 'viewer' || value === 'admin' ? value : 'member'
}

function actionButton(label: string, disabled: boolean, onClick: () => void): HTMLButtonElement {
  const action = button(label, { className: 'dashboard-users__button dashboard-users__button--secondary', disabled })
  action.setAttribute('aria-label', label)
  if (!disabled) action.addEventListener('click', onClick)
  return action
}

function button(label: string, options: { className?: string; disabled?: boolean; pressed?: boolean } = {}): HTMLButtonElement {
  const node = document.createElement('button')
  node.type = 'button'
  node.className = options.className ?? 'dashboard-users__button'
  node.textContent = label
  if (options.pressed !== undefined) node.setAttribute('aria-pressed', String(options.pressed))
  if (options.disabled) {
    node.disabled = true
    node.setAttribute('aria-disabled', 'true')
  }
  return node
}

function showConfirmation(root: HTMLElement, message: string, run: () => Promise<void>): void {
  root.querySelector('[role="dialog"]')?.remove()

  const dialog = document.createElement('div')
  dialog.className = 'dashboard-users__dialog'
  dialog.setAttribute('role', 'dialog')
  dialog.setAttribute('aria-label', message)
  dialog.append(text(message, 'dashboard-users__dialog-title'))

  const confirm = button('Confirm', { className: 'dashboard-users__button dashboard-users__button--danger' })
  confirm.addEventListener('click', () => {
    confirm.disabled = true
    run().catch((caught) => {
      dialog.append(inlineAlert(caught instanceof Error ? caught.message : 'User management action failed'))
    })
  })

  const cancel = button('Cancel', { className: 'dashboard-users__button dashboard-users__button--ghost' })
  cancel.addEventListener('click', () => dialog.remove())
  dialog.append(confirm, cancel)
  root.append(dialog)
}

function showPasswordResetConfirmation(root: HTMLElement, username: string, run: (temporaryPassword: string) => Promise<void>): void {
  root.querySelector('[role="dialog"]')?.remove()

  const dialog = document.createElement('div')
  dialog.className = 'dashboard-users__dialog'
  dialog.setAttribute('role', 'dialog')
  dialog.setAttribute('aria-label', `Reset temporary password for ${username}?`)
  dialog.append(text(`Reset temporary password for ${username}?`, 'dashboard-users__dialog-title'))

  const input = document.createElement('input')
  input.name = 'temporary_password'
  input.type = 'password'
  input.autocomplete = 'new-password'
  input.required = true
  dialog.append(input)

  const confirm = button('Confirm', { className: 'dashboard-users__button dashboard-users__button--danger' })
  confirm.addEventListener('click', () => {
    dialog.querySelector('[role="alert"]')?.remove()
    const temporaryPassword = input.value
    if (!isValidTemporaryPassword(temporaryPassword)) {
      dialog.append(inlineAlert(temporaryPasswordLengthMessage()))
      return
    }
    confirm.disabled = true
    run(temporaryPassword).catch((caught) => {
      dialog.append(inlineAlert(caught instanceof Error ? caught.message : 'User management action failed'))
    })
  })

  const cancel = button('Cancel', { className: 'dashboard-users__button dashboard-users__button--ghost' })
  cancel.addEventListener('click', () => dialog.remove())
  dialog.append(confirm, cancel)
  root.append(dialog)
}

function inlineAlert(message: string): HTMLElement {
  const alert = text(message, 'dashboard-users__alert')
  alert.setAttribute('role', 'alert')
  return alert
}

function isValidTemporaryPassword(temporaryPassword: string): boolean {
  return temporaryPassword.trim().length >= TEMPORARY_PASSWORD_MIN_LENGTH
}

function temporaryPasswordLengthMessage(): string {
  return `Temporary password must be at least ${TEMPORARY_PASSWORD_MIN_LENGTH} characters.`
}

function canMutate(user: User, options: UserManagementOptions): boolean {
  return Boolean(options.actions) && options.currentLevel === 'admin' && options.currentUsername !== user.username && !isUserPending(user, options) && !options.pendingAction
}

function isUserPending(user: User, options: UserManagementOptions): boolean {
  return options.pendingAction?.username === user.username
}

function isAdminSeat(user: User): boolean {
  return user.level === 'admin'
}
