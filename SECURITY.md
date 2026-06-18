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
- **The token cache** stores a live session token (written with `0600` permissions where supported).
  By default it lives in your **OS user-config directory** (`speediance/token.json` under `%AppData%`
  on Windows, `~/.config` on Linux, `~/Library/Application Support` on macOS) — deliberately **outside**
  the working directory so a routine `git add -A` can't commit it. Override the path with
  `SPEEDIANCE_TOKEN_CACHE` or the `token_cache_path` config key; `config path` shows where it resolved.
  A token left by an older version in a working-directory `.token.json` is relocated to the per-user
  location on first run.
- **`.env`** may store the same credentials.

These are **gitignored** and must never be committed. For agents/headless use, prefer environment
variables (`SPEEDIANCE_EMAIL` / `SPEEDIANCE_PASSWORD`) over an on-disk file.

> This is an **unofficial** client for a reverse-engineered API. Use it only with your own account and
> data, at your own risk.

## Supported versions

Only the latest released version receives fixes.
