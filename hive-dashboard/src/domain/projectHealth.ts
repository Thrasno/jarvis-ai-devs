import type { ProjectSyncStatus } from './dashboard'

export type ProjectHealthInput = {
  readonly status?: ProjectSyncStatus | null
  readonly memoryCount: number
  readonly lastMemoryAt?: string | null
}

type ProjectSummarySortable = ProjectHealthInput & {
  readonly name: string
}

const staleMemoryThresholdYears = 2
const healthPrecedence: Record<ProjectSyncStatus, number> = {
  degraded: 0,
  unknown: 1,
  healthy: 2
}

export function deriveProjectHealth(input: ProjectHealthInput, evaluationDate = new Date()): ProjectSyncStatus {
  if (isStaleForAtLeastTwoYears(input.lastMemoryAt, evaluationDate)) {
    return 'degraded'
  }

  if (input.status === 'healthy' || input.status === 'degraded' || input.status === 'unknown') {
    return input.status
  }

  return 'unknown'
}

export function sortProjectSummaries<TProject extends ProjectSummarySortable>(
  projects: readonly TProject[],
  evaluationDate = new Date()
): TProject[] {
  return [...projects].sort((left, right) => {
    const leftHealth = deriveProjectHealth(left, evaluationDate)
    const rightHealth = deriveProjectHealth(right, evaluationDate)
    const healthComparison = healthPrecedence[leftHealth] - healthPrecedence[rightHealth]

    if (healthComparison !== 0) {
      return healthComparison
    }

    return left.name.localeCompare(right.name, undefined, { sensitivity: 'accent' })
  })
}

function isStaleForAtLeastTwoYears(lastMemoryAt: string | null | undefined, evaluationDate: Date): boolean {
  if (!lastMemoryAt) {
    return false
  }

  const lastMemoryDate = new Date(lastMemoryAt)

  if (Number.isNaN(lastMemoryDate.getTime()) || Number.isNaN(evaluationDate.getTime())) {
    return false
  }

  const staleThreshold = new Date(evaluationDate)
  staleThreshold.setFullYear(staleThreshold.getFullYear() - staleMemoryThresholdYears)

  return lastMemoryDate <= staleThreshold
}
