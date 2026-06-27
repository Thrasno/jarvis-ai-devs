/**
 * Brand — NEXUS HIVE emblem + wordmark + optional tagline.
 *
 * Returns an HTML string ready for innerHTML injection.
 * The emblem uses the real PNG asset (nexus-emblem.png) via Vite asset import.
 */
import emblemUrl from '../assets/nexus-emblem.png'

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
  <img
    src="${emblemUrl}"
    class="nexus-emblem"
    data-testid="nexus-emblem"
    alt=""
    aria-hidden="true"
    width="${size}"
    height="${size}"
  />
  <span class="dashboard-brand__wordmark">NEXUS HIVE</span>
  ${tagline}
</span>`
}
