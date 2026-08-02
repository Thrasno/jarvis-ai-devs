import type { QuarantineDetailResponse, QuarantineSummary } from '../api/client'
import type { ViewState } from './Overview'

export const quarantineStates = {
  PENDING: 'pending',
  UNSUPPORTED: 'unsupported'
} as const

const pollingDelays = {
  INITIAL: 15_000,
  MAXIMUM: 60_000
} as const

export type QuarantineViewData = {
  summaries: readonly QuarantineSummary[]
  detail?: QuarantineDetailResponse
}

export type QuarantineViewOptions = {
  selectedProject?: string
  filter: string
  pendingRelease?: boolean
  message?: string
  scrollTop?: number
  onSelect?(project: string): void
  onRelease?(detail: QuarantineDetailResponse): Promise<void> | void
  onNextPage?(): void
  onScrollTop?(scrollTop: number): void
}

export type QuarantineControllerState = QuarantineViewOptions & {
  status: 'ready' | typeof quarantineStates.UNSUPPORTED
  detail?: QuarantineDetailResponse
  retryDelayMs: number
  pollingStopped: boolean
  cursor?: string
  nextCursor?: string
  scrollTop: number
}

export type QuarantineControllerOptions = {
  fetchDetail(project: string, generation?: number, after?: string, signal?: AbortSignal): Promise<QuarantineDetailResponse>
  release(detail: QuarantineDetailResponse, signal?: AbortSignal): Promise<unknown>
  onReleaseSuccess?(): Promise<void> | void
  onUnauthorized?(): void
  onUpdate?(state: QuarantineControllerState): void
}

export function createQuarantineController(options: QuarantineControllerOptions): {
  readonly state: QuarantineControllerState
  select(project: string): void
  setFilter(filter: string): void
  setCursor(cursor?: string): void
  setScrollTop(scrollTop: number): void
  hydrate(detail: QuarantineDetailResponse): void
  refresh(): Promise<void>
  release(): Promise<void>
  startPolling(): void
  setVisibility(hidden: boolean): void
  destroy(): void
  unsupported(): void
} {
  const state: QuarantineControllerState = { status: 'ready', filter: '', retryDelayMs: pollingDelays.INITIAL, pollingStopped: false, scrollTop: 0 }
  let requestSequence = 0
  let releaseSequence = 0
  let destroyed = false
  let hidden = false
  let timer: ReturnType<typeof setTimeout> | undefined
  let request: AbortController | undefined
  let releaseRequest: AbortController | undefined

  const stopTimer = () => {
    if (timer !== undefined) clearTimeout(timer)
    timer = undefined
  }
  const schedule = () => {
    stopTimer()
    if (destroyed || hidden || state.pollingStopped || !state.selectedProject) return
    timer = setTimeout(() => {
      void refreshAndSchedule()
    }, pollingDelays.INITIAL)
  }
  const refreshAndSchedule = async () => {
    await refresh()
    schedule()
  }
  const refresh = async () => {
    if (!state.selectedProject || state.status === quarantineStates.UNSUPPORTED || state.pollingStopped || destroyed) return
    request?.abort()
    const controller = new AbortController()
    request = controller
    const sequence = ++requestSequence
    try {
      const detail = await options.fetchDetail(state.selectedProject, state.detail?.generation, state.cursor, controller.signal)
      if (destroyed || controller.signal.aborted || sequence !== requestSequence || (state.detail && detail.generation < state.detail.generation)) return
      state.detail = detail
      state.nextCursor = detail.next_cursor
      state.message = undefined
      state.retryDelayMs = pollingDelays.INITIAL
      options.onUpdate?.(state)
    } catch (error) {
      if (destroyed || controller.signal.aborted || sequence !== requestSequence) return
      if (isUnauthorizedError(error)) {
        state.pollingStopped = true
        stopTimer()
        options.onUnauthorized?.()
        return
      }
      if (isAuthorizationError(error)) {
        state.pollingStopped = true
        state.message = 'Quarantine Center access denied.'
        stopTimer()
        return
      }
      if (isUnsupportedError(error)) {
        state.status = quarantineStates.UNSUPPORTED
        state.pollingStopped = true
        state.message = 'Quarantine Center is not supported by this server.'
        stopTimer()
        return
      }
      state.retryDelayMs = Math.min(state.retryDelayMs * 2, pollingDelays.MAXIMUM)
      state.message = messageFor(error)
    } finally {
      if (request === controller) request = undefined
    }
  }
  const notify = () => options.onUpdate?.(state)
  return {
    state,
    select(project) {
      state.selectedProject = project
      state.cursor = undefined
      schedule()
    },
    setFilter(filter) { state.filter = filter },
    setCursor(cursor) { state.cursor = cursor },
    setScrollTop(scrollTop) { state.scrollTop = scrollTop },
    hydrate(detail) {
      if (state.detail && detail.generation < state.detail.generation) return
      state.detail = detail
      state.nextCursor = detail.next_cursor
    },
    refresh,
    async release() {
      if (!state.detail || state.pendingRelease || destroyed) return
      const detail = state.detail
      const sequence = ++releaseSequence
      releaseRequest?.abort()
      const controller = new AbortController()
      releaseRequest = controller
      state.pendingRelease = true
      state.message = undefined
      notify()
      try {
        await options.release(detail, controller.signal)
        if (destroyed || controller.signal.aborted || sequence !== releaseSequence || state.selectedProject !== detail.canonical_project_key || state.detail?.generation !== detail.generation) return
        await options.onReleaseSuccess?.()
      } catch (error) {
        if (destroyed || controller.signal.aborted || sequence !== releaseSequence) return
        if (isUnauthorizedError(error)) {
          state.pollingStopped = true
          stopTimer()
          options.onUnauthorized?.()
          return
        }
        state.message = `Release failed: ${messageFor(error)}`
        notify()
      } finally {
        if (releaseRequest === controller) {
          releaseRequest = undefined
          if (!destroyed && sequence === releaseSequence) {
            state.pendingRelease = false
            notify()
          }
        }
      }
    },
    startPolling() { schedule() },
    setVisibility(isHidden) {
      hidden = isHidden
      if (hidden) stopTimer()
      else schedule()
    },
    destroy() {
      destroyed = true
      requestSequence += 1
      releaseSequence += 1
      stopTimer()
      request?.abort()
      releaseRequest?.abort()
    },
    unsupported() {
      state.status = quarantineStates.UNSUPPORTED
      state.pollingStopped = true
      state.message = 'Quarantine Center is not supported by this server.'
    }
  }
}

