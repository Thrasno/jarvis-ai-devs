type OptionalString = string | null | undefined
type OptionalNumber = number | null | undefined

export type DashboardUrlFilters = {
  query?: OptionalString
  project?: OptionalString
  category?: OptionalString
  developer?: OptionalString
  author?: OptionalString
  actorUserId?: OptionalString
  actor_user_id?: OptionalString
  window?: OptionalString
  dev_id?: OptionalString
  from?: OptionalString
  since?: OptionalString
  until?: OptionalString
  tags?: readonly OptionalString[] | null
  tag?: readonly OptionalString[] | OptionalString
  status?: OptionalString
  action?: OptionalString
  outcome?: OptionalString
  client?: OptionalString
  daemon_id?: OptionalString
  error_code?: OptionalString
  health?: OptionalString
  limit?: OptionalNumber
  offset?: OptionalNumber
}

export type MemoryDiscoveryUrlFilters = Pick<DashboardUrlFilters, 'query' | 'project' | 'category' | 'from' | 'until' | 'limit' | 'offset'>

type ScalarFilterKey = Exclude<keyof DashboardUrlFilters, 'tags' | 'tag' | 'actorUserId'>

const scalarFilters: readonly { key: ScalarFilterKey; param: string }[] = [
  { key: 'window', param: 'window' },
  { key: 'query', param: 'query' },
  { key: 'project', param: 'project' },
  { key: 'dev_id', param: 'dev_id' },
  { key: 'category', param: 'category' },
  { key: 'developer', param: 'developer' },
  { key: 'author', param: 'author' },
  { key: 'actor_user_id', param: 'actor_user_id' },
  { key: 'from', param: 'from' },
  { key: 'since', param: 'since' },
  { key: 'until', param: 'until' },
  { key: 'status', param: 'status' },
  { key: 'action', param: 'action' },
  { key: 'outcome', param: 'outcome' },
  { key: 'client', param: 'client' },
  { key: 'daemon_id', param: 'daemon_id' },
  { key: 'error_code', param: 'error_code' },
  { key: 'health', param: 'health' },
  { key: 'limit', param: 'limit' },
  { key: 'offset', param: 'offset' }
]
const tagInsertAfter = 'until'

export function serializeDashboardFilters(filters: DashboardUrlFilters): string {
  const params = new URLSearchParams()
  for (const { key, param } of scalarFilters) {
    const value = key === 'actor_user_id' ? filters.actorUserId ?? filters.actor_user_id : filters[key]
    appendScalar(params, param, value)
    if (key === tagInsertAfter) {
      for (const tag of tagsFor(filters)) appendScalar(params, 'tag', tag)
    }
  }
  return params.toString()
}

export function parseDashboardFilters(input: string | URLSearchParams): DashboardUrlFilters {
  const params = typeof input === 'string' ? new URLSearchParams(input.startsWith('?') ? input.slice(1) : input) : input
  const filters: DashboardUrlFilters = {}
  setString(filters, 'query', first(params, 'query'))
  setString(filters, 'project', first(params, 'project'))
  setString(filters, 'window', first(params, 'window'))
  setString(filters, 'dev_id', first(params, 'dev_id'))
  setString(filters, 'category', first(params, 'category'))
  setString(filters, 'developer', first(params, 'developer'))
  setString(filters, 'author', first(params, 'author'))
  setString(filters, 'actor_user_id', first(params, 'actor_user_id'))
  setString(filters, 'from', first(params, 'from'))
  setString(filters, 'since', first(params, 'since'))
  setString(filters, 'until', first(params, 'until'))
  setString(filters, 'status', first(params, 'status'))
  setString(filters, 'action', first(params, 'action'))
  setString(filters, 'outcome', first(params, 'outcome'))
  setString(filters, 'client', first(params, 'client'))
  setString(filters, 'daemon_id', first(params, 'daemon_id'))
  setString(filters, 'error_code', first(params, 'error_code'))
  setString(filters, 'health', first(params, 'health'))
  setNumber(filters, 'limit', first(params, 'limit'))
  setNumber(filters, 'offset', first(params, 'offset'))

  const tags = params.getAll('tag').filter((value) => value.trim() !== '')
  if (tags.length > 0) filters.tags = tags
  return filters
}

export function appendDashboardFilters(path: string, filters: DashboardUrlFilters): string {
  const query = serializeDashboardFilters(filters)
  if (!query) return path
  return `${path}${path.includes('?') ? '&' : '?'}${query}`
}

function appendScalar(params: URLSearchParams, key: string, value: OptionalString | OptionalNumber): void {
  if (typeof value === 'number') {
    if (Number.isInteger(value) && minValueFor(key) <= value) params.append(key, String(value))
    return
  }
  if (value === undefined || value === null || value.trim() === '') return
  params.append(key, value)
}

function first(params: URLSearchParams, key: string): string | null {
  const [value] = params.getAll(key)
  return value ?? null
}

function setString<T extends keyof DashboardUrlFilters>(filters: DashboardUrlFilters, key: T, value: string | null): void {
  if (value !== null && value.trim() !== '') filters[key] = value as DashboardUrlFilters[T]
}

function setNumber<T extends 'limit' | 'offset'>(filters: DashboardUrlFilters, key: T, value: string | null): void {
  if (value === null || value.trim() === '') return
  const parsed = Number(value)
  if (Number.isInteger(parsed) && minValueFor(key) <= parsed) filters[key] = parsed
}

function minValueFor(key: string): number {
  return key === 'limit' ? 1 : 0
}

function tagsFor(filters: DashboardUrlFilters): readonly OptionalString[] {
  if (Array.isArray(filters.tags)) return filters.tags
  const tag = filters.tag
  if (Array.isArray(tag)) return tag as readonly OptionalString[]
  if (typeof tag === 'string') return [tag]
  return []
}
