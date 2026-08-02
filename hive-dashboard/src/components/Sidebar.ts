import type { CurrentProfileViewModel, DashboardScreenKey, NavigationGroupViewModel } from '../domain/dashboard'
import { renderBrand } from './Brand'

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
  activityFeed: '/dashboard/activityFeed',
  userManagement: '/dashboard/userManagement',
  auditLog: '/dashboard/auditLog',
  quarantines: '/dashboard/quarantines'
}

const HIDDEN_NAV_SCREENS = new Set<DashboardScreenKey>([
  'memories',
  'knowledgeGraph',
  'contributors',
  'developerTimeline',
  'syncStatus',
  'analytics',
  'conflictViewer'
])

/**
 * Returns an SVG string for a given nav screen id.
 * Each icon uses viewBox="0 0 20 20", 16×16 rendered, stroke-based design.
 */
export function navIconForScreen(screenId: DashboardScreenKey, active: boolean): string {
  const color = active ? '#9DC2F6' : '#6b7686'
  const baseAttrs = `viewBox="0 0 20 20" fill="none" stroke="${color}" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" width="16" height="16" aria-hidden="true" focusable="false"`

  const iconShapes: Partial<Record<DashboardScreenKey, string>> = {
    overview: `<rect x="2.5" y="2.5" width="6" height="6"/><rect x="11.5" y="2.5" width="6" height="6"/><rect x="2.5" y="11.5" width="6" height="6"/><rect x="11.5" y="11.5" width="6" height="6"/>`,
    projects: `<ellipse cx="10" cy="4.5" rx="6.5" ry="2.4"/><path d="M3.5 4.5v11c0 1.3 2.9 2.4 6.5 2.4s6.5-1.1 6.5-2.4v-11"/><path d="M3.5 10c0 1.3 2.9 2.4 6.5 2.4s6.5-1.1 6.5-2.4"/>`,
    memories: `<rect x="3" y="2.5" width="14" height="15" rx="1"/><line x1="6" y1="6.5" x2="14" y2="6.5"/><line x1="6" y1="10" x2="14" y2="10"/><line x1="6" y1="13.5" x2="11" y2="13.5"/>`,
    knowledgeBrowser: `<rect x="3" y="2.5" width="14" height="15" rx="1"/><line x1="6" y1="6.5" x2="14" y2="6.5"/><line x1="6" y1="10" x2="14" y2="10"/><line x1="6" y1="13.5" x2="11" y2="13.5"/>`,
    activityFeed: `<path d="M2.5 10h3l2.5-6 3.5 12 2.5-6h3.5"/>`,
    userManagement: `<path d="M10 2.5l6 2.2v4.6c0 4-2.6 6.6-6 7.7-3.4-1.1-6-3.7-6-7.7V4.7z"/><path d="M7.5 10l1.8 1.8L13 8"/>`,
    auditLog: `<rect x="4" y="3" width="12" height="14" rx="1"/><line x1="7" y1="7" x2="13" y2="7"/><line x1="7" y1="10" x2="13" y2="10"/><line x1="7" y1="13" x2="11" y2="13"/>`,
    quarantines: `<path d="M10 2.5l6 2.2v4.6c0 4-2.6 6.6-6 7.7-3.4-1.1-6-3.7-6-7.7V4.7z"/><path d="M7 10h6M10 7v6"/>`
  }

  const shapes = iconShapes[screenId] ?? `<circle cx="10" cy="10" r="5"/>`
  return `<svg class="dashboard-nav-icon" ${baseAttrs}>${shapes}</svg>`
}

