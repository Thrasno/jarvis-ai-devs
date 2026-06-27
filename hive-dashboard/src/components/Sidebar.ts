import type { CurrentProfileViewModel, DashboardScreenKey, NavigationGroupViewModel } from '../domain/dashboard'

export type UserLevel = 'viewer' | 'member' | 'admin'

export type SidebarProps = {
  readonly groups: readonly NavigationGroupViewModel[]
  readonly currentPath: string
  readonly userLevel: UserLevel
  readonly profile: CurrentProfileViewModel
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
  activityFeed: '/dashboard/activityFeed',
  userManagement: '/dashboard/userManagement',
  auditLog: '/dashboard/auditLog'
}

const HIDDEN_NAV_SCREENS = new Set<DashboardScreenKey>([
  'knowledgeGraph',
  'contributors',
  'developerTimeline',
  'syncStatus',
  'analytics',
  'conflictViewer'
])

export function renderSidebar(container: HTMLElement, props: SidebarProps): void {
  container.replaceChildren()

  const sidebar = document.createElement('aside')
  sidebar.className = 'dashboard-sidebar'
  sidebar.dataset.dashboardPrimitive = 'sidebar'

  const nav = document.createElement('nav')
  nav.setAttribute('aria-label', 'Dashboard sections')

  const visibleGroups = props.groups
    .filter((group) => group.key !== 'governance' || props.userLevel === 'admin')
    .map((group) => ({
      ...group,
      entries: group.entries.filter((entry) => !HIDDEN_NAV_SCREENS.has(entry.screen))
    }))
    .filter((group) => group.entries.length > 0)

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

  renderProfileBlock(sidebar, { profile: props.profile, onLogout: props.onLogout })

  container.append(sidebar)
}

type ProfileBlockProps = {
  readonly profile: CurrentProfileViewModel
  readonly onLogout: () => void
}

function renderProfileBlock(container: HTMLElement, props: ProfileBlockProps): void {
  const block = document.createElement('div')
  block.className = 'dashboard-sidebar__profile'
  block.dataset.sidebarProfile = ''

  // Initials avatar
  const avatar = document.createElement('div')
  avatar.className = 'dashboard-sidebar__profile-avatar'
  avatar.setAttribute('aria-hidden', 'true')
  avatar.textContent = props.profile.initials

  // Name
  const name = document.createElement('p')
  name.className = 'dashboard-sidebar__profile-name'
  name.textContent = props.profile.name

  // Email
  const email = document.createElement('p')
  email.className = 'dashboard-sidebar__profile-email'
  email.textContent = props.profile.email

  // Role badge
  const roleBadge = document.createElement('span')
  roleBadge.className = 'dashboard-status status'
  roleBadge.dataset.dashboardStatus = 'neutral'
  roleBadge.textContent = props.profile.role

  // Logout button
  const logoutButton = document.createElement('button')
  logoutButton.type = 'button'
  logoutButton.className = 'dashboard-control control dashboard-sidebar__logout'
  logoutButton.dataset.dashboardPrimitive = 'control'
  logoutButton.dataset.sidebarAction = 'logout'
  logoutButton.textContent = props.profile.logoutLabel
  logoutButton.addEventListener('click', () => props.onLogout())

  block.append(avatar, name, email, roleBadge, logoutButton)
  container.append(block)
}
