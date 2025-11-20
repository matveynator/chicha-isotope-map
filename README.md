[![Latest stable release build](https://github.com/matveynator/chicha-isotope-map/actions/workflows/release.yml/badge.svg)](https://github.com/matveynator/chicha-isotope-map/actions/workflows/release.yml)

<img width="30%" align="left" alt="chicha-isotope-map" src="https://github.com/user-attachments/assets/39bfa7b1-03fb-43dd-89bd-8d6c516fd4db" />

- [🇬🇧 English](/README.md)
- [🇫🇷 Français](/doc/README_FR.md)
- [🇯🇵 日本語](/doc/README_JP.md)
- [🇷🇺 Русский](/doc/README_RU.md)

# ☢️ World Radiation Map
We keep this map humble and clear, in the spirit of Dmitry Likhachov: a newcomer should instantly see whether radiation is nearby — where they live, grow food, gather mushrooms and herbs, herd cattle, or draw water. In most clean forests, fields, and rivers the background stays near 2–3 µR/h; anything above is usually man-made. You can trace how uranium mines in Czechia, Russia, Kazakhstan, and Mongolia left long scars; how Fukushima created a dark blotch; how Chernobyl and the Bryansk region became “tumors” on the map; how radon-rich seams in France, Czechia, and the Caucasian Mineral Waters raise the risk of lung and stomach cancer. Leaching uranium and rare earths leaves soluble salts underground; they slip into aquifers and then into our water and food. If this map protects even one person or animal, it was worth building.

Live demo: [https://pelora.org/](https://pelora.org/) — your node will look the same.

👉 [Unified download page](https://github.com/matveynator/chicha-isotope-map/releases) (all platforms, latest builds)

👉 [DeepWiki: Chicha Isotope Map](https://deepwiki.com/matveynator/chicha-isotope-map)

---

### 📸 Example view
<a href="https://pelora.org" target="_blank"><img width="800" alt="pelora.org chicha-isotope-map example" src="https://github.com/user-attachments/assets/be706959-a2d5-4949-9378-811f4022aa98" /></a>

---

## 🧭 What’s inside
- A live map of measurements from many detectors; pick the layer you like.
- Upload your own tracks; fresh points pop up around the place you view.
- Import via URL or file, export as an archive.
- Run as a single node or join a network: more nodes → more transparency.

The project grows with active help from the community: many great ideas came from **Rob Alden** and friends in open dosimetry worldwide (thank you, Greenpeace and other environmental teams).

---

## 🚀 Quick start (beginner friendly)
Fastest path: download the binary. No Docker, no databases, no extra tools — download, run, done.

### Option 1. Binary (recommended)
1) Open the [releases page](https://github.com/matveynator/chicha-isotope-map/releases) and download the build for your system.
2) Make it executable and run:
```bash
chmod +x ./chicha-isotope-map
./chicha-isotope-map
```
3) Open [http://localhost:8765](http://localhost:8765) — the map is already live.

Optional knobs:
- `-port 8765` — local port.
- `-domain maps.example.org` — HTTPS with Let’s Encrypt (needs 80/443).
- `-default-lat` / `-default-lon` / `-default-zoom` / `-default-layer` — opening map view.
- Storage: `-db-type sqlite|duckdb|chai|clickhouse|pgx`, `-db-path` for file databases, `-db-conn` for network ones.

### Option 2. Public node with a domain
1) Run the binary with your domain:
```bash
./chicha-isotope-map -domain example.org
```
2) Keep ports 80/443 open for Let’s Encrypt. After issuance, the map is at [https://example.org](https://example.org).

### Option 3. Docker (all packaged)
1) Install Docker (Desktop or CLI).
2) Find **matveynator/chicha-isotope-map** on Docker Hub and click **Run** (or execute one command):
```bash
docker run -d -p 8765:8765 --name chicha-isotope-map matveynator/chicha-isotope-map:latest
```
3) Open [http://localhost:8765](http://localhost:8765) — that’s it.

---

## 📥 Import data
- Ready-made database: a full archive is on [pelora.org](https://pelora.org/); point the loader at its URL or download and add via **Upload**.
- Web import: **Upload** → pick files (`.kml`, `.kmz`, `.json`, `.rctrk`, `.csv`, `.gpx`, bGeigie Nano/Zen `$BNRDD`, AtomFast, RadiaCode, Safecast, etc.).
- API import: `curl -F 'files[]=@/path/to/file.log' http://localhost:8765/upload` (diagnostics: `/upload_diag`).

## 📤 Export
- Single track: `/api/track/{trackID}.json` (legacy `.cim` also works).
- Scheduled archive: `/api/json/weekly.tgz` (or `/daily.tgz`, `/monthly.tgz`, `/yearly.tgz`). Inside: one JSON per track.

---

## 🧠 Advanced options
- Databases: built-in SQLite by default; switch to DuckDB, Chai, ClickHouse, or PostgreSQL (`pgx`).
- Import: via URL or file, archives accepted.
- Export: JSON archives, single track, old `.cim` still supported.
- Appearance: starting coordinates and layer (`-default-*`).

---

## 🤝 Why your own node & a bit of history
- We wanted anyone, without training, to see if radiation threatens where they live, grow food, or collect water.
- Your node gives a baseline and history (typically 0.8–4 µR/h), so deviations stand out.
- The more nodes exist, the harder it is to miss contamination.

Chicha-Isotope-Map was created for the **Dmitry Ignatenko laboratory**, inspired by **Safecast**, and fueled by open data from the AtomFast and Radiacode communities. If the map spares even one life, it was not in vain.
