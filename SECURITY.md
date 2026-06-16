# Security Policy

## Reporting a vulnerability

Please report security issues **privately** rather than opening a public issue:

- Use GitHub's **"Report a vulnerability"** (Security → Advisories) on this repository, or
- email the maintainer (see the GitHub profile for `stozo04`).

Include steps to reproduce and the impact. You'll get an acknowledgement as soon as possible, and a
fix or mitigation will be coordinated before any public disclosure.

## Handling credentials (important for users)

This tool authenticates to your personal Speediance account. Treat these as secrets:

- **`config.json`** stores your email and **password in plaintext**.
- **`.token.json`** stores a live session token (written with `0600` permissions where supported).
- **`.env`** may store the same credentials.

All three are **gitignored** and must never be committed. For agents/headless use, prefer environment
variables (`SPEEDIANCE_EMAIL` / `SPEEDIANCE_PASSWORD`) over an on-disk file. The token cache location is
overridable via `SPEEDIANCE_TOKEN_CACHE`.

> This is an **unofficial** client for a reverse-engineered API. Use it only with your own account and
> data, at your own risk.

## Supported versions

Only the latest released version receives fixes.
