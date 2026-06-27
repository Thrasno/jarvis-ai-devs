/**
 * Brand — NEXUS HIVE emblem + wordmark + optional tagline.
 *
 * Returns an HTML string ready for innerHTML injection.
 * The emblem is rendered as an inline SVG (no external asset dependency).
 */

/**
 * Renders the brand block: emblem + wordmark.
 * Pass `withTagline: true` on the login screen.
 * Pass `size` to control emblem dimensions (default 30 for sidebar, 64 for login).
 */
export function renderBrand(options: { withTagline?: boolean; size?: number } = {}): string {
  const size = options.size ?? 30
  const tagline = options.withTagline
    ? `<p class="dashboard-brand__tagline">Team memory, governed.</p>`
    : ''

  return `<span class="dashboard-brand">
  <svg data-testid="nexus-emblem" class="nexus-emblem" width="${size}" height="${size}" viewBox="0 0 40 44" fill="none" xmlns="http://www.w3.org/2000/svg" aria-hidden="true" focusable="false">
    <defs>
      <linearGradient id="nexus-emblem-gradient-${size}" x1="0" y1="0" x2="1" y2="1">
        <stop offset="0%" stop-color="#E0246F"/>
        <stop offset="50%" stop-color="#8b5cf0"/>
        <stop offset="100%" stop-color="#3B82E8"/>
      </linearGradient>
    </defs>
    <path d="M20 2.5 L35.6 11.25 V28.75 L20 41.5 L4.4 28.75 V11.25 Z" stroke="url(#nexus-emblem-gradient-${size})" stroke-width="2" stroke-linejoin="round"/>
    <g stroke="url(#nexus-emblem-gradient-${size})" stroke-width="1.3">
      <ellipse cx="20" cy="21.5" rx="10.5" ry="4.2"/>
      <ellipse cx="20" cy="21.5" rx="10.5" ry="4.2" transform="rotate(60 20 21.5)"/>
      <ellipse cx="20" cy="21.5" rx="10.5" ry="4.2" transform="rotate(120 20 21.5)"/>
    </g>
    <circle cx="20" cy="21.5" r="2.3" fill="#E0246F"/>
  </svg>
  <span class="dashboard-brand__wordmark">NEXUS HIVE</span>
  ${tagline}
</span>`
}
