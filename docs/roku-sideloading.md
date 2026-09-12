# Roku sideloading — Local Photo Screensaver (Phase 0)

## Package layout

Zip the **contents** of `roku-app/` so that `manifest` is at the **root** of the archive:

```
manifest
source/Main.brs
components/ScreensaverScene.xml
components/ScreensaverScene.brs
components/SettingsScene.xml
components/SettingsScene.brs
images/…
```

**Wrong:** zipping the `roku-app` folder itself so the zip contains `roku-app/manifest`.

### Zip on Windows (PowerShell)

```powershell
cd path\to\local-photo-screensaver\roku-app
Compress-Archive -Path * -DestinationPath ..\local-photo-screensaver.zip -Force
```

### Zip on macOS / Linux

```bash
cd roku-app
zip -r ../local-photo-screensaver.zip .
```

## Developer Mode on the Roku

1. On the Roku home screen, press **Home** three times, then **Up** twice, then **Right** once, then **Left** once, then **Right** once, then **Left** once, then **Right** once (classic secret sequence). Or use **Settings → System → Advanced system settings → Developer options** if already enabled.
2. Enable **Developer Mode**; accept the EULA.
3. Set a developer password when prompted.
4. Note the Roku’s **IP address** shown on that screen (or under **Settings → Network → About**).

## Web installer

1. On a PC on the same LAN, open `http://<roku-ip>` in a browser.
2. Log in with user `rokudev` and the developer password.
3. Under **Install utility**, choose your `.zip` and **Upload**.
4. The channel/screensaver package installs and may launch once.

## Set as system screensaver

1. Configure the server host IP via **screensaver settings** (`RunScreenSaverSettings`) — enter the PC LAN IP running `photoserver` on port `8787`.
2. **Settings → Screensaver →** select **Local Photo Screensaver**.
3. Optionally shorten wait time under screensaver settings for faster testing.

## Troubleshooting

| Symptom | Check |
|---------|--------|
| “Connecting…” forever | PC firewall / wrong IP / server not running |
| Playlist empty | `--photos` folder has JPEG/PNG; health endpoint OK |
| Sideload rejected | `manifest` must be zip root; `screensaver_title` present; no channel `title`-only package |
| Settings not saving | Registry write needs `flush()` (included); re-open settings |

## Notes

- Phase 0 uses synchronous `roUrlTransfer` for the small playlist JSON (spike simplicity).
- Placeholder icons under `images/` are solid-color PNGs for packaging; replace for any public build.
