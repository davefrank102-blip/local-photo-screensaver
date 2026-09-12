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

## Endpoints

- `GET /api/v1/health`
- `GET /api/v1/spike/playlist`
- `GET /api/v1/images/{id}`

Sample photos: point `--photos` at your own folder. `testdata/` is empty on purpose (no large binaries in git).
