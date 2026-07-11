import { describe, expect, it, vi } from 'vitest'
import { ApiError } from '../api/client'
import { renderAccount } from './Account'

describe('account view', () => {
  it('uses accessible password inputs and validates UTF-8 byte limits plus confirmation before submitting', async () => {
    const onChangePassword = vi.fn().mockResolvedValue(undefined)
    const view = renderAccount({ onChangePassword })
    const form = view.querySelector<HTMLFormElement>('form[aria-label="Change password"]')!
    const current = form.querySelector<HTMLInputElement>('input[name="current_password"]')!
    const next = form.querySelector<HTMLInputElement>('input[name="new_password"]')!
    const confirmation = form.querySelector<HTMLInputElement>('input[name="confirmation"]')!

    expect(current.type).toBe('password')
    expect(next.autocomplete).toBe('new-password')
    current.value = 'current-password'
    next.value = 'é'.repeat(37)
    confirmation.value = next.value
    form.dispatchEvent(new SubmitEvent('submit', { bubbles: true, cancelable: true }))
    await Promise.resolve()

    expect(onChangePassword).not.toHaveBeenCalled()
    expect(form.querySelector('[role="alert"]')?.textContent).toContain('between 8 and 72 bytes')

    next.value = 'new-password'
    confirmation.value = 'different-password'
    form.dispatchEvent(new SubmitEvent('submit', { bubbles: true, cancelable: true }))
    await Promise.resolve()

    expect(onChangePassword).not.toHaveBeenCalled()
    expect(form.querySelector('[role="alert"]')?.textContent).toContain('do not match')
  })

  it('maps current-password failures to the field and clears values after a failed request', async () => {
    const onChangePassword = vi.fn().mockRejectedValue(new ApiError('Invalid current password', 400, 'CURRENT_PASSWORD_INVALID'))
    const view = renderAccount({ onChangePassword })
    const form = view.querySelector<HTMLFormElement>('form[aria-label="Change password"]')!
    for (const [name, value] of [['current_password', 'old-password'], ['new_password', 'new-password'], ['confirmation', 'new-password']] as const) {
      form.querySelector<HTMLInputElement>(`input[name="${name}"]`)!.value = value
    }

    form.dispatchEvent(new SubmitEvent('submit', { bubbles: true, cancelable: true }))
    await Promise.resolve()

    expect(onChangePassword).toHaveBeenCalledWith({ current_password: 'old-password', new_password: 'new-password' })
    expect(form.querySelector('[role="alert"]')?.textContent).toContain('current password')
    expect(Array.from(form.querySelectorAll<HTMLInputElement>('input')).every((input) => input.value === '')).toBe(true)
  })
})
