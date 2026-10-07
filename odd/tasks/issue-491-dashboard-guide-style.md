# Issue 491: Dashboard visual style for onboarding guides

## Intent and scope
User requests the visual style of Hive API Dashboard for guides in existing PR #766, not a prose rewrite. Restyle docs/presentaciones/windows-onboarding.html, ubuntu-onboarding.html and glosario-jarvis.html in the clean PR worktree. Preserve text, commands, links, IDs, screenshot assets, evidence caveats and standalone offline operation. Exclude checklist tool, generated configuration, dashboard product sources and dirty original checkout.

Reference: hive-dashboard/src/styles.css, components/Sidebar.ts and Brand.ts. Use dark surfaces, subtle borders, pink/blue/violet accents, compact chrome, navigation rail and reading panels. Keep Nexus identity. Font fallbacks must work offline; no Google-font downloads or new runtime dependencies. Preserve mobile, keyboard and print usability.

## Method and delivery
Organic delegated implementation; multi-file writing and preparation trigger one bounded writer. Static documentation: strict implementation TDD does not apply; no fabricated RED or Go checks. Runner: Python HTMLParser/content invariants and git diff --check. No builds. Delivery remains existing PR #766; prior feature accepted size exception. Forecast 200–350 authored diff lines, advisory only; preserve readable source rather than minifying. Do not commit or push during writing; report local candidate and validation first.

## Tasks
- [x] T1 (delegated explore): Mapped PR worktree and real dashboard styling. Clean worktree matches published head 82847901; original dirty checkout is stale and preserved.
- [x] T2 (delegated writer): CSS-only restyle of all three documents. Worker reports git diff --check and HTMLParser content/resource/accessibility/hash invariants passed. HTML outside style blocks unchanged. Diff +357/-82, 439 authored lines; readable CSS retained beyond advisory forecast.
- [x] T3 (independent verifier and parent spot check): Independent structural PASS: CSS identical, HTML outside style unchanged, 62 local references resolve, 11 image hashes match HEAD, no remote resources. Firefox screenshots at 1440x1000 and 390x844 for all three, glossary additionally at 901/900x1000; sampled top viewports show no visible defect. Parent reran git diff --check successfully. No approval/receipt claimed.
- [ ] T4 (remaining manual QA): Check below-fold command overflow, anchor scrolling, real keyboard skip/focus and print pagination; screenshot attempts with #instalar stayed at the top and do not prove anchor behavior.

## Acceptance and evidence
All three guides share dashboard-style composition, not merely palette. Content/commands/accessibility attributes and screenshots remain intact; links/anchors/images resolve; no remote runtime dependencies; mobile and print CSS retained. Real browser rendering must be checked when available, otherwise explicitly pending. Structural checks are not visual QA.

## Progress and next step
T1–T3 complete in clean /home/andres/Desarrollo/Proyectos/jarvis-dev-pr-766. Worker gpt-6.1-sol/medium and independent verifier passed static checks; parent spot check passed. An initial verifier assertion compared shifted source line numbers and failed; corrected semantic invariant check passed (test-harness error). Render screenshots exist in /tmp/jarvis-766-verify-i_k9u1kr/; top-view desktop/mobile samples inspected successfully, not full interaction/print QA. T4 remains pending. No builds or installs. User explicitly authorized publication. Native review review-c089de377a5d2af9 approved and exact acknowledgement completed; one informational task-record warning did not open a correction. Work-unit commit a2f851aa was pushed without force to PR #766. This evidence-only follow-up records delivery without changing the reviewed CSS. Next: remaining T4 manual QA; issue #491 remains open. Original dirty checkout preserved except separately authorized removal of rejected role-journey draft. Live #491 still requests roles/trainer material and needs separately authorized scope update.
