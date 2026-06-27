/**
 * Brand — NEXUS HIVE emblem + wordmark + optional tagline.
 *
 * The brand block reads: [nexus emblem image] [tri-color N] [EXUS HIVE wordmark].
 * The emblem image is the hexagonal node ring PNG asset.
 * The tri-color "N" SVG is the Conpas NLogo glyph.
 * Together with the wordmark "EXUS HIVE" it reads "NEXUS HIVE".
 *
 * Returns an HTML string ready for innerHTML injection.
 */
import emblemUrl from '../assets/nexus-emblem.png'

/**
 * Renders the brand block: nexus emblem image + Conpas NLogo emblem + "EXUS HIVE" wordmark.
 * Pass `withTagline: true` on the login screen.
 * Pass `size` to control the base size (default 30 for sidebar, 64 for login).
 * Emblem image: sidebar (size ≤30) → 32px; login (size 64) → 56px.
 * N glyph dimensions scale with size: sidebar (30) → 16×20px, login (64) → 24×30px.
 */
export function renderBrand(options: { withTagline?: boolean; size?: number } = {}): string {
  const size = options.size ?? 30
  // Scale the N glyph proportionally: at size 30 → 16×20, at size 64 → 24×30
  const nWidth = size <= 30 ? 16 : 24
  const nHeight = size <= 30 ? 20 : 30
  // Scale the emblem image: sidebar → 32px, login → 56px
  const emblemSize = size <= 30 ? 32 : 56
  const tagline = options.withTagline
    ? `<p class="dashboard-brand__tagline">Team memory, governed.</p>`
    : ''

  return `<span class="dashboard-brand">
  <img src="${emblemUrl}" width="${emblemSize}" height="${emblemSize}" alt="" aria-hidden="true" data-testid="nexus-emblem-img" class="dashboard-brand__emblem-img" style="object-fit:contain;flex-shrink:0">
  <svg class="nexus-n" data-testid="nexus-emblem" width="${nWidth}" height="${nHeight}" viewBox="0 0 100 100" fill="none" preserveAspectRatio="none" aria-hidden="true" focusable="false">
    <polygon points="9,11 30,11 30,89 9,89" fill="#22B85C"/>
    <polygon points="70,11 91,11 91,89 70,89" fill="#E0246F"/>
    <polygon points="30,11 49,11 70,89 51,89" fill="#3B82E8"/>
  </svg><span class="dashboard-brand__wordmark">EXUS HIVE</span>
  ${tagline}
</span>`
}
