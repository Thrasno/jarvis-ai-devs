/**
 * Brand — NEXUS HIVE emblem + wordmark + optional tagline.
 *
 * Returns an HTML string ready for innerHTML injection.
 * The emblem is an inline SVG hexagon with a linearGradient stroke (no binary assets).
 */

const EMBLEM_SVG = `<svg
  data-testid="nexus-emblem"
  class="nexus-emblem"
  xmlns="http://www.w3.org/2000/svg"
  width="32"
  height="36"
  viewBox="0 0 32 36"
  aria-hidden="true"
  focusable="false"
>
  <defs>
    <linearGradient id="nexus-emblem-gradient" x1="0%" y1="0%" x2="100%" y2="0%">
      <stop offset="0%" stop-color="#E0246F" />
      <stop offset="50%" stop-color="#8b5cf0" />
      <stop offset="100%" stop-color="#3B82E8" />
    </linearGradient>
  </defs>
  <polygon
    points="16,2 30,10 30,26 16,34 2,26 2,10"
    fill="none"
    stroke="url(#nexus-emblem-gradient)"
    stroke-width="2.5"
    stroke-linejoin="round"
  />
</svg>`

/**
 * Renders the brand block: emblem + wordmark.
 * Pass `withTagline: true` on the login screen.
 */
export function renderBrand(options: { withTagline?: boolean } = {}): string {
  const tagline = options.withTagline
    ? `<p class="dashboard-brand__tagline">Team memory, governed.</p>`
    : ''

  return `<span class="dashboard-brand">
  ${EMBLEM_SVG}
  <span class="dashboard-brand__wordmark">NEXUS HIVE</span>
  ${tagline}
</span>`
}
