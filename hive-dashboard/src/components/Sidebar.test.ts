import { describe, expect, it, vi } from 'vitest'
import type { UserLevel } from '../components/Sidebar'
import { renderSidebar } from '../components/Sidebar'
import { currentDashboardProfile, dashboardNavigationGroups } from '../fixtures/hive-dashboard/shared'

const baseProps = {
  groups: dashboardNavigationGroups,
  currentPath: '/dashboard',
  userLevel: 'admin' as UserLevel,
  profile: currentDashboardProfile,
  onNavigate: vi.fn(),
  onLogout: vi.fn()
}

describe('Sidebar', () => {
  it('renders only visible real-route nav groups for a member user', () => {
    const container = document.createElement('div')

    renderSidebar(container, { ...baseProps, userLevel: 'member' })

    const nav = container.querySelector('nav[aria-label="Dashboard sections"]')
    expect(nav).not.toBeNull()
    expect(nav?.textContent).toContain('Explore')
    expect(nav?.textContent).not.toContain('Memories')
    expect(nav?.textContent).not.toContain('Global Search')
    expect(nav?.textContent).not.toContain('Team')
    expect(nav?.textContent).not.toContain('Insights')
    expect(nav?.textContent).not.toContain('Knowledge Graph')
    expect(nav?.textContent).not.toContain('Contributors')
    expect(nav?.textContent).not.toContain('Developer Timeline')
    expect(nav?.textContent).not.toContain('Sync Status')
    expect(nav?.textContent).not.toContain('Analytics')
    expect(nav?.textContent).not.toContain('Conflict Viewer')
  })

  it('hides the Governance group for member users', () => {
    const container = document.createElement('div')

    renderSidebar(container, { ...baseProps, userLevel: 'member' })

    expect(container.textContent).not.toContain('Governance')
  })

  it('hides the Governance group for viewer users', () => {
    const container = document.createElement('div')

    renderSidebar(container, { ...baseProps, userLevel: 'viewer' })

    expect(container.textContent).not.toContain('Governance')
  })

  it('shows the Governance group for admin users', () => {
    const container = document.createElement('div')

    renderSidebar(container, { ...baseProps, userLevel: 'admin' })

    expect(container.textContent).toContain('Governance')
    expect(container.textContent).toContain('User Management')
    expect(container.textContent).toContain('Audit Log')
  })

  it('marks the active entry matching the current path', () => {
    const container = document.createElement('div')

    renderSidebar(container, { ...baseProps, currentPath: '/dashboard/activityFeed' })

    const activeEntry = container.querySelector('.dashboard-nav-entry--active')
    expect(activeEntry).not.toBeNull()
    expect(activeEntry?.textContent).toContain('Activity Feed')
  })

  it('does not mark other entries as active', () => {
    const container = document.createElement('div')

    renderSidebar(container, { ...baseProps, currentPath: '/dashboard/activityFeed' })

    const allActive = container.querySelectorAll('.dashboard-nav-entry--active')
    expect(allActive.length).toBe(1)
  })

  it('does not render groups that only contain hidden Coming Soon entries', () => {
    const container = document.createElement('div')

    renderSidebar(container, { ...baseProps, userLevel: 'admin' })

    expect([...container.querySelectorAll('[data-nav-group]')].map((group) => group.getAttribute('data-nav-group'))).toEqual(['explore', 'governance'])
    expect(container.querySelector('[data-nav-entry="memories"]')).toBeNull()
    expect(container.querySelector('[data-nav-entry="globalSearch"]')).toBeNull()
  })

  it('calls onLogout exactly once when the logout button is clicked', () => {
    const container = document.createElement('div')
    const onLogout = vi.fn()

    renderSidebar(container, { ...baseProps, onLogout })

    const logoutButton = container.querySelector('[data-sidebar-action="logout"]')
    expect(logoutButton).not.toBeNull()
    logoutButton!.dispatchEvent(new MouseEvent('click', { bubbles: true }))
    expect(onLogout).toHaveBeenCalledTimes(1)
  })
})

