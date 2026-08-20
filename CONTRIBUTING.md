# Contributing

## Workflow

1. Create a focused branch from `features/wails3`.
2. Keep changes scoped to the behavior being changed and preserve the existing UI and service contracts.
3. Regenerate bindings after changing exported Go service methods:

   ```powershell
   wails3 generate bindings -ts -clean
   ```

4. Run the checks below before opening a pull request.
5. Describe user-visible behavior, platform coverage, and any YouTube response assumptions in the pull request.

## Local checks

```powershell
npm --prefix frontend ci
npm --prefix frontend run typecheck
go test ./...
go vet ./...
git diff --check
wails3 build GOOS=windows ARCH=amd64 PRODUCTION=true
```

Production Windows builds require UPX and use `--best --lzma --force` compression. Pass `COMPRESS=false` when a locally uncompressed build is needed.

Do not edit files under `frontend/bindings` by hand. They are generated from the exported Go service API. Do not commit `frontend/node_modules`, `frontend/dist`, `bin`, `.task`, build outputs, credentials, updater keys, or local application data.

## Code expectations

- Preserve the existing command behavior and JSON field names when changing the Go service.
- Keep network failures visible to the frontend instead of returning fabricated results.
- Avoid logging tokens, cookies, private URLs, or personal data.
- Update the README when user-facing commands, requirements, storage, or supported platforms change.

## Pull requests

Pull requests should include a concise summary, verification commands and results, screenshots or recordings for visual changes when useful, and a note for any platform-specific limitation. Security-sensitive issues must use the process in [SECURITY.md](SECURITY.md) instead of a public issue.
