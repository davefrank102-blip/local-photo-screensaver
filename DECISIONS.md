# Decisions

## Companion language: Go
Single static Windows `.exe`, easy cross-compile, good HTTP + image libs.

## No auth on playlist/images (MVP)
LAN trust + firewall. Admin UI is localhost-only. Auth/pairing is future work.

## Opaque image IDs
Playlist exposes hashed IDs, not filesystem paths.

## Roku: SceneGraph screensaver + settings tile
`screensaver_title` + `RunScreenSaver` / `RunScreenSaverSettings`, plus `title` + `Main` so sideload shows a home tile for setup.

## Playlist paging (50) + early prefetch
Keep Roku memory light; fetch next page ~3 slides before batch end.

## EXIF auto-orient on serve
Correct orientation server-side; cache under LocalAppData; cap long edge ~1920 for LAN.

## Junk cleaner = heuristics first
Filename / EXIF / aspect / mostly-white — exclude by default; optional delete. Local ML deferred.
