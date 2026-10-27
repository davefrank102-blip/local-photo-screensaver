# photoserver (Phase 0)

LAN companion for Local Photo Screensaver.

## Build (Windows exe)

```bash
GOOS=windows GOARCH=amd64 go build -o photoserver.exe ./cmd/photoserver
```

On Windows with Go installed:

```bat
go build -o photoserver.exe ./cmd/photoserver
```

## Run

```bat
photoserver.exe --photos "C:\path\to\photos" --addr 0.0.0.0:8787
```

Flags:

| Flag | Default | Notes |
|------|---------|--------|
| `--photos` | _(required)_ | Root directory; recursive JPEG/PNG only |
| `--addr` | `0.0.0.0:8787` | **LAN-only** — firewall this; no auth in Phase 0 |
| `--cache-max-mb` | `1024` (or `LPS_CACHE_MAX_MB`) | Size cap for the oriented-image cache; least-recently-used files are evicted first. `0` = unlimited |

### Oriented-image cache

Resized/auto-rotated JPEGs are cached in `%LOCALAPPDATA%\LocalPhotoScreensaver\orient-cache`. On startup the server deletes entries for photos no longer in the library (or whose file changed) and leftover temp files, then enforces the size cap and logs how much it freed. The cap is re-checked every 5 minutes and after every 50 new cache writes, in the background.

## Endpoints

- `GET /api/v1/health`
- `GET /api/v1/spike/playlist`
- `GET /api/v1/images/{id}`

Sample photos: point `--photos` at your own folder. `testdata/` is empty on purpose (no large binaries in git).