export function renderQuarantine(state: ViewState<QuarantineViewData>, options: QuarantineViewOptions): HTMLElement {
  const root = document.createElement('section')
  root.className = 'dashboard-quarantine'
  root.dataset.dashboardView = 'quarantine'
  root.setAttribute('aria-labelledby', 'dashboard-quarantine-title')
  root.scrollTop = options.scrollTop ?? 0
  root.addEventListener('scroll', () => options.onScrollTop?.(root.scrollTop))
  const title = document.createElement('h2')
  title.id = 'dashboard-quarantine-title'
  title.textContent = 'Quarantine Center'
  root.append(title)
  if (state.status === 'loading') return appendStatus(root, 'Loading quarantined projects…')
  if (state.status === 'error') return appendAlert(root, state.message)
  if (options.message) appendAlert(root, options.message)
  const selected = state.data.detail
  const filter = document.createElement('input')
  filter.type = 'search'
  filter.value = options.filter
  filter.placeholder = 'Filter usernames'
  filter.setAttribute('aria-label', 'Filter quarantine progress by username')
  root.append(filter)
  const list = document.createElement('ul')
  list.setAttribute('aria-label', 'Quarantined projects')
  for (const summary of state.data.summaries) {
    const item = document.createElement('li')
    const button = document.createElement('button')
    button.type = 'button'
    button.textContent = `${summary.project} · generation ${summary.generation}`
    button.dataset.quarantineProject = summary.canonical_project_key
    if (summary.canonical_project_key === options.selectedProject) button.setAttribute('aria-current', 'true')
    button.addEventListener('click', () => options.onSelect?.(summary.canonical_project_key))
    item.append(button)
    list.append(item)
  }
  root.append(list)
  if (!selected) return root
  const totals = document.createElement('p')
  totals.textContent = `Active ${selected.totals.active} · Acknowledged ${selected.totals.acknowledged} · Pending ${selected.totals.pending}`
  root.append(totals)
  const release = document.createElement('button')
  release.type = 'button'
  release.dataset.quarantineRelease = ''
  release.disabled = options.pendingRelease === true
  release.textContent = options.pendingRelease ? 'Releasing…' : 'Release immediately'
  release.setAttribute('aria-label', `Release ${selected.project} immediately`)
  release.addEventListener('click', () => { void options.onRelease?.(selected) })
  root.append(release)
  const table = document.createElement('table')
  table.setAttribute('aria-label', 'Current generation convergence')
  const header = table.createTHead().insertRow()
  for (const label of ['Username', 'Current generation', 'Outcome', 'Acknowledged at']) {
    const cell = document.createElement('th')
    cell.scope = 'col'
    cell.textContent = label
    header.append(cell)
  }
  const normalizedFilter = options.filter.trim().toLowerCase()
  for (const row of selected.progress.filter((item) => item.username.toLowerCase().includes(normalizedFilter))) {
    const tableRow = table.insertRow()
    for (const value of [row.username, String(selected.generation), row.state === quarantineStates.PENDING ? 'No ACK received' : row.state, row.acknowledged_at ?? '—']) {
      const cell = tableRow.insertCell()
      cell.textContent = value
    }
  }
  root.append(table)
  if (selected.next_cursor) {
    const nextPage = document.createElement('button')
    nextPage.type = 'button'
    nextPage.dataset.quarantineNextPage = ''
    nextPage.textContent = 'Next page'
    nextPage.addEventListener('click', () => options.onNextPage?.())
    root.append(nextPage)
  }
  return root
}

function appendStatus(root: HTMLElement, value: string): HTMLElement {
  const message = document.createElement('p')
  message.setAttribute('role', 'status')
  message.textContent = value
  root.append(message)
  return root
}

function appendAlert(root: HTMLElement, value: string): HTMLElement {
  const message = document.createElement('p')
  message.setAttribute('role', 'alert')
  message.textContent = value
  root.append(message)
  return root
}

function messageFor(error: unknown): string {
  return error instanceof Error && error.message ? error.message : 'Unable to refresh quarantine progress.'
}

function errorStatus(error: unknown): number | undefined {
  return typeof error === 'object' && error !== null && 'status' in error && typeof error.status === 'number' ? error.status : undefined
}

function isAuthorizationError(error: unknown): boolean {
  return errorStatus(error) === 403
}

function isUnauthorizedError(error: unknown): boolean {
  return errorStatus(error) === 401
}

function isUnsupportedError(error: unknown): boolean {
  const status = errorStatus(error)
  return status === 404 || status === 405 || status === 501
}
