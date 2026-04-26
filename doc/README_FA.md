[🇬🇧 English](/README.md) | [🇫🇷 Français](/doc/README_FR.md) | [🇯🇵 日本語](/doc/README_JP.md) | [🇨🇭 Schwiizerdütsch](/doc/README_DE_CH.md) | [🇮🇹 Italiano](/doc/README_IT.md) | [🇨🇳 中文](/doc/README_ZH.md) | [🇮🇳 हिन्दी](/doc/README_HI.md) | [🇮🇷 فارسی](/doc/README_FA.md) | [🇷🇺 Русский](/doc/README_RU.md) | [🇲🇳 Монгол](/doc/README_MN.md) | [🇰🇿 Қазақша](/doc/README_KK.md)

# Chicha Isotope Map (فارسی)

این نسخه به فارسی ترجمه شده و از نظر محتوا با README انگلیسیِ فعلی همگام است.

<div align="center">
  <img src="https://raw.githubusercontent.com/matveynator/chicha-isotope-map/main/public_html/images/chicha-isotope-map-round-logo.png" alt="Chicha Isotope Map logo" width="120" />

Download page for the latest stable build: <a href="https://matveynator.github.io/chicha-isotope-map/">matveynator.github.io/chicha-isotope-map</a> <br>
Radiacode, AtomFast, BGeigie Safecast devices supported.