describe('Sidebar profile block', () => {
  it.each(['admin', 'member', 'viewer'] as const)('keeps the %s role token aligned on the avatar and compact role pill', (role) => {
    const container = document.createElement('div')
    const profile = {
      initials: 'A',
      name: 'alice',
      email: 'alice@example.com',
      role,
      logoutLabel: 'Sign out'
    }

    renderSidebar(container, { ...baseProps, userLevel: role, profile, onLogout: vi.fn() })

    const profileBlock = container.querySelector('[data-sidebar-profile]')
    const avatar = container.querySelector('.dashboard-sidebar__profile-avatar')
    const rolePill = container.querySelector('[data-sidebar-role-pill]')
    expect(profileBlock).not.toBeNull()
    expect(avatar?.getAttribute('data-user-role')).toBe(role)
    expect(rolePill?.getAttribute('data-user-role')).toBe(role)
    expect(rolePill?.getAttribute('style')).toBeNull()
    expect(profileBlock?.textContent).toContain('A')
    expect(profileBlock?.textContent).toContain('alice@example.com')
    expect(profileBlock?.textContent).toContain(role)
  })

  it('shows display name when provided instead of raw email prefix', () => {
    const container = document.createElement('div')
    const profile = {
      initials: 'AS',
      name: 'Alice Smith',
      email: 'alice@example.com',
      role: 'member' as const,
      logoutLabel: 'Sign out'
    }

    renderSidebar(container, { ...baseProps, userLevel: 'member', profile, onLogout: vi.fn() })

    const profileBlock = container.querySelector('[data-sidebar-profile]')
    expect(profileBlock?.textContent).toContain('Alice Smith')
  })

  it('logout control in profile block triggers onLogout exactly once', () => {
    const container = document.createElement('div')
    const onLogout = vi.fn()

    renderSidebar(container, { ...baseProps, profile: currentDashboardProfile, onLogout })

    const logoutControl = container.querySelector('[data-sidebar-action="logout"]')
    expect(logoutControl).not.toBeNull()
    logoutControl!.dispatchEvent(new MouseEvent('click', { bubbles: true }))
    expect(onLogout).toHaveBeenCalledTimes(1)
  })

  it('renders role beside the name and keeps the email truncation hook', () => {
    const container = document.createElement('div')
    renderSidebar(container, { ...baseProps })

    const identity = container.querySelector('.dashboard-sidebar__profile-identity')
    expect(identity?.querySelector('.dashboard-sidebar__profile-name')).not.toBeNull()
    expect(identity?.querySelector('[data-sidebar-role-pill]')).not.toBeNull()
    expect(container.querySelector('.dashboard-sidebar__profile-email')).not.toBeNull()
  })
})

