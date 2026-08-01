import { describe, expect, it } from 'vitest'
import { markdownViewer } from './MarkdownViewer'

describe('markdown viewer', () => {
  it('renders the supported GFM document profile with an accessible label', () => {
    const markdown = [
      '# Heading',
      '',
      'Plain **strong** and *emphasised* text with `inline <code>`.',
      '',
      '1. Ordered',
      '2. List',
      '',
      '- Unordered',
      '- [x] Completed',
      '',
      '> Quoted',
      '',
      '```html',
      '<button onclick="alert(1)">safe code</button>',
      '```',
      '',
      '| Name | Value |',
      '| --- | --- |',
      '| One | Two |',
      '',
      '---'
    ].join('\n')

    const view = markdownViewer(markdown, 'Memory content')

    expect(view).toBeInstanceOf(HTMLElement)
    expect(view.className).toBe('dashboard-markdown')
    expect(view.getAttribute('aria-label')).toBe('Memory content')
    expect(view.querySelector('h1')?.textContent).toBe('Heading')
    expect(view.querySelector('strong')?.textContent).toBe('strong')
    expect(view.querySelector('em')?.textContent).toBe('emphasised')
    expect(view.querySelectorAll('ol li')).toHaveLength(2)
    expect(view.querySelector('input')?.hasAttribute('disabled')).toBe(true)
    expect(view.querySelector('blockquote')?.textContent).toContain('Quoted')
    expect(view.querySelector('pre code')?.textContent).toContain('onclick="alert(1)"')
    expect(view.querySelector('table')).not.toBeNull()
    expect(view.querySelector('hr')).not.toBeNull()
  })

  it('neutralises raw HTML and strips dangerous markup, attributes, styles, and URLs', () => {
    const payload = [
      '<script>alert(1)</script>',
      '<iframe src="https://example.com"></iframe>',
      '<img src=x onerror="alert(1)" style="display:block">',
      '<a href="javascript:alert(1)" onclick="alert(1)" style="color:red">unsafe</a>',
      '<div onmouseover="alert(1)">raw</div>'
    ].join('\n')

    const view = markdownViewer(payload, 'Unsafe content')

    expect(view.querySelector('script, iframe, img, div')).toBeNull()
    expect(view.querySelector('[style], [onclick], [onerror], [onmouseover]')).toBeNull()
    expect(view.querySelector('a[href]')).toBeNull()
    expect(view.textContent).toContain('<script>alert(1)</script>')
    expect(view.textContent).toContain('<div onmouseover="alert(1)">raw</div>')
  })

  it('protects external links while preserving safe relative and same-origin links', () => {
    const view = markdownViewer(
      '[external](https://example.com/docs) [relative](/dashboard) [mail](mailto:team@example.com)',
      'Links'
    )
    const [external, relative, mail] = Array.from(view.querySelectorAll('a'))

    expect(external.getAttribute('rel')).toBe('noopener noreferrer')
    expect(relative.hasAttribute('rel')).toBe(false)
    expect(mail.hasAttribute('rel')).toBe(false)
  })

  it('degrades empty and malformed input safely and renders deterministically', () => {
    const empty = markdownViewer('', 'Empty content')
    const malformed = markdownViewer('[unfinished](<javascript:alert(1)', 'Malformed content')
    const first = markdownViewer('Plain text', 'Content')
    const second = markdownViewer('Plain text', 'Content')

    expect(empty.textContent).toBe('')
    expect(malformed.textContent).toContain('[unfinished]')
    expect(malformed.querySelector('[href^="javascript:"]')).toBeNull()
    expect(first.outerHTML).toBe(second.outerHTML)
  })
})
