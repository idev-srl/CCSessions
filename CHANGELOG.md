# Changelog

All notable changes to this project are documented in this file.
The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and the project uses
[Semantic Versioning](https://semver.org/).

## [1.0.1] - 2026-10-14

### Fixed

- Project folders of directories with non-ASCII characters in their path (e.g. `C:\Progetti\Università`, emoji)
  were not found, so the auto-memory was not shown or removed. The folder name is now computed like Claude Code
  does, per UTF-16 code unit, including the truncation with a hash for names longer than 200 characters.
- When the computed folder does not exist but all of a directory's transcripts are in a single folder, that
  folder is used as the project folder. Folders shared with other directories are still never deleted whole.

### Documentation

- FAQ: with an npm install Claude Code runs as `node.exe`, so the "Remove" safety check relies only on
  `<base>\sessions\<pid>.json`; close every session before removing a directory.

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
- Release executable Authenticode-signed by iDev srl.

[1.0.1]: https://github.com/idev-srl/CCSessions/releases/tag/v1.0.1
[1.0.0]: https://github.com/idev-srl/CCSessions/releases/tag/v1.0.0
