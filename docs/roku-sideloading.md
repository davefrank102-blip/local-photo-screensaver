# Roku sideloading

## Package

Zip the **contents** of `roku-app/` so the archive root contains `manifest`, `source/`, `components/`, `images/`.

**Wrong:** zipping the folder so the archive contains `roku-app/manifest`.

### PowerShell (forward-slash safe)

```powershell
cd roku-app
# Prefer .NET ZipFile with '/' separators — Compress-Archive may use '\' and Roku rejects the package
Add-Type -AssemblyName System.IO.Compression.FileSystem
$dest = Join-Path (Split-Path (Get-Location)) 'local-photo-screensaver.zip'
if (Test-Path $dest) { Remove-Item $dest -Force }
$zip = [IO.Compression.ZipFile]::Open($dest, 'Create')
Get-ChildItem -Recurse -File | ForEach-Object {
  $rel = $_.FullName.Substring((Get-Location).Path.Length + 1).Replace('\','/')
  [void][IO.Compression.ZipFileExtensions]::CreateEntryFromFile($zip, $_.FullName, $rel)
}
$zip.Dispose()
```

## Developer Mode

Home ×3 → Up ×2 → Right → Left → Right → Left → Right → enable Developer Mode, set password, note IP.

## Install

Browser → `http://<roku-ip>` → user `rokudev` → Upload zip (choose **zip**, not squashfs).

## Use

1. Open the home-tile app once to confirm settings / set server IP  
2. **Settings → Theme → Screensavers → Local Photo Screensaver → Set as screensaver**  
3. **Change screensaver settings** to edit the PC IP  

Screensavers do not appear as a normal channel after install if you only had `screensaver_title`; this package also includes a home tile for setup.
