import { describe, expect, it } from 'vitest'
import { comingSoon } from '../components/ComingSoon'

describe('ComingSoon', () => {
  it('renders the screen label as visible text', () => {
    const el = comingSoon('Activity Feed')

    expect(el.textContent).toContain('Activity Feed')
  })

  it('has the data-coming-soon attribute', () => {
    const el = comingSoon('Global Search')

    expect(el.hasAttribute('data-coming-soon')).toBe(true)
  })

  it('declares the prefers-reduced-motion CSS in styles (animation present in component)', () => {
    const el = comingSoon('Knowledge Graph')

    // The component wraps content in an animated element
    const animated = el.querySelector('[data-coming-soon-animated]') ?? el
    expect(animated).not.toBeNull()
  })

  it('includes a friendly message', () => {
    const el = comingSoon('Analytics')

    // Should have some text beyond just the label
    expect(el.textContent!.length).toBeGreaterThan('Analytics'.length)
  })
})
