# Local Photo Screensaver

Privacy-first **Roku system screensaver** + **local companion server**. Photos stay on your LAN. No cloud, no accounts, no ads, no telemetry.

**Phase 0** is a feasibility spike only: prove that a Go server on your PC can feed still JPEG/PNG images to a sideloaded Roku screensaver over HTTP on the LAN.

## Mission

- Photos never leave your local network.
- Companion server runs on a machine you control.
- Roku shows a fading slideshow using SceneGraph Posters (no video/audio).

## Repo layout

| Path | Purpose |
|------|---------|
| `server/` | Go companion (`photoserver`) |
| `roku-app/` | Sideloadable Roku screensaver package |
| `protocol/` | Minimal OpenAPI for the spike API |
| `docs/` | Sideloading and ops notes |

## Phase 0 — how to run

### 1. Run the Go server (Windows)

From `server/`:

```bat
go build -o photoserver.exe ./cmd/photoserver
photoserver.exe --photos "C:\Users\You\Pictures" --addr 0.0.0.0:8787
```

Cross-compile from Linux/macOS:

```bash
cd server
GOOS=windows GOARCH=amd64 go build -o photoserver.exe ./cmd/photoserver
```

- `--photos` is **required** — point at a folder of JPEG/PNG (scanned recursively).
- `--addr` defaults to `0.0.0.0:8787` (all interfaces). **Restrict to your LAN** with OS firewall; Phase 0 has **no authentication**. Prefer binding carefully and never expose this port to the public internet.
- Health check: `http://127.0.0.1:8787/api/v1/health`
- Playlist: `http://127.0.0.1:8787/api/v1/spike/playlist`

Do not commit large photo binaries. Use your own folder; `server/testdata/` is only a placeholder.

### 2. Sideload the Roku app

See [`docs/roku-sideloading.md`](docs/roku-sideloading.md). Summary:

1. Enable **Developer Mode** on the Roku.
2. Zip the contents of `roku-app/` (manifest at zip root — not a parent folder).
3. Upload via the Roku web installer (`http://<roku-ip>`).

### 3. Configure server IP + set as screensaver

1. Open the sideloaded channel’s **screensaver settings** (or channel settings) and enter your PC’s LAN IPv4 (registry key `LocalPhotoSpike` / `serverHost`). Default fallback in code: `192.168.1.10`.
2. On the Roku: **Settings → Screensaver →** pick **Local Photo Screensaver**.
3. Ensure the PC server is running and the Roku can reach `http://<pc-ip>:8787`.

## Phase 0 limitations

- No pairing / auth — anyone on the LAN who can reach the port can fetch images.
- Manual IP entry (no mDNS discovery yet).
- JPEG/PNG only; playlist capped (~50); shuffle in memory at request time.
- Opaque image IDs (hex hashes) — never raw filesystem paths.
- Spike API under `/api/v1/spike/…` — not a final protocol.

## License

Apache-2.0 — see [`LICENSE`](LICENSE).

## Security

See [`SECURITY.md`](SECURITY.md). Spike trusts the LAN only.
