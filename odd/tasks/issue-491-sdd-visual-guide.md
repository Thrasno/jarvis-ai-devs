# Issue 491 — Illustrated SDD guide

## Objective
Create a Spanish illustrated SDD workflow guide matching the installation guides and publish the personal checklist removal to PR #766.

## Scope and constraints
- Worktree: jarvis-dev-pr-766; branch: docs/issue-491-onboarding-checklist.
- Use existing screenshots in docs/presentaciones/capturas/pantallazosSDD.
- Explain workflow, human approvals, benefits, use cases and limitations using repository-backed commands.
- Preserve unrelated changes, including odd/tasks/issue-491-dashboard-guide-style.md.
- No builds. Passive documentation has no meaningful RED; use structural/content checks instead.
- User explicitly authorized publishing the checklist deletion. Guide publication is to the existing requested PR.

## Tasks
- [x] T1 Remove personal checklist and publish deletion — complete.
  Acceptance: only checklist removal and this task record committed; push confirmed.
- [x] T3 Anonymize and verify five screenshots — complete; commit 99d7df01.
  Acceptance: replace sensitive identifiers and client-specific business details irreversibly with fictional examples; preserve readable phase structure; inspect flattened PNGs; original Git history is not claimed sanitized.
- [x] T2 Create, verify and publish illustrated SDD guide — complete; commit 7b247daa.
  Acceptance: installation/Hive API visual style; five existing screenshots with captions/alt; correct phases and human gates; valid local links; no fabricated execution evidence; published HTMLPreview URL.

## Evidence
- Checklist removal already exists locally: docs/issue-491-onboarding-checklist.html, 129 deleted lines.
- No references found under docs to its filename.
- RDD enabled globally; passive documentation exception applies, not source mutation.
- Exploration delegated to gentle-ai-explore.
- T1 commit: ea31ec02; push rejected (remote contains concurrent work). T1 remains incomplete until publication.
- Separate read-only divergence diagnosis delegated; no force push, reset, or discarded changes.
- Engram mirror saved; full readback unavailable because tool schema rejected observation_id.

- Divergence resolved by non-destructive merge 3d991d59; push confirmed 393431c5..3d991d59. Unrelated modification preserved.
- Exploration verified actual SDD commands and persistence rules. Spec/design both depend on proposal, tasks on both; /sdd-ff plans only. Use existing installation HTML as style source.
- Edit surfaces: docs/presentaciones/sdd-guia.html, docs/presentaciones/windows-onboarding.html, docs/presentaciones/ubuntu-onboarding.html, docs/sdd-user-guide.md (navigation links only in existing files).

- T2 worker stopped before edits: inspected five screenshots and flagged potentially sensitive CRM operational details and invoice identifiers. Synthetic/publication-safe status is unconfirmed.
- Guide not created; HTML/link/browser checks pending. Await anonymized images or explicit confirmation all screenshot content is synthetic and publicly publishable.

- User explicitly authorized assistant-side screenshot anonymization. Use local deterministic image editing; no external upload, blur, history rewrite or original backups in repository.

- T3 worker replaced all five images with coherent fictional reading-list examples because client content permeated originals; retained dark editor style and original dimensions, with explicit illustrative label.
- Worker checks passed: visual read of five outputs, RGB PNG dimensions, CRC/decompression, IHDR/IDAT/IEND only (no metadata/alpha/trailing payload), ImageMagick decoding, git diff --check.
- Independent image/privacy verification delegated to gentle-ai-verify; T3 remains in progress until reviewed and committed. Original Git history remains unsanitized.

- Independent T3 verification PASS: five visual inspections, PNG CRC/order/decompression/no metadata/no trailing bytes; dimensions match originals. Parent Proposal spot check passed. Commit 99d7df01 includes only the five images.

- T2 guide created: docs/presentaciones/sdd-guia.html (296 lines), plus navigation links in Windows, Ubuntu and SDD reference (+4 lines). CSS identical to installation template.
- Writer checks PASS: git diff --check; HTMLParser balanced markup, Spanish lang, 10 unique IDs, 22 local references/anchors, five images with alt/captions, ten commands.
- Independent content/desktop/mobile browser verification delegated. Writer browser rendering was not run due artifact-scope limitation; verifier explicitly authorized ephemeral profiles/screenshots outside repo.
- Native ASSESS unavailable due undeclared untracked HTML; returned high-risk-equivalent independent verification plan, already underway. No runtime code changed; no Go tests/build applicable.

- Independent checks PASS: content against source, HTMLParser/local links/alt/anchors and git diff --check. Firefox desktop 1440x1000 and exact mobile 390x844 render without page overflow; all images load; navigation works. Intentional command-block horizontal scrolling confirmed.
- Print check: 13 A4 pages inspected, no clipping/missing figures; some whitespace. External destinations, other browsers, physical printing and touch gestures unverified.
- Mobile caveat: embedded screenshot text small. Added five full-size illustration links in captions; targeted link/browser recheck underway. Prior print results precede these caption additions.

- Targeted final recheck PASS: five links resolve; 390x844 mobile captions fit, image link opens correct native-resolution PNG and browser back restores guide. git diff --check passed independently and in parent spot check.
- Guide commit 7b247daa (4 files, 300 additions) and sanitized image commit 99d7df01 published; push confirmed 3d991d59..7b247daa.
- HTMLPreview: https://htmlpreview.github.io/?https://github.com/Thrasno/jarvis-ai-devs/blob/docs/issue-491-onboarding-checklist/docs/presentaciones/sdd-guia.html
- Final print after caption-link additions not repeated; earlier print checked 13 A4 pages. No Go tests/builds (static documentation only). Native assessment unavailable; independent verifier completed, no native approval claimed.

## Next step
Human reviews published guide in PR #766. Original Git history remains unsanitized; history rewrite was not authorized.
