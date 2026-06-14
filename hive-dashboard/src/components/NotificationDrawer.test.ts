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
})
