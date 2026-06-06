import { afterEach, vi } from 'vitest'

afterEach(() => {
  sessionStorage.clear()
  document.body.innerHTML = ''
  vi.restoreAllMocks()
})
