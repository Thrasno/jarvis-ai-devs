import DOMPurify from 'dompurify'
import { Marked, type Tokens } from 'marked'

const ALLOWED_TAGS = [
  'a', 'blockquote', 'br', 'code', 'del', 'em', 'h1', 'h2', 'h3', 'h4', 'h5', 'h6',
  'hr', 'input', 'li', 'ol', 'p', 'pre', 'strong', 'table', 'tbody', 'td', 'th',
  'thead', 'tr', 'ul'
]
const ALLOWED_ATTRIBUTES = ['checked', 'disabled', 'href', 'title', 'type']
const ALLOWED_URI = /^(?:(?:https?|mailto):|[/?#.]|$)/i

const parser = new Marked({
  async: false,
  breaks: false,
  gfm: true,
  renderer: {
    html(token: Tokens.HTML | Tokens.Tag): string {
      return escapeHtml(token.text)
    }
  }
})

export function markdownViewer(markdown: string, accessibleLabel: string): HTMLElement {
  const root = document.createElement('div')
  root.className = 'dashboard-markdown'
  root.setAttribute('aria-label', accessibleLabel)

  const parsed = parser.parse(markdown)
  const fragment = DOMPurify.sanitize(typeof parsed === 'string' ? parsed : '', {
    ALLOWED_ATTR: ALLOWED_ATTRIBUTES,
    ALLOWED_TAGS,
    ALLOWED_URI_REGEXP: ALLOWED_URI,
    FORBID_ATTR: ['style'],
    FORBID_TAGS: ['iframe', 'script', 'style'],
    RETURN_DOM_FRAGMENT: true
  })
  root.append(fragment)
  protectExternalLinks(root)
  return root
}

function escapeHtml(value: string): string {
  const node = document.createElement('span')
  node.textContent = value
  return node.innerHTML
}

function protectExternalLinks(root: HTMLElement): void {
  for (const link of root.querySelectorAll<HTMLAnchorElement>('a[href]')) {
    try {
      const url = new URL(link.getAttribute('href') ?? '', window.location.href)
      if (url.protocol === 'http:' || url.protocol === 'https:') {
        if (url.origin !== window.location.origin) link.rel = 'noopener noreferrer'
      }
    } catch {
      link.removeAttribute('href')
    }
  }
}
