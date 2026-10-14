# Changelog

All notable changes to this project are documented in this file.
The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and the project uses
[Semantic Versioning](https://semver.org/).

## [1.0.0] - 2026-10-14

### Added

- Directory list merged from `~/.claude.json` and the session transcripts, with search, sorting and filters.
- Session details: last activity, duration, message counts, size, AI title and first prompt.
- Actions: open in File Explorer, Windows Terminal (PowerShell fallback) or VS Code, `claude --continue`,
  `claude --resume` and resume of a specific session, copy path or session ID.
- "Running" badge for sessions owned by a live Claude Code process.
- Delete a single session, or remove a directory from Claude Code completely (transcripts, auto-memory and
  `.claude.json` entry, with backup). Both ask for confirmation; directory removal is blocked while Claude Code runs.
- `cleanupPeriodDays` editor with a preview of what the next cleanup would delete and detection of overriding
  managed, policy and project settings.
- Light and dark theme following Windows.
- About page with license and attribution.

[1.0.0]: https://github.com/idev-srl/CCSessions/releases/tag/v1.0.0
