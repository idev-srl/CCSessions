# Contributing to CC Sessions

Thanks for your interest! Bug reports, ideas and pull requests are welcome.

## Getting started

```powershell
go install github.com/wailsapp/wails/v2/cmd/wails@latest
git clone https://github.com/idev-srl/CCSessions.git
cd CCSessions
wails dev
```

## Guidelines

- **Never write to Claude Code's state files** except through the dedicated, tested functions in
  `internal/ccdata` (`settings.json` for `cleanupPeriodDays`, and `.claude.json` only for an explicit directory
  removal while Claude Code is closed). Always back up and write atomically.
- Keep `internal/ccdata` free of Wails dependencies so it stays testable and reusable.
- The transcript format is not a stable API: parse defensively and never crash on unexpected input.
- Add unit tests for any change in `internal/ccdata` and run `go test ./internal/...` before opening a PR.
- Run `gofmt` and `npx tsc --noEmit` (in `frontend/`).
- UI text and documentation are in English.
- Screenshots must use demo data, never real paths or project names.

## License

By contributing you agree that your contributions are licensed under the [Apache License 2.0](LICENSE).
