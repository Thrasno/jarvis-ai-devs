# Issue 491 — PDF guide delivery

## Objective
Replace the four published HTML guides with self-contained PDFs and remove the 16 tracked PNG source images from the current PR tree.

## Scope
- PR #766; worktree jarvis-dev-pr-766; branch docs/issue-491-onboarding-checklist.
- Same-basename PDFs in docs/presentaciones: windows-onboarding, ubuntu-onboarding, glosario-jarvis, sdd-guia.
- Update functional links in docs/getting-started.md, docs/installation.md, docs/sdd-user-guide.md.
- Keep capturas/.gitignore and unrelated modified odd/tasks/issue-491-dashboard-guide-style.md.
- Preserve historical task documents. No Git history rewriting or builds. User authorized deletion, commits and publication.

## Tasks
- [x] P1 Export and verify four PDFs — complete; commit 8f058450.
  Checks: embedded images, complete/readable pages, Spanish content, usable cross-guide/reference links, no temporary file paths or private metadata; independent verification.
- [x] P2 Remove HTML/images, update links and publish — complete; commit ca2ad07e.
  Checks: four PDFs tracked, four HTML and sixteen PNGs absent, functional Markdown links resolve; only authorized paths committed; push and remote confirmed.

## Decisions
- Generate using local Firefox with isolated temporary profile; no external upload or dependency installation.
- Update conversion inputs: visible printable guide links, PDF-oriented footer, remove PNG full-size anchors, map repository links to public GitHub URLs, preserve internal destinations when supported.
- Keep source images until PDF content and links pass independent checks.
- Static document conversion has no meaningful RED; no Go tests/builds applicable.
- Current-tree removal does not sanitize original Git history.

## Evidence
- Read-only explorer identified four HTMLs, sixteen tracked PNGs and three functional Markdown link surfaces.
- Firefox 156.0.1, pdfinfo, pdftoppm and pdftotext available.

- Writer generated four PDFs: Windows 5 pages/10 images, Ubuntu 5/11, glossary 4/0 raster figures, SDD 11/5 synthetic illustrations.
- Writer checks PASS: pdfinfo, PDF URLs/image inventory, extracted content (192 main blocks), all 25 rendered pages read, 90 link annotations audited, no local paths/deleted-source links/obsolete editing promises; original source hashes unchanged.
- PDFs are untagged; dense terminal illustrations require zoom. Cross-guide public targets await publication.
- Independent verifier delegated before any source deletion.

- Independent verification found actionable copy/paste defect in both onboarding PDFs page 2: stable/beta installer URLs split across physical extracted lines, changing shell semantics. Hold source removal; P1 remains incomplete.
- Other independent checks PASS: all 25 rendered pages, 10/11/0/5 image counts, preserved content, 90 public HTTPS annotations with no temporary/deleted-source targets, source hashes and unrelated modification unchanged.

- Windows/Ubuntu regenerated with temporary nonwrapping 7.25pt command blocks. Worker checks PASS: both raw/layout extraction preserve exact pipelines on page 2, beta assignments preceding separately; ten pages visually checked, original counts/content/links preserved and 25 other files hash-unchanged.
- Independent targeted recheck delegated; no sources deleted yet. Command text smaller; zoom useful; PDFs untagged.

- Independent corrected-PDF recheck PASS: exact physical installer lines in raw/layout extraction, all ten pages visually inspected, 10/11 images, safe annotations and other source/PDF hashes preserved.
- P1 committed as 8f058450; parent git diff --check spot check passed.

- Cleanup writer cannot perform deletions under its safety contract; parent performed the explicitly authorized mechanical allowlist deletion (4 HTML, 16 PNG). No folder-wide deletion or history rewrite.
- Parent recorded preservation hashes; PDF hashes match committed versions. capturas/.gitignore and unrelated dashboard task retained.
- Three-file Markdown link updates and final tree checks delegated back to writer.

- P2 checks PASS: exactly four HTML/sixteen PNG absent; all PDFs byte-identical to 8f058450; preservation hashes match; five Markdown PDF links resolve; 370 functional docs scanned with no deleted-asset references. Parent diff/readback and git diff --check passed.
- Cleanup commit ca2ad07e published with P1; push confirmed 1759aaf3..ca2ad07e.
- GitHub recursive tree and PR changed-file inventory confirm four PDF guides present and no HTML/PNG sources under docs/presentaciones; only capturas/.gitignore retained.
- PDFs remain untagged and dense command/image text benefits from zoom; external destination HTTP behavior and physical printing not tested. No Go tests/builds applicable, installers never executed. Git history intentionally unchanged.

## Next step
Human reviews PDFs in PR #766. No pending implementation checks.
