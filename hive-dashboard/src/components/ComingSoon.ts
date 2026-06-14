/**
 * ComingSoon — animated placeholder for unimplemented dashboard routes.
 */
export function comingSoon(screenLabel: string): HTMLElement {
  const card = document.createElement('div')
  card.className = 'dashboard-coming-soon'
  card.dataset.dashboardPrimitive = 'coming-soon'
  card.setAttribute('data-coming-soon', '')

  const glyph = document.createElement('div')
  glyph.className = 'dashboard-coming-soon__glyph'
  glyph.setAttribute('data-coming-soon-animated', '')
  glyph.setAttribute('aria-hidden', 'true')
  glyph.textContent = '✦'

  const title = document.createElement('h2')
  title.className = 'dashboard-coming-soon__title'
  title.textContent = screenLabel

  const message = document.createElement('p')
  message.className = 'dashboard-coming-soon__message'
  message.textContent = 'This screen is coming soon. We\'re building something great here.'

  card.append(glyph, title, message)
  return card
}
