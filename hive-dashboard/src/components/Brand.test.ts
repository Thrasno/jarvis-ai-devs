import { describe, expect, it } from 'vitest'
import { renderBrand } from './Brand'

describe('Brand lockup', () => {
  it('renders the login variant as the branded vertical hierarchy', () => {
    const container = document.createElement('div')
    container.innerHTML = renderBrand({ variant: 'login' })

    const brand = container.querySelector('.dashboard-brand--login')
    const children = Array.from(brand?.children ?? [])

    expect(children.map((child) => child.className)).toEqual([
      'dashboard-brand__emblem-img',
      'dashboard-brand__lockup',
      'dashboard-brand__tagline'
    ])
    expect(children[0]?.tagName.toLowerCase()).toBe('img')
    expect(children[1]?.querySelector('svg.nexus-n')).not.toBeNull()
    expect(children[1]?.querySelector('.dashboard-brand__wordmark')?.textContent).toBe('EXUS HIVE')
    expect(children[2]?.textContent).toBe('Team memory governed')
  })

  it('keeps the default sidebar brand dimensions and excludes the login tagline', () => {
    const container = document.createElement('div')
    container.innerHTML = renderBrand()

    const brand = container.querySelector('.dashboard-brand')
    const image = brand?.querySelector<HTMLImageElement>(':scope > .dashboard-brand__emblem-img')
    const glyph = brand?.querySelector<SVGElement>(':scope > .dashboard-brand__lockup .nexus-n')

    expect(brand?.classList.contains('dashboard-brand--login')).toBe(false)
    expect(image?.width).toBe(32)
    expect(image?.height).toBe(32)
    expect(glyph?.getAttribute('width')).toBe('16')
    expect(glyph?.getAttribute('height')).toBe('20')
    expect(brand?.querySelector('.dashboard-brand__tagline')).toBeNull()
  })

  it('renders a .dashboard-brand__lockup element wrapping N + wordmark', () => {
    const html = renderBrand()
    const container = document.createElement('div')
    container.innerHTML = html

    const lockup = container.querySelector('.dashboard-brand__lockup')
    expect(lockup).not.toBeNull()
  })

  it('lockup contains both the nexus-n SVG and the wordmark span', () => {
    const html = renderBrand()
    const container = document.createElement('div')
    container.innerHTML = html

    const lockup = container.querySelector('.dashboard-brand__lockup')
    expect(lockup?.querySelector('svg.nexus-n')).not.toBeNull()
    expect(lockup?.querySelector('.dashboard-brand__wordmark')).not.toBeNull()
  })

  it('emblem image is a sibling of the lockup, not inside it', () => {
    const html = renderBrand()
    const container = document.createElement('div')
    container.innerHTML = html

    const brand = container.querySelector('.dashboard-brand')
    const emblem = brand?.querySelector(':scope > .dashboard-brand__emblem-img')
    const lockup = brand?.querySelector(':scope > .dashboard-brand__lockup')

    // Both should be direct children of .dashboard-brand
    expect(emblem).not.toBeNull()
    expect(lockup).not.toBeNull()
  })

  it('wordmark reads EXUS HIVE', () => {
    const html = renderBrand()
    const container = document.createElement('div')
    container.innerHTML = html

    const wordmark = container.querySelector('.dashboard-brand__wordmark')
    expect(wordmark?.textContent).toBe('EXUS HIVE')
  })
})
