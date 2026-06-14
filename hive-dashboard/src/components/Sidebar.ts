import type { DashboardScreenKey, NavigationGroupViewModel } from '../domain/dashboard'

export type UserLevel = 'viewer' | 'member' | 'admin'

export type SidebarProps = {
  readonly groups: readonly NavigationGroupViewModel[]
  readonly currentPath: string
  readonly userLevel: UserLevel
  readonly onNavigate: (path: string) => void
  readonly onLogout: () => void
}

/**
 * Maps a DashboardScreenKey to its canonical path.
 * This is a lightweight forward-reference to the ROUTES table in main.ts,
 * kept here to avoid circular imports. Sidebar only needs the path lookup.
 */
const SCREEN_PATHS: Partial<Record<DashboardScreenKey, string>> = {
  overview: '/dashboard',
  memories: '/dashboard/memories',
  projects: '/dashboard/projects',
  knowledgeBrowser: '/dashboard/knowledgeBrowser',
  globalSearch: '/dashboard/globalSearch',
  knowledgeGraph: '/dashboard/knowledgeGraph',
  activityFeed: '/dashboard/activityFeed',
  contributors: '/dashboard/contributors',
  developerTimeline: '/dashboard/developerTimeline',
  syncStatus: '/dashboard/syncStatus',
  analytics: '/dashboard/analytics',
  userManagement: '/dashboard/userManagement',
  auditLog: '/dashboard/auditLog',
  conflictViewer: '/dashboard/conflictViewer'
}

export function renderSidebar(container: HTMLElement, props: SidebarProps): void {
  container.replaceChildren()

  const sidebar = document.createElement('aside')
  sidebar.className = 'dashboard-sidebar'
  sidebar.dataset.dashboardPrimitive = 'sidebar'

  const nav = document.createElement('nav')
  nav.setAttribute('aria-label', 'Dashboard sections')

  const visibleGroups = props.groups.filter(
    (group) => group.key !== 'governance' || props.userLevel === 'admin'
  )

  for (const group of visibleGroups) {
    const groupEl = document.createElement('div')
    groupEl.className = 'dashboard-nav-group'
    groupEl.dataset.navGroup = group.key

    const groupLabel = document.createElement('p')
    groupLabel.className = 'dashboard-nav-group__label eyebrow'
    groupLabel.textContent = group.label
    groupEl.append(groupLabel)

    for (const entry of group.entries) {
      const path = SCREEN_PATHS[entry.screen] ?? `/dashboard/${entry.screen}`
      const isActive = props.currentPath === path

      const entryEl = document.createElement('a')
      entryEl.href = path
      entryEl.className = isActive
        ? 'dashboard-nav-entry dashboard-nav-entry--active'
        : 'dashboard-nav-entry'
      entryEl.dataset.navEntry = entry.screen
      entryEl.textContent = entry.label
      entryEl.addEventListener('click', (event) => {
        event.preventDefault()
        props.onNavigate(path)
      })
      groupEl.append(entryEl)
    }

    nav.append(groupEl)
  }

  sidebar.append(nav)

  const logoutButton = document.createElement('button')
  logoutButton.type = 'button'
  logoutButton.className = 'dashboard-control control dashboard-sidebar__logout'
  logoutButton.dataset.dashboardPrimitive = 'control'
  logoutButton.dataset.sidebarAction = 'logout'
  logoutButton.textContent = 'Sign out'
  logoutButton.addEventListener('click', () => props.onLogout())
  sidebar.append(logoutButton)

  container.append(sidebar)
}
