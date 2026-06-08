import type { User } from '../api/client'
import { append, error, list, panel, text } from '../components/dom'
import type { ViewState } from './Overview'

export function renderUsers(state: ViewState<{ users: User[] }>): HTMLElement {
  const card = panel('Users')
  if (state.status === 'loading') return append(card, text('Loading users…'))
  if (state.status === 'error') return error(card, state.message)
  return append(card, list(state.data.users.map((user) => `${user.email} — ${user.level} · ${user.is_active ? 'active' : 'inactive'}`), 'No users found'))
}