[![Latest stable release build](https://github.com/matveynator/chicha-isotope-map/actions/workflows/release.yml/badge.svg)](https://github.com/matveynator/chicha-isotope-map/actions/workflows/release.yml)

[Live demo](https://pelora.org/)

</div>

## Downloads (Stable Release)

Use the **Stable Release** channel so one constant link always points to the newest binaries built from commits containing `stable release`.

- **Smart download page (recommended):** https://matveynator.github.io/chicha-isotope-map/
- **Stable Release tag (all artifacts):** https://github.com/matveynator/chicha-isotope-map/releases/tag/stable-release

### Desktop app (GUI)

| OS | Architecture / Variant | Artifact |
|---|---|---|
| macOS | Universal (Intel + Apple Silicon) | [chicha-isotope-map_darwin_universal_desktop.dmg](https://github.com/matveynator/chicha-isotope-map/releases/download/stable-release/chicha-isotope-map_darwin_universal_desktop.dmg) |
| Windows | amd64 | [chicha-isotope-map_windows_amd64_desktop.zip](https://github.com/matveynator/chicha-isotope-map/releases/download/stable-release/chicha-isotope-map_windows_amd64_desktop.zip) |
| Windows | arm64 | [chicha-isotope-map_windows_arm64_desktop.zip](https://github.com/matveynator/chicha-isotope-map/releases/download/stable-release/chicha-isotope-map_windows_arm64_desktop.zip) |
| Linux GTK 4.0 (Ubuntu 22.04 / Mint 21.x) | amd64 | [chicha-isotope-map_linux_amd64_desktop_gtk40.zip](https://github.com/matveynator/chicha-isotope-map/releases/download/stable-release/chicha-isotope-map_linux_amd64_desktop_gtk40.zip) |
| Linux GTK 4.0 (Ubuntu 22.04 / Mint 21.x) | arm64 | [chicha-isotope-map_linux_arm64_desktop_gtk40.zip](https://github.com/matveynator/chicha-isotope-map/releases/download/stable-release/chicha-isotope-map_linux_arm64_desktop_gtk40.zip) |
| Linux GTK 4.1 (Ubuntu 24.04+ / Mint 22+) | amd64 | [chicha-isotope-map_linux_amd64_desktop_gtk41.zip](https://github.com/matveynator/chicha-isotope-map/releases/download/stable-release/chicha-isotope-map_linux_amd64_desktop_gtk41.zip) |
| Linux GTK 4.1 (Ubuntu 24.04+ / Mint 22+) | arm64 | [chicha-isotope-map_linux_arm64_desktop_gtk41.zip](https://github.com/matveynator/chicha-isotope-map/releases/download/stable-release/chicha-isotope-map_linux_arm64_desktop_gtk41.zip) |

### Server binaries (self-hosted)

| OS | Architecture | Artifact |
|---|---|---|
| Linux | amd64 | [chicha-isotope-map_linux_amd64](https://github.com/matveynator/chicha-isotope-map/releases/download/stable-release/chicha-isotope-map_linux_amd64) |
| Linux | arm64 | [chicha-isotope-map_linux_arm64](https://github.com/matveynator/chicha-isotope-map/releases/download/stable-release/chicha-isotope-map_linux_arm64) |
| Windows | amd64 | [chicha-isotope-map_windows_amd64.exe](https://github.com/matveynator/chicha-isotope-map/releases/download/stable-release/chicha-isotope-map_windows_amd64.exe) |
| Windows | arm64 | [chicha-isotope-map_windows_arm64.exe](https://github.com/matveynator/chicha-isotope-map/releases/download/stable-release/chicha-isotope-map_windows_arm64.exe) |
| macOS | amd64 | [chicha-isotope-map_darwin_amd64](https://github.com/matveynator/chicha-isotope-map/releases/download/stable-release/chicha-isotope-map_darwin_amd64) |
| macOS | arm64 | [chicha-isotope-map_darwin_arm64](https://github.com/matveynator/chicha-isotope-map/releases/download/stable-release/chicha-isotope-map_darwin_arm64) |
| FreeBSD | amd64 / arm64 | [Stable Release assets](https://github.com/matveynator/chicha-isotope-map/releases/tag/stable-release) |
| OpenBSD | amd64 / arm64 | [Stable Release assets](https://github.com/matveynator/chicha-isotope-map/releases/tag/stable-release) |

### Quick run

Windows:
1. Download a `.zip` desktop build (or `.exe` server build).
2. Extract if needed.
3. Run the binary. If SmartScreen appears: **More info → Run anyway**.

macOS (server binary example):
```bash
chmod +x ./chicha-isotope-map_darwin_*
./chicha-isotope-map_darwin_*
```

Linux desktop (GTK 4.1 amd64 example):
```bash
curl -fL https://github.com/matveynator/chicha-isotope-map/releases/download/stable-release/chicha-isotope-map_linux_amd64_desktop_gtk41.zip -o chicha-isotope-map-desktop.zip
unzip chicha-isotope-map-desktop.zip
chmod +x ./chicha-isotope-map_linux_amd64_desktop_gtk41
./chicha-isotope-map_linux_amd64_desktop_gtk41
```

Linux server quick install:
```bash
sudo curl -fL https://github.com/matveynator/chicha-isotope-map/releases/download/stable-release/chicha-isotope-map_linux_amd64 -o /usr/local/bin/chicha-isotope-map
sudo chmod +x /usr/local/bin/chicha-isotope-map
/usr/local/bin/chicha-isotope-map
```
Open: http://localhost:8765

## Configuration and deployment

### Useful flags
- `-port 8765`
- `-domain your-domain.example`
- `-default-lat`, `-default-lon`, `-default-zoom`, `-default-layer`
- `-mapbox-token YOUR_TOKEN`
- `-setup` (Linux only)
- `-import-tgz-url URL`
- `-import-tgz-path /path/to/file.tgz`

### Database flags
- `-db-type sqlite|duckdb|chai|clickhouse|pgx`
- `-db-path /path/to/file`
- `-db-conn CONNECTION_STRING`

### Build from source

Desktop WebView builds require CGO.

```bash
CGO_ENABLED=1 go build -tags desktop .
./chicha-isotope-map -desktop
```

Server-only binary (no embedded desktop window):
```bash
CGO_ENABLED=0 go build .
./chicha-isotope-map
```

## Universal isotope catalog and spectrum drivers

The project now has a device-agnostic spectrum pipeline in `pkg/spectrum/` with a common driver interface and shared isotope analyzer/catalog.

## Acknowledgements

This project was conceived to grant people a clear and immediate understanding of radiation safety in the very places they inhabit.
