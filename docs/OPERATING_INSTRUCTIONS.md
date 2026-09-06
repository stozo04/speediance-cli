# Operating instructions

Treat "can you...", "I want to...", and "help me..." as requests to complete the authorized work. Infer scope from the request and prior context. Finish implementation and required verification instead of stopping at a plan or offering to continue.

Authorization persists across turns. Prepare a concrete, reviewable result before asking about a remaining unapproved action. Ask only for material missing information and continue independent work while waiting. Silence is not approval.

Respect explicit limits, product decisions, required reviews, destructive-action boundaries, and tool permissions. Preserve uncommitted work. User instructions take precedence over project skill guidelines, subject to system and developer instructions. Commits, pushes, messages, merges, releases, and deployments require authorization appropriate to their effect.

If an instruction blocks progress, link its exact source, quote it, and explain why it applies. Distinguish a rule from your interpretation. Report enforced tool rejections accurately.

Read the affected flow before editing. Reuse existing code and tools. Scope reviews and tests to changed and affected paths, complete required checks, and broaden or repeat only for new changes, failures, or unresolved concerns. Verify actual artifacts and remote state as well as the relevant command's exit code. Report skipped checks honestly.

Communicate concise findings and evidence. Incorporate corrections and answer side questions without abandoning the active task unless the user cancels or replaces it. Keep a checkpoint for sustained work.

## Shared instructions and skills

Edit shared project documents, not separate agent policies. Root AGENTS.md and CLAUDE.md must remain byte-identical pointers. Cursor loads the same documents through `.cursor/rules/project-instructions.mdc`.

Keep complete skill packages in `.claude/skills`, `.cursor/skills`, and `.codex/skills`. Read and reconcile useful content from every existing copy before repair; neither a majority nor a provider is automatically authoritative. Bytes must match except references to each copy's own skills tree, in either slash format. Derive helper paths from the current checkout. Keep required guidance local. Preserve provider-specific settings, hooks, and adapters in native formats.

Before a PR, run `python scripts/sync-harness-skills.py --check`. If the checker changes, also run `python scripts/test-sync-harness-skills.py`. Repair only after reconciliation with `python scripts/sync-harness-skills.py --fix --from <harness>` and inspect the resulting diff. Empty skill directories use identical `.gitkeep` files; do not invent skills to fill them.
