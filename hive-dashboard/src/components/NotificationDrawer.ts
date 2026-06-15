import type { NotificationSummaryViewModel, NotificationViewModel } from '../domain/dashboard'

export type DrawerProps = {
  readonly notifications: readonly NotificationViewModel[]
  readonly summary: NotificationSummaryViewModel
  readonly readIds: ReadonlySet<string>
  readonly open?: boolean
  readonly onMarkAllRead: () => void
  readonly onClose: () => void
}

export function renderNotificationDrawer(container: HTMLElement, props: DrawerProps): void {
  container.replaceChildren()

  const drawer = document.createElement('div')
  drawer.className = 'dashboard-drawer'
  drawer.dataset.dashboardPrimitive = 'drawer'
  drawer.setAttribute('aria-labelledby', 'notifications-title')

  if (props.open) {
    drawer.dataset.open = ''
    drawer.setAttribute('role', 'dialog')
    drawer.setAttribute('aria-modal', 'true')
    drawer.addEventListener('keydown', (event) => handleDrawerKeydown(event, props.onClose))
  } else {
    drawer.hidden = true
    drawer.setAttribute('inert', '')
    drawer.setAttribute('aria-hidden', 'true')
  }

  // Header
  const header = document.createElement('div')
  header.className = 'dashboard-drawer__header'

  const title = document.createElement('h2')
  title.id = 'notifications-title'
  title.className = 'dashboard-drawer__title'
  title.textContent = 'Notifications'

  const counter = document.createElement('span')
  counter.className = 'dashboard-drawer__counter'
  counter.dataset.notificationCounter = ''
  counter.textContent = `${props.summary.unread}/${props.summary.total}`

  const markAllReadBtn = document.createElement('button')
  markAllReadBtn.type = 'button'
  markAllReadBtn.className = 'dashboard-control control dashboard-drawer__mark-all-read'
  markAllReadBtn.dataset.dashboardPrimitive = 'control'
  markAllReadBtn.dataset.markAllRead = ''
  markAllReadBtn.textContent = 'Mark all read'
  markAllReadBtn.addEventListener('click', () => props.onMarkAllRead())

  const closeBtn = document.createElement('button')
  closeBtn.type = 'button'
  closeBtn.className = 'dashboard-control control dashboard-drawer__close'
  closeBtn.dataset.dashboardPrimitive = 'control'
  closeBtn.dataset.notificationClose = ''
  closeBtn.dataset.drawerClose = ''
  closeBtn.setAttribute('aria-label', 'Close notifications')
  closeBtn.textContent = '×'
  closeBtn.addEventListener('click', () => props.onClose())

  header.append(title, counter, markAllReadBtn, closeBtn)
  drawer.append(header)

  // Notification list or empty state
  if (props.notifications.length === 0) {
    const empty = document.createElement('p')
    empty.className = 'dashboard-drawer__empty'
    empty.dataset.notificationEmpty = ''
    empty.textContent = 'No notifications yet. Check back soon!'
    drawer.append(empty)
  } else {
    const list = document.createElement('ul')
    list.className = 'dashboard-drawer__list'

    for (const notification of props.notifications) {
      const card = renderNotificationCard(notification, props.readIds)
      list.append(card)
    }

    drawer.append(list)
  }

  container.append(drawer)
}

function handleDrawerKeydown(event: KeyboardEvent, onClose: () => void): void {
  if (event.key === 'Escape') {
    event.preventDefault()
    onClose()
    return
  }

  trapDrawerFocus(event)
}

function trapDrawerFocus(event: KeyboardEvent): void {
  if (event.key !== 'Tab') return

  const drawer = event.currentTarget
  if (!(drawer instanceof HTMLElement)) return

  const focusable = getFocusableElements(drawer)
  if (focusable.length === 0) {
    event.preventDefault()
    drawer.focus()
    return
  }

  const first = focusable[0]
  const last = focusable[focusable.length - 1]
  const activeElement = document.activeElement

  if (event.shiftKey && activeElement === first) {
    event.preventDefault()
    last.focus()
    return
  }

  if (!event.shiftKey && activeElement === last) {
    event.preventDefault()
    first.focus()
  }
}

function getFocusableElements(container: HTMLElement): HTMLElement[] {
  return Array.from(container.querySelectorAll<HTMLElement>([
    'a[href]',
    'button:not([disabled])',
    'input:not([disabled])',
    'select:not([disabled])',
    'textarea:not([disabled])',
    '[tabindex]:not([tabindex="-1"])'
  ].join(',')))
}

function renderNotificationCard(notification: NotificationViewModel, readIds: ReadonlySet<string>): HTMLElement {
  const isRead = readIds.has(notification.id)
  const isUnread = notification.unread && !isRead

  const card = document.createElement('li')
  card.className = isUnread
    ? 'dashboard-notification-card dashboard-notification-card--unread'
    : 'dashboard-notification-card'
  card.dataset.notificationCard = notification.id

  // Unread indicator dot
  if (isUnread) {
    const dot = document.createElement('span')
    dot.className = 'dashboard-notification-card__unread-dot'
    dot.dataset.unreadIndicator = ''
    dot.setAttribute('aria-label', 'Unread')
    card.append(dot)
  }

  // Title
  const titleEl = document.createElement('p')
  titleEl.className = 'dashboard-notification-card__title'
  titleEl.textContent = notification.title

  // Category badge
  const categoryBadge = document.createElement('span')
  categoryBadge.className = 'dashboard-status status'
  categoryBadge.dataset.dashboardStatus = 'neutral'
  categoryBadge.textContent = notification.category

  // Actor handle
  const actor = document.createElement('span')
  actor.className = 'dashboard-notification-card__actor'
  actor.textContent = `@${notification.actorHandle}`

  // Project name
  const project = document.createElement('span')
  project.className = 'dashboard-notification-card__project'
  project.textContent = notification.projectName

  // Time label
  const time = document.createElement('span')
  time.className = 'dashboard-notification-card__time'
  time.textContent = notification.timeLabel

  card.append(titleEl, categoryBadge, actor, project, time)
  return card
}
