# Installation (Windows + Roku)

## Requirements

- Windows 10/11 PC on the same LAN as your Roku
- Go 1.22+ **or** a prebuilt `photoserver.exe` from [Releases](https://github.com/davefrank102-blip/local-photo-screensaver/releases)
- Roku with Developer Mode enabled (sideload)

## Companion server

```powershell
cd server
go build -o photoserver.exe ./cmd/photoserver
.\photoserver.exe --addr 0.0.0.0:8787
```

Open http://127.0.0.1:8787/ and add photo folders. Optional one-click: create a shortcut that runs `photoserver.exe` then opens that URL.

### Firewall

Allow inbound TCP **8787** (Private/Public as needed):

```powershell
New-NetFirewallRule -DisplayName "Local Photo Screensaver (TCP 8787)" -Direction Inbound -Action Allow -Protocol TCP -LocalPort 8787 -Profile Private,Public
```

Verify from a phone on Wi‑Fi: `http://<pc-lan-ip>:8787/api/v1/spike/playlist`

## Roku

See [roku-sideloading.md](roku-sideloading.md). Enter the PC LAN IP in screensaver settings (default placeholder in the app is `192.168.1.10` — change it).
