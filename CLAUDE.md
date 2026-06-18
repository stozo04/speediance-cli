# CLAUDE.md — contributor guidance for Claude Code

Developer-facing brief for working **on** `speediance-cli` (a single-binary Go
CLI for the unofficial Speediance / Gym Monster cloud API). For *using* the CLI
from an agent see `AGENTS.md`; for the design contract see `GOAL.md`.

## MANDATORY — ClawHub security standards

Before changing anything that touches **credential handling, configuration
resolution, environment/`.env` loading, file writes, network calls, logging
output, or `SKILL.md`**, and before **publishing the skill to ClawHub**, you MUST
read and follow `.claude/CLAWHUB_STANDARDS.md`.

It is imported just below so its full text is always in your context — treat its
rules and pre-publish checklist as binding, and pin every new security behavior
with an immutable regression test (as described there).

@.claude/CLAWHUB_STANDARDS.md

## Build & verify

- `go build ./... && go vet ./... && go test ./...` must pass; `gofmt -l` must be clean.
- `main` is PR-protected — land changes via pull request.
