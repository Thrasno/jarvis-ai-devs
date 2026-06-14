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
  it('renders Explore, Team, and Insights nav groups for a member user', () => {
    const container = document.createElement('div')

    renderSidebar(container, { ...baseProps, userLevel: 'member' })

    const nav = container.querySelector('nav[aria-label="Dashboard sections"]')
    expect(nav).not.toBeNull()
    expect(nav?.textContent).toContain('Explore')
    expect(nav?.textContent).toContain('Team')
    expect(nav?.textContent).toContain('Insights')
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
  it('shows initials avatar, email, and role badge for a member user', () => {
    const container = document.createElement('div')
    const profile = {
      initials: 'A',
      name: 'alice',
      email: 'alice@example.com',
      role: 'member' as const,
      logoutLabel: 'Sign out'
    }

    renderSidebar(container, { ...baseProps, userLevel: 'member', profile, onLogout: vi.fn() })

    const profileBlock = container.querySelector('[data-sidebar-profile]')
    expect(profileBlock).not.toBeNull()
    expect(profileBlock?.textContent).toContain('A')
    expect(profileBlock?.textContent).toContain('alice@example.com')
    expect(profileBlock?.textContent).toContain('member')
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

  it('logout button in profile block triggers onLogout exactly once', () => {
    const container = document.createElement('div')
    const onLogout = vi.fn()

    renderSidebar(container, { ...baseProps, profile: currentDashboardProfile, onLogout })

    const logoutButton = container.querySelector('[data-sidebar-profile] button')
    expect(logoutButton).not.toBeNull()
    logoutButton!.dispatchEvent(new MouseEvent('click', { bubbles: true }))
    expect(onLogout).toHaveBeenCalledTimes(1)
  })
})
