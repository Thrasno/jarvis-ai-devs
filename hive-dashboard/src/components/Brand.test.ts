import { describe, expect, it } from 'vitest'
import { renderBrand } from './Brand'

describe('Brand lockup', () => {
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
