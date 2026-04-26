[🇬🇧 English](/README.md) | [🇫🇷 Français](/doc/README_FR.md) | [🇯🇵 日本語](/doc/README_JP.md) | [🇨🇭 Schwiizerdütsch](/doc/README_DE_CH.md) | [🇮🇹 Italiano](/doc/README_IT.md) | [🇨🇳 中文](/doc/README_ZH.md) | [🇮🇳 हिन्दी](/doc/README_HI.md) | [🇮🇷 فارسی](/doc/README_FA.md) | [🇷🇺 Русский](/doc/README_RU.md) | [🇲🇳 Монгол](/doc/README_MN.md) | [🇰🇿 Қазақша](/doc/README_KK.md)

<div align="center">
  <img src="https://raw.githubusercontent.com/matveynator/chicha-isotope-map/main/public_html/images/chicha-isotope-map-round-logo.png" alt="Chicha Isotope Map logo" width="120" />

# Chicha Isotope Map

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

Pick your OS first, then architecture.

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

---

## Program screenshots

Screenshots are taken from the official stable download page:

<img src="https://matveynator.github.io/chicha-isotope-map/chicha-isotope-map-macosx-universal.png">
<img src="https://matveynator.github.io/chicha-isotope-map/windows-google-light.png">
<img src="https://matveynator.github.io/chicha-isotope-map/linux-amd64-desktop.png">

---
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

### PostgreSQL example
```bash
./chicha-isotope-map \
  -db-type pgx \
  -db-conn 'postgres://USER:PASSWORD@HOST:5432/chicha?sslmode=allow' \
  -port 8765
```

### Public HTTPS domain
```bash
./chicha-isotope-map -domain your-domain.example
```
Ports `80/443` must be open.

### Preload real tracks once
```bash
./chicha-isotope-map -import-tgz-url https://pelora.org/api/json/weekly.tgz
```

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

---

## Universal isotope catalog and spectrum drivers

The project now has a device-agnostic spectrum pipeline in `pkg/spectrum/`:

- Common `Driver` interface for instrument parsers (`CanParse` + `Parse`).
- Built-in Radiacode XML driver as the first implementation.
- Shared `SpectrumMeasurement` model so AtomSpectra and other devices can be plugged in without rewriting isotope analysis.
- Shared analyzer that detects peaks and maps alpha/beta/gamma energies to nuclides from one common catalog.
- Composite-spectrum estimator that ranks two- and three-nuclide mixtures when peaks overlap in scintillation detectors.

Catalog scope in the built-in dataset:
- Cosmogenic and primordial isotopes (`H-3`, `C-14`, `K-40`, etc.).
- Full natural decay families used in field dosimetry:
  - Thorium-232 chain,
  - Uranium-238 chain,
  - Uranium-235 (actinium) chain,
  - Neptunium-237 family.
- Common environmental/industrial/medical/fallout isotopes (`Cs-137`, `Co-60`, `I-131`, `Am-241`, `Eu-152`, ...).
- Heavy transuranics up to `Og-294` for completeness of identifier parsing.

This gives one universal parser entrypoint and one extensible isotope catalog, while keeping drivers isolated by format.

---

## Acknowledgements

This project was conceived to grant people a clear and immediate understanding of radiation safety in the very places they inhabit—where they reside, labor, cultivate the land, and draw water.

The Chicha Isotope Map finds its roots in the field research of [Dmitry Ignatenko](https://www.youtube.com/@MrDrimogemon) and has been profoundly shaped by the insights of Rob Oudendijk and the [Safecast community](https://safecast.org). We extend our sincere appreciation to [Safecast](https://simplemap.safecast.org), [AtomFast](https://atomfast.net), [Radiacode](https://radiacode.com), [DoseMap](https://dosemap.org), and the many contributors to open dosimetry whose efforts made this possible.

Should this work serve to safeguard even a single living being, its purpose shall be fully justified.