export function renderSidebar(container: HTMLElement, props: SidebarProps): void {
  container.replaceChildren()

  const sidebar = document.createElement('aside')
  sidebar.className = 'dashboard-sidebar'
  sidebar.dataset.dashboardPrimitive = 'sidebar'

  // Brand block — emblem + wordmark lives exclusively in the sidebar
  const brandEl = document.createElement('div')
  brandEl.className = 'dashboard-sidebar__brand'
  brandEl.innerHTML = renderBrand()
  sidebar.append(brandEl)

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
    groupLabel.className = 'dashboard-nav-group__label'
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

      // Icon
      const iconWrapper = document.createElement('span')
      iconWrapper.className = 'dashboard-nav-entry__icon'
      iconWrapper.innerHTML = navIconForScreen(entry.screen, isActive)
      entryEl.append(iconWrapper)

      // Label
      const labelEl = document.createElement('span')
      labelEl.className = 'dashboard-nav-entry__label'
      labelEl.textContent = entry.label
      entryEl.append(labelEl)

      entryEl.addEventListener('click', (event) => {
        event.preventDefault()
        props.onNavigate(path)
      })
      groupEl.append(entryEl)
    }

    nav.append(groupEl)
  }

  sidebar.append(nav)

  renderProfileBlock(sidebar, {
    currentPath: props.currentPath,
    profile: props.profile,
    onLogout: props.onLogout,
    onNavigate: props.onNavigate
  })

  container.append(sidebar)
}

type ProfileBlockProps = {
  readonly currentPath: string
  readonly profile: CurrentProfileViewModel
  readonly onLogout: () => void
  readonly onNavigate: (path: string) => void
}

function renderProfileBlock(container: HTMLElement, props: ProfileBlockProps): void {
  const block = document.createElement('div')
  block.className = 'dashboard-sidebar__profile'
  block.dataset.sidebarProfile = ''

  // Profile row: avatar + name/email container
  const profileRow = document.createElement('div')
  profileRow.className = 'dashboard-sidebar__profile-row'
  profileRow.dataset.sidebarProfileRow = ''

  // Initials avatar
  const avatar = document.createElement('div')
  avatar.className = 'dashboard-sidebar__profile-avatar'
  avatar.dataset.userRole = props.profile.role
  avatar.setAttribute('aria-hidden', 'true')
  avatar.textContent = props.profile.initials

  // Name + email column
  const textCol = document.createElement('div')
  textCol.className = 'dashboard-sidebar__profile-text'

  const identity = document.createElement('div')
  identity.className = 'dashboard-sidebar__profile-identity'

  const name = document.createElement('p')
  name.className = 'dashboard-sidebar__profile-name'
  name.textContent = props.profile.name

  const rolePill = document.createElement('span')
  rolePill.className = 'dashboard-sidebar__role-pill'
  rolePill.dataset.sidebarRolePill = ''
  rolePill.dataset.userRole = props.profile.role
  rolePill.textContent = props.profile.role
  identity.append(name, rolePill)

  const email = document.createElement('p')
  email.className = 'dashboard-sidebar__profile-email'
  email.textContent = props.profile.email

  textCol.append(identity, email)
  profileRow.append(avatar, textCol)

  const actionsRow = document.createElement('div')
  actionsRow.className = 'dashboard-sidebar__profile-actions'

  const accountLink = document.createElement('a')
  accountLink.href = '/dashboard/account'
  accountLink.className = 'dashboard-sidebar__account'
  accountLink.dataset.sidebarAction = 'account'
  accountLink.setAttribute('aria-label', 'Account')
  if (props.currentPath === '/dashboard/account') {
    accountLink.classList.add('dashboard-sidebar__account--active')
    accountLink.setAttribute('aria-current', 'page')
  }

  const accountIcon = document.createElement('span')
  accountIcon.className = 'dashboard-sidebar__account-icon'
  accountIcon.setAttribute('aria-hidden', 'true')
  accountIcon.innerHTML = '<svg viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.4"><circle cx="8" cy="5" r="2.5"/><path d="M3.5 13c.5-2.4 2-3.6 4.5-3.6s4 1.2 4.5 3.6"/></svg>'
  const accountLabel = document.createElement('span')
  accountLabel.textContent = 'Account'
  accountLink.append(accountIcon, accountLabel)
  accountLink.addEventListener('click', (event) => {
    event.preventDefault()
    props.onNavigate('/dashboard/account')
  })
  const logoutButton = document.createElement('button')
  logoutButton.type = 'button'
  logoutButton.className = 'dashboard-sidebar__logout'
  logoutButton.dataset.sidebarAction = 'logout'
  logoutButton.textContent = props.profile.logoutLabel
  logoutButton.addEventListener('click', () => {
    props.onLogout()
  })

  actionsRow.append(accountLink, logoutButton)
  block.append(profileRow, actionsRow)

  container.append(block)
}
