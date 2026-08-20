# YTLite

YTLite is a lightweight YouTube desktop player built with Wails3, Go, TypeScript, and Vite. It keeps the existing experience—search, discovery, Shorts, subscriptions, playlists, queueing, history, blocking, custom window controls, tray controls, and the embedded player guard—in a native desktop application.

## Features

- YouTube search with continuation paging and direct `youtube-nocookie.com` playback.
- Home discovery, Shorts, related playback, queueing, history, and playlists.
- Subscription import/export, channel feeds, avatar loading, and channel removal.
- Video and channel blocking persisted in the application data directory.
- Auto-play, reduced-motion, publish-date, responsive navigation, and player-control settings.
- Frameless 1280×720 window with resize limits, custom minimize/hide/maximize controls, tray show/hide, and Windows input locking.
- Embedded player guard for ad/analytics request filtering, player actions, Shorts navigation, mini-player/Picture-in-Picture controls, drag handling, and input shortcuts.

## Requirements

- Go 1.26 or newer.
- Node.js 22 or newer and npm.
- Wails3 CLI `v3.0.0-beta.11`.
- UPX for compressed production Windows builds.
- Microsoft WebView2 on Windows.

The YouTube scraper uses public web responses and is intentionally unauthenticated. Changes to YouTube's page or continuation data can affect results without a code change in YTLite.

## Development

```powershell
npm --prefix frontend ci
wails3 generate bindings -ts -clean
wails3 task dev
```

The development task starts the Vite frontend and the Wails3 application. To build a Windows x64 executable:

```powershell
wails3 build GOOS=windows ARCH=amd64 PRODUCTION=true
```

The executable is written to `bin/ytlite.exe` and compressed with UPX using `--best --lzma --force`. Pass `COMPRESS=false` to disable compression for a local build. The generated frontend bundle is under `frontend/dist`, and generated TypeScript bindings are under `frontend/bindings`; both are build products and the bindings should be regenerated after Go service changes.

Useful checks:

```powershell
npm --prefix frontend run typecheck
go test ./...
go vet ./...
git diff --check
```

## Data

On Windows, YTLite stores `subscriptions.json` and `blocks.json` under `%APPDATA%\land.moreno.ytlite`. Existing data from the former application directories is migrated on first access. History, playlists, and presentation settings remain in the frontend local-storage namespace used by the application.

## Repository branches

`features/wails3` is the active Wails3 implementation. The prior Tauri baseline is retained on `features/tauri` for reference and rollback.

## License

YTLite is distributed under the MIT License. See [LICENSE.md](LICENSE.md).
