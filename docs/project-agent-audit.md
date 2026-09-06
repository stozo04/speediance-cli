# Project agent audit — 2026-09-06

## Scope and findings

Audited tracked root instructions, both local standards, contributor/security/release
instructions, the historical GOAL, root SKILL.md, Makefile, CI, and release packaging.
Excluded private reference data, dependencies, build output, and nested worktrees.
The original checkout is preserved; this branch starts at origin/main 7857392.
The existing setup-documentation PR #26 and dependency PRs are separate work and remain untouched.

## Changes and reconciliation

- Preserved the full agent CLI contract in docs/MACHINE_CONTRACT.md and contributor
  guidance in docs/PROJECT_INSTRUCTIONS.md. Identical root pointers and Cursor's
  alwaysApply project rule load the same shared operating and project guidance.
- Applied authorized-action persistence, material-only clarification, concrete approval,
  explicit boundaries, scoped verification, artifact proof, and honest blocker/skip reporting
  from the model guide: https://developers.openai.com/api/docs/guides/latest-model?model=gpt-6-astra.
- The root published SKILL.md is the only existing skill package. Copied it byte-for-byte
  to all three harnesses (one file each), preserving its permissions and product contract.
  There were no competing copies, helpers, or provider settings to reconcile. Keep the
  root publication artifact and mirrors synchronized on future edits.
- Reused OpenLoop PR #176's checker and 16 regression cases; generalized its introduction
  and remote-default fallback only. make check-agents participates in make check and CI.
  It covers missing/deleted/untracked files, drift, identical foreign references in both
  slash forms, CRLF preservation, source-preserving repair, and ambiguity refusal.
- Retained both .claude standards at their established paths as shared local documents;
  all harnesses explicitly read them. CLI_CONVENTIONS.md remains byte-identical to its
  sibling copy; it requires no read from another checkout. Its historical @import
  description now routes through the root pointer to explicit shared-document reads.
- Preserved frozen API/JSON semantics, credential privacy, immutable security tests,
  no-doctor decision, GM1-only guarantee, Conventional Commits, and release/publish gates.
  Fixed the release-playbook filename's case in the relocated contributor document.
- Included the pointer targets and referenced policy docs in GoReleaser archives, and
  the SKILL.md already promised by its footer. Publishing triggers are unchanged.
- The required make check formatter adjusted two Go call layouts in config.go and
  config_test.go; no runtime logic or test assertion changed.

## Validation

- make check passed: mirror parity, all 16 checker regression cases, tidy, formatting,
  vet, golangci-lint (0 issues), and race tests (7 passing packages; 1 has no tests).
- go build ./... and make build passed; the built binary exists and is nonempty.
- Root pointers are byte-identical; root SKILL.md matches all three skill copies.
- goreleaser check validated the configuration. Final diff and whitespace reviewed.

## Skipped and limitations

No live account login, workout retrieval, program creation, ClawHub publication,
release, deployment, or full cross-platform archive build was performed. Runtime and
account behavior is unchanged; fixture/race tests cover the existing security guards.
Windows Unix-permission assertions remain platform-inapplicable where the existing
suite says so; Linux CI runs them. Setup/credential ordering in the unchanged published
skill remains tracked by existing PR #26; this audit does not duplicate its rewrite.
