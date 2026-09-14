# Local Photo Screensaver

Privacy-first **Roku screensaver** that shows photos from your own PC/NAS over your LAN. Photos never upload to the cloud.

| Piece | What it is |
| --- | --- |
| `server/` | Go companion (`photoserver`) — indexes JPEG/PNG, serves playlist + images, local admin UI |
| `roku-app/` | Sideloadable Roku SceneGraph screensaver |
| `docs/` | Install, sideload, privacy, troubleshooting |

**Status:** usable MVP (Windows companion + Roku sideload). Not a Roku Channel Store submission yet.

## Features

- LAN-only photo serving (opaque image IDs; paths stay on the PC)
- Admin UI on the PC (`http://127.0.0.1:8787/`) — pick folders/years, shuffle vs in-order, seconds per slide
- Playlist paging (50 at a time, prefetch before batch end)
- EXIF auto-orientation (+ resize ~1920px) with on-disk cache
- Heuristic “junk” cleaner (screenshots/docs) — exclude by default, Keep / Delete in UI
- Dual package: home-screen settings tile + system screensaver

## Quick start (Windows)

### 1. Run the companion server

```powershell
cd server
go build -o photoserver.exe ./cmd/photoserver
.\photoserver.exe --addr 0.0.0.0:8787
```

Or cross-compile from Linux/macOS:

```bash
cd server
GOOS=windows GOARCH=amd64 go build -o photoserver.exe ./cmd/photoserver
```

Open **http://127.0.0.1:8787/** on the same PC:

1. Pick photo folders / year folders
2. Note your PC’s LAN IP (e.g. from `ipconfig`)
3. Allow inbound **TCP 8787** in Windows Firewall for Private/Public as needed

Optional seed folder:

```powershell
.\photoserver.exe --photos "D:\Photos" --addr 0.0.0.0:8787
```

### 2. Sideload the Roku app

Zip the **contents** of `roku-app/` so `manifest` is at the zip root (not a nested `roku-app/` folder). See [docs/roku-sideloading.md](docs/roku-sideloading.md).

1. Enable Developer Mode on the Roku
2. Open `http://<roku-ip>` → upload the zip (compression: zip)
3. Settings → Theme → Screensavers → set **Local Photo Screensaver**
4. Screensaver settings → enter the **PC LAN IP** (port `8787` is implied)

## Privacy model

- Photos stay on your machine; only playlist metadata and image bytes go to devices on your LAN
- Admin / folder-picker APIs are **localhost-only**
- Playlist and image APIs are **unauthenticated** in this MVP — treat your Wi‑Fi as the trust boundary (firewall recommended)
- Details: [docs/privacy.md](docs/privacy.md), [SECURITY.md](SECURITY.md)

## Repo layout

```
server/cmd/photoserver/   # companion server
roku-app/                 # Roku package sources
docs/                     # human docs
protocol/                 # API notes
```

## License

Apache-2.0 — see [LICENSE](LICENSE).
