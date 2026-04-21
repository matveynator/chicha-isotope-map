
[🇬🇧 English](/README.md) | [🇫🇷 Français](/doc/README_FR.md) | [🇯🇵 日本語](/doc/README_JP.md) | [🇨🇭 Schwiizerdütsch](/doc/README_DE_CH.md) | [🇮🇹 Italiano](/doc/README_IT.md) | [🇨🇳 中文](/doc/README_ZH.md) | [🇮🇳 हिन्दी](/doc/README_HI.md) | [🇮🇷 فارسی](/doc/README_FA.md) | [🇷🇺 Русский](/doc/README_RU.md) | [🇲🇳 Монгол](/doc/README_MN.md) | [🇰🇿 Қазақша](/doc/README_KK.md)

<div align="center">
  <img src="https://raw.githubusercontent.com/matveynator/chicha-isotope-map/main/public_html/images/chicha-isotope-map-round-logo.png" alt="Chicha Isotope Map logo" width="120" />

# Chicha Isotope Map

Download page for the latest stable build: <a href="https://matveynator.github.io/chicha-isotope-map/">matveynator.github.io/chicha-isotope-map</a> <br>
Radiacode, AtomFast, BGeigie Safecast devices supported.

[![Latest stable release build](https://github.com/matveynator/chicha-isotope-map/actions/workflows/release.yml/badge.svg)](https://github.com/matveynator/chicha-isotope-map/actions/workflows/release.yml)


[Live demo](https://pelora.org/)

</div>


## Desktop app (GUI)

Best for personal local use.

### Download
- [Desktop for Windows (amd64)](https://github.com/matveynator/chicha-isotope-map/releases/download/stable-release/chicha-isotope-map_windows_amd64_desktop.exe)
- [Desktop for Windows (arm64)](https://github.com/matveynator/chicha-isotope-map/releases/download/stable-release/chicha-isotope-map_windows_arm64_desktop.exe)
- [Desktop for Windows (amd64, zip)](https://github.com/matveynator/chicha-isotope-map/releases/download/stable-release/chicha-isotope-map_windows_amd64_desktop.zip)
- [Desktop for Windows (arm64, zip)](https://github.com/matveynator/chicha-isotope-map/releases/download/stable-release/chicha-isotope-map_windows_arm64_desktop.zip)
- [Desktop for macOS Apple Silicon (arm64)](https://github.com/matveynator/chicha-isotope-map/releases/download/stable-release/chicha-isotope-map_darwin_arm64_desktop)
- [Desktop for macOS Intel (amd64)](https://github.com/matveynator/chicha-isotope-map/releases/download/stable-release/chicha-isotope-map_darwin_amd64_desktop)
- [Desktop for Linux (amd64, Mint 21 / Ubuntu 22.04, zip)](https://github.com/matveynator/chicha-isotope-map/releases/download/stable-release/chicha-isotope-map_linux_amd64_desktop_gtk40.zip)
- [Desktop for Linux (amd64, Mint 22 / Ubuntu 24.04+, zip)](https://github.com/matveynator/chicha-isotope-map/releases/download/stable-release/chicha-isotope-map_linux_amd64_desktop_gtk41.zip)
- [Desktop for Linux (arm64, Mint 21 / Ubuntu 22.04, zip)](https://github.com/matveynator/chicha-isotope-map/releases/download/stable-release/chicha-isotope-map_linux_arm64_desktop_gtk40.zip)
- [Desktop for Linux (arm64, Mint 22 / Ubuntu 24.04+, zip)](https://github.com/matveynator/chicha-isotope-map/releases/download/stable-release/chicha-isotope-map_linux_arm64_desktop_gtk41.zip)

### Run
Windows:
1. Download `.exe` directly (or `.zip` if your browser/security policy blocks direct `.exe` downloads)
2. Double click
3. If SmartScreen appears: **More info → Run anyway**

macOS:
```bash
chmod +x ./chicha-isotope-map_darwin_*_desktop
./chicha-isotope-map_darwin_*_desktop
```

Linux:
```bash
curl -fL https://github.com/matveynator/chicha-isotope-map/releases/download/stable-release/chicha-isotope-map_linux_amd64_desktop_gtk41.zip -o chicha-isotope-map-desktop.zip
unzip chicha-isotope-map-desktop.zip
chmod +x ./chicha-isotope-map_linux_amd64_desktop_gtk41
./chicha-isotope-map_linux_amd64_desktop_gtk41
```
Linux desktop release now ships as a regular executable binary in a `.zip` archive.
Use `*_gtk40` for Ubuntu 22.04 / Mint 21 and `*_gtk41` for Ubuntu 24.04+ / Mint 22 (both amd64 and arm64).

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

## Offline map download roadmap (desktop + server)

This section describes a practical design for full offline work: user selects an area, app downloads required map layers and track overlays, then renders everything without internet.

### Goals
- One offline workflow for both modes:
  - Desktop app (`-desktop`)
  - Server mode opened in browser
- Deterministic cache package: selected bounds + selected zoom range + selected layers.
- Explicit storage limits and predictable eviction.

### 1) Data model for offline region

Persist one `offline_region` manifest per user action:
- `region_id` (UUID)
- `created_at`
- `bbox` (`minLon,minLat,maxLon,maxLat`)
- `zoom_min`, `zoom_max`
- `base_layers[]` (e.g., OSM, satellite)
- `overlay_layers[]` (track heatmap, markers, optional tiles)
- `tile_count_estimate`, `byte_estimate`
- `status` (`queued|downloading|ready|failed|canceled`)

Each manifest points to downloaded tile and overlay records. Keep manifests immutable after `ready`; update by creating a new region version.

### 2) Client storage (IndexedDB first)

Use IndexedDB as the primary browser-side storage for both desktop WebView and server-browser sessions:

- DB: `chicha_offline_v1`
- Stores:
  - `offline_regions` (manifest metadata)
  - `tiles` (key: `source|z|x|y|styleVersion`, value: binary blob + metadata)
  - `overlays` (vector chunks / compressed GeoJSON for tracks)
  - `jobs` (download progress, retry counters)

Recommended indexes:
- `tiles.by_source_zxy`
- `tiles.by_last_access`
- `tiles.by_region_id`
- `jobs.by_status`

Why IndexedDB:
- Works in browser and desktop WebView with one code path.
- Blob storage is efficient enough for map tiles.
- Can stream chunks and resume interrupted downloads.

### 3) Server-side cache mirror

For server deployments, add a disk cache so offline packs survive browser storage resets:

- Directory layout:
  - `data/offline/regions/<region_id>/manifest.json`
  - `data/offline/tiles/<source>/<z>/<x>/<y>.tile`
  - `data/offline/overlays/<region_id>/<chunk_id>.bin`
- Optional metadata table in existing SQL DB:
  - `offline_regions`
  - `offline_region_tiles`
  - `offline_region_overlays`

Client still uses IndexedDB for fast read. Server cache is authoritative backup and supports sharing one prepared region with many users.

### 4) Area selection and tile enumeration

User flow:
1. Click **Offline mode**
2. Draw rectangle / polygon
3. Choose zoom range and layers
4. See estimate (`N tiles`, `~MB/GB`)
5. Confirm download

Tile enumeration algorithm:
- Convert selected geometry to XYZ tile ranges for each zoom level.
- If polygon mode is used, keep only tiles intersecting polygon.
- Deduplicate by tile key across layers that share source/version.
- Emit a deterministic queue (`source,z,x,y`) sorted by:
  1. lower zoom first (faster coarse preview),
  2. then distance from map center.

### 5) Download pipeline (channel-oriented)

Use a pipeline model for Go + frontend workers:

`enumerate -> fetch -> verify -> persist -> index -> progress`

Rules:
- Bounded worker pool per source host (avoid bans).
- Retry with exponential backoff for transient failures.
- Verify content type + non-empty body before persist.
- Persist atomically (temp key/file then commit).
- Progress events over WebSocket:
  - `offline_job_started`
  - `offline_tile_saved`
  - `offline_overlay_saved`
  - `offline_job_failed`
  - `offline_job_completed`

### 6) Read path in offline mode

At render time:
1. Try IndexedDB tile/overlay.
2. If miss and server mode enabled: ask server offline cache endpoint.
3. If still miss and online allowed: fetch network and optionally backfill cache.
4. If strict offline enabled: return explicit placeholder + log miss counter.

This keeps behavior explicit and debuggable.

### 7) Eviction and quotas

Required controls:
- Global byte quota for IndexedDB cache.
- Optional per-region quota.
- LRU eviction on `tiles.by_last_access`.
- “Pin region” flag to prevent eviction.
- Pre-download warning if estimated size exceeds remaining budget.

### 8) Overlay/track offline strategy

Tracks should not rely on live API when region is offline-ready.

Recommended approach:
- During region download, query track points intersecting selected geometry.
- Chunk by geohash/S2 cell or tile-aligned vector chunks.
- Store compressed payload (gzip/brotli) in `overlays`.
- Build local spatial index (`cell_id -> chunk_ids`).

Render path loads only visible chunks for current viewport.

### 9) Minimal incremental implementation plan

Phase 1:
- Rectangle selection, base map tiles only, IndexedDB storage, manual delete.

Phase 2:
- Add overlays/tracks download and local chunk index.

Phase 3:
- Add server disk mirror and rehydrate endpoint.

Phase 4:
- Add polygon selection, pinning, quota UI, and integrity checker.

### 10) API contracts to add

- `POST /api/offline/estimate`
  - input: bbox/polygon + zooms + layers
  - output: tile count + byte estimate
- `POST /api/offline/jobs`
  - start download job
- `GET /api/offline/jobs/:id`
  - progress state
- `DELETE /api/offline/regions/:id`
  - delete region
- `GET /api/offline/tiles/:source/:z/:x/:y`
  - server cache fallback

### 11) Why this architecture fits the current project

- Keeps one UX across desktop and server variants.
- Uses web-standard client storage (IndexedDB) with no platform-specific branching.
- Preserves fast startup and predictable behavior during no-network operation.
- Scales from single-user desktop to multi-user hosted server.

---

## History and contributors

This project was conceived to grant people a clear and immediate understanding of radiation safety in the very places they inhabit—where they reside, labor, cultivate the land, and draw water.

The Chicha Isotope Map finds its roots in the field research of [Dmitry Ignatenko](https://www.youtube.com/@MrDrimogemon) and has been profoundly shaped by the insights of Rob Oudendijk and the [Safecast community](https://safecast.org). We extend our sincere appreciation to [Safecast](https://simplemap.safecast.org), [AtomFast](https://atomfast.net), [Radiacode](https://radiacode.com), [DoseMap](https://dosemap.org), and the many contributors to open dosimetry whose efforts made this possible.

Should this work serve to safeguard even a single living being, its purpose shall be fully justified.
