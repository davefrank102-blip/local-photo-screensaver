# Decisions — Phase 0

## Go for the companion server

- Single static binary, easy Windows cross-compile (`GOOS=windows GOARCH=amd64`).
- Stdlib `net/http` + `image` is enough for a spike (no framework).
- Fast recursive scan and simple in-memory id→path map.

## No authentication in the spike

- Goal is feasibility (Roku ↔ LAN HTTP stills), not product hardening.
- Documented LAN-trust-only model; pairing/tokens deferred to a later phase.
- Default bind `0.0.0.0:8787` with explicit README/firewall warnings.

## Opaque image IDs

- IDs are hex digests of the relative path (SHA-256 truncated), never raw paths.
- Rejects non-hex IDs and `..` traversal on the image route.
- Defense-in-depth: resolved file must remain under `--photos` root.

## SceneGraph `rsg_version=1.3`

- Widely available on modern Roku OS builds.
- Two Poster nodes + opacity cross-fade keeps memory light (stills only).

## Manifest: `screensaver_title` only

- Screensaver packages must use `screensaver_title` (not channel `title`).
- No `RunUserInterface` / channel Main UI — entry is `RunScreenSaver()` only.
- Optional `RunScreenSaverSettings()` stores server host IP in the registry.

## Spike playlist endpoint

- `/api/v1/spike/playlist` is explicitly a temporary shape for the feasibility demo.
- Cap + shuffle keep responses small for the Roku URL transfer path used in Phase 0.
