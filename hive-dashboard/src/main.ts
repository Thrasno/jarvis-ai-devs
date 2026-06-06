import { createSessionStore, type AuthState } from './auth/session'
import './styles.css'

type AppActions = {
  onLogin(email: string, password: string): Promise<void> | void
  onLogout(): void
}

export function renderApp(container: HTMLElement, state: AuthState, actions: AppActions): void {
  container.replaceChildren()
  state.status === 'anonymous' ? renderLogin(container, actions) : renderShell(container, state, actions)
}

function renderLogin(container: HTMLElement, actions: AppActions): void {
  const form = document.createElement('form')
  form.className = 'card login-card'
  form.innerHTML = `
    <p class="eyebrow">Hive API</p>
    <h1>Sign in to Hive API</h1>
    <label>Email<input name="email" type="email" autocomplete="email" required /></label>
    <label>Password<input name="password" type="password" autocomplete="current-password" required /></label>
    <button type="submit">Sign in</button>
  `
  form.addEventListener('submit', async (event) => {
    event.preventDefault()
    const data = new FormData(form)
    await actions.onLogin(String(data.get('email') ?? ''), String(data.get('password') ?? ''))
  })
  container.append(form)
}

function renderShell(container: HTMLElement, state: Extract<AuthState, { status: 'authenticated' }>, actions: AppActions): void {
  const header = document.createElement('header')
  header.innerHTML = '<p class="eyebrow">Hive API</p><h1>Hive API Dashboard</h1>'
  const identity = document.createElement('p')
  identity.textContent = `Signed in as ${state.user.email}`
  const logout = document.createElement('button')
  logout.type = 'button'
  logout.textContent = 'Sign out'
  logout.addEventListener('click', actions.onLogout)
  header.append(identity, logout)

  const panel = document.createElement('article')
  panel.className = 'card'
  panel.innerHTML =
    state.user.level === 'admin'
      ? '<h2>Admin dashboard</h2><p>Authentication is active. API-backed views arrive in the next slice.</p>'
      : '<h2>Admin access required</h2><p>This dashboard requires an admin Hive API identity.</p>'

  const shell = document.createElement('section')
  shell.className = 'shell'
  shell.append(header, panel)
  container.append(shell)
}

const root = document.getElementById('app')
if (root) {
  const session = createSessionStore()
  const rerender = (state: AuthState) => renderApp(root, state, actions)
  const actions: AppActions = {
    async onLogin(email, password) {
      rerender(await session.login(email, password))
    },
    onLogout() {
      rerender(session.logout())
    }
  }

  session.bootstrap().then(rerender)
}
