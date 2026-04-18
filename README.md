
[🇬🇧 English](/README.md) | [🇫🇷 Français](/doc/README_FR.md) | [🇯🇵 日本語](/doc/README_JP.md) | [🇨🇭 Schwiizerdütsch](/doc/README_DE_CH.md) | [🇮🇹 Italiano](/doc/README_IT.md) | [🇨🇳 中文](/doc/README_ZH.md) | [🇮🇳 हिन्दी](/doc/README_HI.md) | [🇮🇷 فارسی](/doc/README_FA.md) | [🇷🇺 Русский](/doc/README_RU.md) | [🇲🇳 Монгол](/doc/README_MN.md) | [🇰🇿 Қазақша](/doc/README_KK.md)

<div align="center">
  <img src="https://raw.githubusercontent.com/matveynator/chicha-isotope-map/main/public_html/images/chicha-isotope-map-round-logo.png" alt="Chicha Isotope Map logo" width="120" />

# Chicha Isotope Map

Download page for the latest stable build: <a href="https://matveynator.github.io/chicha-isotope-map/doc/pages/">matveynator.github.io/chicha-isotope-map/doc/pages</a>. <br>
Radiacode, AtomFast, BGeigie Safecast devices supported.

[![Latest stable release build](https://github.com/matveynator/chicha-isotope-map/actions/workflows/release.yml/badge.svg)](https://github.com/matveynator/chicha-isotope-map/actions/workflows/release.yml)


[Live demo](https://pelora.org/)

</div>


## Desktop app (GUI)

Best for personal local use.

### Download
- [Desktop for Windows (amd64)](https://github.com/matveynator/chicha-isotope-map/releases/download/stable-release/chicha-isotope-map_windows_amd64_desktop.exe)
- [Desktop for Windows (arm64)](https://github.com/matveynator/chicha-isotope-map/releases/download/stable-release/chicha-isotope-map_windows_arm64_desktop.exe)
- [Desktop for macOS Apple Silicon (arm64)](https://github.com/matveynator/chicha-isotope-map/releases/download/stable-release/chicha-isotope-map_darwin_arm64_desktop)
- [Desktop for macOS Intel (amd64)](https://github.com/matveynator/chicha-isotope-map/releases/download/stable-release/chicha-isotope-map_darwin_amd64_desktop)
- [Desktop for Linux (amd64, single-file AppImage)](https://github.com/matveynator/chicha-isotope-map/releases/download/stable-release/chicha-isotope-map_linux_amd64_desktop.AppImage)
- [Desktop for Linux (arm64, single-file AppImage)](https://github.com/matveynator/chicha-isotope-map/releases/download/stable-release/chicha-isotope-map_linux_arm64_desktop.AppImage)

### Run
Windows:
1. Download `.exe`
2. Double click
3. If SmartScreen appears: **More info → Run anyway**

macOS:
```bash
chmod +x ./chicha-isotope-map_darwin_*_desktop
./chicha-isotope-map_darwin_*_desktop
```

Linux:
```bash
curl -fL https://github.com/matveynator/chicha-isotope-map/releases/download/stable-release/chicha-isotope-map_linux_amd64_desktop.AppImage -o chicha-isotope-map-desktop.AppImage
chmod +x ./chicha-isotope-map-desktop.AppImage
./chicha-isotope-map-desktop.AppImage
```
Linux desktop release is now a single file (`.AppImage`) with embedded app metadata and icon.

### Build desktop from source

Desktop WebView builds require CGO.

macOS/Linux:
```bash
CGO_ENABLED=1 go build -tags desktop .
./chicha-isotope-map -desktop
```

Server-only binary (no embedded desktop window):
```bash
CGO_ENABLED=0 go build .
./chicha-isotope-map
```

### Screenshots:

<img width="100%" alt="desktop placeholder 1" src="https://github.com/user-attachments/assets/617a0ced-4280-41c2-9320-de1cfd33a61f" />
<img width="100%" alt="desktop placeholder 2" src="https://github.com/user-attachments/assets/13256b23-744d-4d02-a26c-ae9aef5b0d87" />

---

## Server version (self-hosted)

### Quick start (Linux)
```bash
sudo curl -fL https://github.com/matveynator/chicha-isotope-map/releases/download/stable-release/chicha-isotope-map_linux_amd64 -o /usr/local/bin/chicha-isotope-map; chmod +x /usr/local/bin/chicha-isotope-map; /usr/local/bin/chicha-isotope-map;
```
Open: [http://localhost:8765](http://localhost:8765)

### Other server binaries
- [Server for Windows (amd64)](https://github.com/matveynator/chicha-isotope-map/releases/download/stable-release/chicha-isotope-map_windows_amd64.exe)
- [Server for macOS arm64](https://github.com/matveynator/chicha-isotope-map/releases/download/stable-release/chicha-isotope-map_darwin_arm64)
- [Server for macOS amd64](https://github.com/matveynator/chicha-isotope-map/releases/download/stable-release/chicha-isotope-map_darwin_amd64)
- [All release assets (FreeBSD/OpenBSD/arm64/etc)](https://github.com/matveynator/chicha-isotope-map/releases/tag/stable-release)

### Public HTTPS domain
```bash
./chicha-isotope-map -domain your-domain.example
```
Ports 80/443 must be open.

---

## Advanced flags and PostgreSQL

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

### PostgreSQL example
```bash
./chicha-isotope-map \
  -db-type pgx \
  -db-conn 'postgres://USER:PASSWORD@HOST:5432/chicha?sslmode=allow' \
  -port 8765
```

### Preload real tracks once
```bash
./chicha-isotope-map -import-tgz-url https://pelora.org/api/json/weekly.tgz
```

---

## History and contributors

This project was conceived to grant people a clear and immediate understanding of radiation safety in the very places they inhabit—where they reside, labor, cultivate the land, and draw water.

The Chicha Isotope Map finds its roots in the field research of [Dmitry Ignatenko](https://www.youtube.com/@MrDrimogemon) and has been profoundly shaped by the insights of Rob Oudendijk and the [Safecast community](https://safecast.org). We extend our sincere appreciation to [Safecast](https://simplemap.safecast.org), [AtomFast](https://atomfast.net), [Radiacode](https://radiacode.com), [DoseMap](https://dosemap.org), and the many contributors to open dosimetry whose efforts made this possible.

Should this work serve to safeguard even a single living being, its purpose shall be fully justified.
