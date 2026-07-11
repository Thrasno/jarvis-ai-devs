import { ApiError, type ChangePasswordRequest } from '../api/client'

const MIN_PASSWORD_BYTES = 8
const MAX_PASSWORD_BYTES = 72

export type AccountActions = {
  onChangePassword(request: ChangePasswordRequest): Promise<void>
}

export function renderAccount(actions: AccountActions): HTMLElement {
  const root = document.createElement('section')
  root.dataset.dashboardView = 'account'
  root.setAttribute('aria-labelledby', 'account-title')
  root.innerHTML = `
    <p>Account security</p>
    <h2 id="account-title">Change password</h2>
    <form aria-label="Change password">
      <label>Current password<input name="current_password" type="password" autocomplete="current-password" required></label>
      <label>New password<input name="new_password" type="password" autocomplete="new-password" required></label>
      <label>Confirm new password<input name="confirmation" type="password" autocomplete="new-password" required></label>
      <button type="submit">Change password</button>
    </form>`
  const form = root.querySelector<HTMLFormElement>('form')!
  const submit = form.querySelector<HTMLButtonElement>('button')!
  form.addEventListener('submit', (event) => {
    event.preventDefault()
    form.querySelector('[role="alert"]')?.remove()
    const currentPassword = value(form, 'current_password')
    const newPassword = value(form, 'new_password')
    const confirmation = value(form, 'confirmation')
    const error = validationError(newPassword, confirmation)
    if (error) return showError(form, error)
    submit.disabled = true
    submit.textContent = 'Changing password…'
    void actions.onChangePassword({ current_password: currentPassword, new_password: newPassword })
      .catch((caught: unknown) => {
        form.reset()
        showError(form, messageFor(caught))
      })
      .finally(() => {
        submit.disabled = false
        submit.textContent = 'Change password'
      })
  })
  return root
}

function value(form: HTMLFormElement, name: string): string {
  return form.querySelector<HTMLInputElement>(`input[name="${name}"]`)!.value
}

function validationError(password: string, confirmation: string): string | undefined {
  const length = new TextEncoder().encode(password).length
  if (length < MIN_PASSWORD_BYTES || length > MAX_PASSWORD_BYTES) return 'New password must be between 8 and 72 bytes.'
  if (password !== confirmation) return 'New passwords do not match.'
}

function messageFor(error: unknown): string {
  if (error instanceof ApiError && error.code === 'CURRENT_PASSWORD_INVALID') return 'Your current password is incorrect.'
  if (error instanceof ApiError && error.code === 'VALIDATION_ERROR') return 'Check the password requirements and try again.'
  return 'Unable to change password. Please try again.'
}

function showError(form: HTMLFormElement, message: string): void {
  const alert = document.createElement('p')
  alert.setAttribute('role', 'alert')
  alert.textContent = message
  form.append(alert)
}
