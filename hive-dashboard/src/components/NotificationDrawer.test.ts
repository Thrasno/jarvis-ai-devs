import { describe, expect, it, vi } from 'vitest'
import { renderNotificationDrawer } from './NotificationDrawer'
import { dashboardNotifications, dashboardNotificationSummary } from '../fixtures/hive-dashboard/shared'

// 3 unread (gateway-auth-boundary, vector-store-single-writer, redis-maxmemory-policy)
// 4 read   (split-ingest-worker-gateway, token-refresh-cold-start, local-first-crdt-reconnect, vector-dimension-pinned)
const allReadIds = new Set(dashboardNotifications.map((n) => n.id))
const noReadIds = new Set<string>()

function baseProps(overrides: Partial<Parameters<typeof renderNotificationDrawer>[1]> = {}) {
  return {
    notifications: dashboardNotifications,
    summary: dashboardNotificationSummary,
    readIds: noReadIds,
    onMarkAllRead: vi.fn(),
    onClose: vi.fn(),
    ...overrides
  }
}

describe('NotificationDrawer', () => {
  it('renders all notification cards with correct total count', () => {
    const container = document.createElement('div')

    renderNotificationDrawer(container, baseProps())

    const cards = container.querySelectorAll('[data-notification-card]')
    expect(cards.length).toBe(dashboardNotifications.length)
  })

  it('shows unread count in header counter', () => {
    const container = document.createElement('div')

    renderNotificationDrawer(container, baseProps())

    const counter = container.querySelector('[data-notification-counter]')
    expect(counter).not.toBeNull()
    // summary is { unread: 3, total: 7 }
    expect(counter?.textContent).toContain('3')
    expect(counter?.textContent).toContain('7')
  })

  it('unread cards have unread indicator; read cards do not', () => {
    const container = document.createElement('div')
    // mark all as read
    renderNotificationDrawer(container, baseProps({ readIds: allReadIds }))

    const indicators = container.querySelectorAll('[data-unread-indicator]')
    expect(indicators.length).toBe(0)

    // now render with no read ids — all 7 notifications show as unread
    const container2 = document.createElement('div')
    renderNotificationDrawer(container2, baseProps({ readIds: noReadIds }))

    const indicators2 = container2.querySelectorAll('[data-unread-indicator]')
    // notifications fixture: 3 have unread:true initially, but drawer uses readIds set
    // with noReadIds (empty set) all entries that have unread:true in the fixture are unread
    const fixtureUnreadCount = dashboardNotifications.filter((n) => n.unread).length
    expect(indicators2.length).toBe(fixtureUnreadCount)
  })

  it('"Mark all read" fires onMarkAllRead callback once', () => {
    const container = document.createElement('div')
    const onMarkAllRead = vi.fn()

    renderNotificationDrawer(container, baseProps({ onMarkAllRead }))

    const btn = container.querySelector('[data-mark-all-read]')
    expect(btn).not.toBeNull()
    btn!.dispatchEvent(new MouseEvent('click', { bubbles: true }))
    expect(onMarkAllRead).toHaveBeenCalledTimes(1)
  })

  it('close button fires onClose callback once', () => {
    const container = document.createElement('div')
    const onClose = vi.fn()

    renderNotificationDrawer(container, baseProps({ onClose }))

    const btn = container.querySelector('[data-notification-close]')
    expect(btn).not.toBeNull()
    btn!.dispatchEvent(new MouseEvent('click', { bubbles: true }))
    expect(onClose).toHaveBeenCalledTimes(1)
  })

  it('Escape closes the open drawer through the existing close callback', () => {
    const container = document.createElement('div')
    const onClose = vi.fn()

    renderNotificationDrawer(container, baseProps({ open: true, onClose }))

    const drawer = container.querySelector('[data-dashboard-primitive="drawer"]')
    expect(drawer).not.toBeNull()

    const event = new KeyboardEvent('keydown', { key: 'Escape', bubbles: true, cancelable: true })
    drawer!.dispatchEvent(event)

    expect(event.defaultPrevented).toBe(true)
    expect(onClose).toHaveBeenCalledTimes(1)
  })

  it('shows a friendly message when notifications list is empty', () => {
    const container = document.createElement('div')

    renderNotificationDrawer(container, baseProps({
      notifications: [],
      summary: { unread: 0, total: 0 }
    }))

    const emptyState = container.querySelector('[data-notification-empty]')
    expect(emptyState).not.toBeNull()
    expect(emptyState?.textContent?.length).toBeGreaterThan(0)
  })

  it('each notification card shows title, category badge, actor handle, project, and time label', () => {
    const container = document.createElement('div')

    renderNotificationDrawer(container, baseProps())

    const firstCard = container.querySelector('[data-notification-card]')
    expect(firstCard).not.toBeNull()
    const first = dashboardNotifications[0]
    expect(firstCard?.textContent).toContain(first.title)
    expect(firstCard?.textContent).toContain(first.category)
    expect(firstCard?.textContent).toContain(first.actorHandle)
    expect(firstCard?.textContent).toContain(first.projectName)
    expect(firstCard?.textContent).toContain(first.timeLabel)
  })

  it('drawer header title reads "Notifications"', () => {
    const container = document.createElement('div')

    renderNotificationDrawer(container, baseProps())

    const title = container.querySelector('.dashboard-drawer__title')
    expect(title).not.toBeNull()
    expect(title?.textContent).toBe('Notifications')
  })

  it('keeps a closed drawer out of the active modal/accessibility state', () => {
    const container = document.createElement('div')

    renderNotificationDrawer(container, baseProps({ open: false }))

    const drawer = container.querySelector('[data-dashboard-primitive="drawer"]')
    expect(drawer).not.toBeNull()
    expect(drawer?.hasAttribute('hidden')).toBe(true)
    expect(drawer?.hasAttribute('inert')).toBe(true)
    expect(drawer?.getAttribute('aria-hidden')).toBe('true')
    expect(drawer?.getAttribute('role')).toBeNull()
    expect(drawer?.getAttribute('aria-modal')).toBeNull()
  })

  it('exposes dialog semantics only when the drawer is open', () => {
    const container = document.createElement('div')

    renderNotificationDrawer(container, baseProps({ open: true }))

    const drawer = container.querySelector('[data-dashboard-primitive="drawer"]')
    expect(drawer).not.toBeNull()
    expect(drawer?.hasAttribute('hidden')).toBe(false)
    expect(drawer?.hasAttribute('inert')).toBe(false)
    expect(drawer?.getAttribute('aria-hidden')).toBeNull()
    expect(drawer?.getAttribute('role')).toBe('dialog')
    expect(drawer?.getAttribute('aria-modal')).toBe('true')
  })

  it('keeps Tab focus inside the open drawer from the last control', () => {
    const container = document.createElement('div')
    document.body.append(container)
    renderNotificationDrawer(container, baseProps({ open: true }))

    const markAllReadBtn = container.querySelector<HTMLButtonElement>('[data-mark-all-read]')
    const closeBtn = container.querySelector<HTMLButtonElement>('[data-notification-close]')
    expect(markAllReadBtn).not.toBeNull()
    expect(closeBtn).not.toBeNull()

    closeBtn!.focus()
    const event = new KeyboardEvent('keydown', { key: 'Tab', bubbles: true, cancelable: true })
    closeBtn!.dispatchEvent(event)

    expect(event.defaultPrevented).toBe(true)
    expect(document.activeElement).toBe(markAllReadBtn)
    container.remove()
  })

  it('keeps Shift+Tab focus inside the open drawer from the first control', () => {
    const container = document.createElement('div')
    document.body.append(container)
    renderNotificationDrawer(container, baseProps({ open: true }))

    const markAllReadBtn = container.querySelector<HTMLButtonElement>('[data-mark-all-read]')
    const closeBtn = container.querySelector<HTMLButtonElement>('[data-notification-close]')
    expect(markAllReadBtn).not.toBeNull()
    expect(closeBtn).not.toBeNull()

    markAllReadBtn!.focus()
    const event = new KeyboardEvent('keydown', { key: 'Tab', shiftKey: true, bubbles: true, cancelable: true })
    markAllReadBtn!.dispatchEvent(event)

    expect(event.defaultPrevented).toBe(true)
    expect(document.activeElement).toBe(closeBtn)
    container.remove()
  })
})