describe('Sidebar nav icons', () => {
  it('each visible nav entry renders an svg icon', () => {
    const container = document.createElement('div')
    renderSidebar(container, { ...baseProps, userLevel: 'admin' })

    const navEntries = container.querySelectorAll('.dashboard-nav-entry')
    expect(navEntries.length).toBeGreaterThan(0)

    for (const entry of navEntries) {
      const icon = entry.querySelector('svg')
      expect(icon).not.toBeNull()
    }
  })

  it('overview nav entry renders a grid icon (4 rects)', () => {
    const container = document.createElement('div')
    renderSidebar(container, { ...baseProps })

    const overviewEntry = container.querySelector('[data-nav-entry="overview"]')
    expect(overviewEntry).not.toBeNull()
    const rects = overviewEntry?.querySelectorAll('svg rect')
    expect(rects?.length).toBe(4)
  })

  it('projects nav entry renders a db icon (ellipse + paths)', () => {
    const container = document.createElement('div')
    renderSidebar(container, { ...baseProps })

    const projectsEntry = container.querySelector('[data-nav-entry="projects"]')
    expect(projectsEntry).not.toBeNull()
    const ellipse = projectsEntry?.querySelector('svg ellipse')
    expect(ellipse).not.toBeNull()
  })

  it('activityFeed nav entry renders an activity icon (path)', () => {
    const container = document.createElement('div')
    renderSidebar(container, { ...baseProps })

    const activityEntry = container.querySelector('[data-nav-entry="activityFeed"]')
    expect(activityEntry).not.toBeNull()
    const path = activityEntry?.querySelector('svg path')
    expect(path).not.toBeNull()
  })

  it('userManagement nav entry renders a shield icon (admin only)', () => {
    const container = document.createElement('div')
    renderSidebar(container, { ...baseProps, userLevel: 'admin' })

    const userMgmtEntry = container.querySelector('[data-nav-entry="userManagement"]')
    expect(userMgmtEntry).not.toBeNull()
    const svg = userMgmtEntry?.querySelector('svg')
    expect(svg).not.toBeNull()
  })
})

describe('Sidebar profile actions', () => {
  it('renders a role pill with the user role text', () => {
    const container = document.createElement('div')
    renderSidebar(container, { ...baseProps, userLevel: 'admin' })

    const rolePill = container.querySelector('[data-sidebar-role-pill]')
    expect(rolePill).not.toBeNull()
    expect(rolePill?.textContent).toContain('admin')
  })

  it('renders Account as navigation and Logout as a button action', () => {
    const container = document.createElement('div')
    renderSidebar(container, { ...baseProps })

    const accountControl = container.querySelector('[data-sidebar-action="account"]')
    const logoutControl = container.querySelector('[data-sidebar-action="logout"]')
    expect(accountControl?.tagName.toLowerCase()).toBe('a')
    expect(accountControl?.getAttribute('href')).toBe('/dashboard/account')
    expect(logoutControl).not.toBeNull()
    expect(logoutControl?.tagName.toLowerCase()).toBe('button')
    expect(logoutControl?.getAttribute('type')).toBe('button')
    expect(logoutControl?.getAttribute('href')).toBeNull()
  })

  it('renders only Account and Logout in the lower action row', () => {
    const container = document.createElement('div')
    renderSidebar(container, { ...baseProps })

    const actions = container.querySelector('.dashboard-sidebar__profile-actions')
    expect(actions?.children).toHaveLength(2)
    expect(actions?.querySelector('[data-sidebar-role-pill]')).toBeNull()
  })

  it('marks Account active only on the account route and navigates once', () => {
    const container = document.createElement('div')
    const onNavigate = vi.fn()
    renderSidebar(container, { ...baseProps, currentPath: '/dashboard/account', onNavigate })

    const accountControl = container.querySelector<HTMLAnchorElement>('[data-sidebar-action="account"]')
    expect(accountControl?.classList.contains('dashboard-sidebar__account--active')).toBe(true)
    expect(accountControl?.getAttribute('aria-current')).toBe('page')
    accountControl?.click()
    expect(onNavigate).toHaveBeenCalledTimes(1)
    expect(onNavigate).toHaveBeenCalledWith('/dashboard/account')
  })

  it('profile row contains hex avatar, name, and email', () => {
    const container = document.createElement('div')
    renderSidebar(container, {
      ...baseProps,
      profile: {
        initials: 'AO',
        name: 'Ada Okafor',
        email: 'ada.okafor@nexus.dev',
        role: 'admin',
        logoutLabel: 'Logout'
      }
    })

    const profileRow = container.querySelector('[data-sidebar-profile-row]')
    expect(profileRow).not.toBeNull()
    expect(profileRow?.textContent).toContain('AO')
    expect(profileRow?.textContent).toContain('Ada Okafor')
    expect(profileRow?.textContent).toContain('ada.okafor@nexus.dev')
  })
})
