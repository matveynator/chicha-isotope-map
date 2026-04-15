<div align="center">
  <img src="https://raw.githubusercontent.com/matveynator/chicha-isotope-map/main/public_html/images/chicha-isotope-map-round-logo.png" alt="Chicha Isotope Map logo" width="120" />

# Chicha Isotope Map

Simple download page for the latest stable build.

[![Latest stable release build](https://github.com/matveynator/chicha-isotope-map/actions/workflows/release.yml/badge.svg)](https://github.com/matveynator/chicha-isotope-map/actions/workflows/release.yml)
</div>

- [Stable release (always updated)](https://github.com/matveynator/chicha-isotope-map/releases/tag/stable-release)
- [Live demo](https://pelora.org/)

---

## 1) Desktop app (GUI)

Best for personal local use.

### Download
- [Desktop for Windows (amd64)](https://github.com/matveynator/chicha-isotope-map/releases/download/stable-release/chicha-isotope-map_windows_amd64_desktop.exe)
- [Desktop for macOS Apple Silicon (arm64)](https://github.com/matveynator/chicha-isotope-map/releases/download/stable-release/chicha-isotope-map_darwin_arm64_desktop)
- [Desktop for macOS Intel (amd64)](https://github.com/matveynator/chicha-isotope-map/releases/download/stable-release/chicha-isotope-map_darwin_amd64_desktop)
- [Desktop for Linux (amd64)](https://github.com/matveynator/chicha-isotope-map/releases/download/stable-release/chicha-isotope-map_linux_amd64_desktop)

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
curl -fL https://github.com/matveynator/chicha-isotope-map/releases/download/stable-release/chicha-isotope-map_linux_amd64_desktop -o chicha-isotope-map-desktop
chmod +x ./chicha-isotope-map-desktop
./chicha-isotope-map-desktop
```

### Screenshots (temporary placeholders)

<img width="100%" alt="desktop placeholder 1" src="https://github.com/user-attachments/assets/617a0ced-4280-41c2-9320-de1cfd33a61f" />
<img width="100%" alt="desktop placeholder 2" src="https://github.com/user-attachments/assets/13256b23-744d-4d02-a26c-ae9aef5b0d87" />

---

## 2) Server version (self-hosted)

Best for VPS or always-on node.

### Quick start (Linux)
```bash
curl -fL https://github.com/matveynator/chicha-isotope-map/releases/download/stable-release/chicha-isotope-map_linux_amd64 -o chicha-isotope-map
chmod +x ./chicha-isotope-map
./chicha-isotope-map
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

## 3) Advanced flags and PostgreSQL

### Useful flags
- `-port 8765`
- `-domain your-domain.example`
- `-default-lat`, `-default-lon`, `-default-zoom`, `-default-layer`
- `-mapbox-token YOUR_TOKEN`
- `-setup`
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

## 4) History and contributors

We built this project so people can quickly understand radiation safety in places where they live, work, farm, or collect water.

Chicha Isotope Map was inspired by **Dmitry Ignatenko** and strongly influenced by **Rob Oudendijk** and the **Safecast** community. Thanks to Safecast, AtomFast, Radiacode, DoseMap, and many open dosimetry contributors.

If this project helps protect even one person or animal, it is worth it.

---

## Languages
- [🇬🇧 English](/README.md)
- [🇫🇷 Français](/doc/README_FR.md)
- [🇯🇵 日本語](/doc/README_JP.md)
- [🇨🇭 Schwiizerdütsch](/doc/README_DE_CH.md)
- [🇮🇹 Italiano](/doc/README_IT.md)
- [🇨🇳 中文](/doc/README_ZH.md)
- [🇮🇳 हिन्दी](/doc/README_HI.md)
- [🇮🇷 فارسی](/doc/README_FA.md)
- [🇷🇺 Русский](/doc/README_RU.md)
- [🇲🇳 Монгол](/doc/README_MN.md)
- [🇰🇿 Қазақша](/doc/README_KK.md)
