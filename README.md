[![Latest stable release build](https://github.com/matveynator/chicha-isotope-map/actions/workflows/release.yml/badge.svg)](https://github.com/matveynator/chicha-isotope-map/actions/workflows/release.yml)

<img width="30%" align="left" alt="chicha-isotope-map" src="https://github.com/user-attachments/assets/39bfa7b1-03fb-43dd-89bd-8d6c516fd4db" />

- [🇬🇧 English](/README.md)
- [🇫🇷 Français](/doc/README_FR.md)
- [🇯🇵 日本語](/doc/README_JP.md)
- [🇷🇺 Русский](/doc/README_RU.md)
- [🇨🇭 Schwiizerdütsch](/doc/README_DE_CH.md)
- [🇮🇹 Italiano](/doc/README_IT.md)
- [🇨🇳 中文](/doc/README_ZH.md)
- [🇮🇳 हिन्दी](/doc/README_HI.md)
- [🇮🇷 فارسی](/doc/README_FA.md)
- [🇲🇳 Монгол](/doc/README_MN.md)
- [🇰🇿 Қазақша](/doc/README_KK.md)

# ☢️ World Radiation Map
This map is built so an unprepared visitor can instantly see whether radiation threatens nearby homes, fields, forests, and water. Clean places stay near 2–3 µR/h; the darker spots are almost always human-made. The map shows how uranium mines in Czechia, Russia, Kazakhstan, and Mongolia left long traces; how Fukushima stands out as a black-red “tumor” on Japan’s coast; how Chernobyl and the Bryansk region scar the land; how radon seams in France, Czechia, and the Caucasian Mineral Waters raise cancer risks. Leaching for uranium and rare earths leaves soluble salts that slip into aquifers and then into our water and food. If this map protects even one person or animal, it was worth building.

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

The project grows with active help from **Safecast** and the wider community: many great ideas came from **Rob Oudendijk** and friends in open dosimetry worldwide (thank you, Greenpeace and other environmental teams).

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
- On the map page, click the green **Upload** button and drop your tracks (`.kml`, `.kmz`, `.json`, `.rctrk`, `.csv`, `.gpx`, bGeigie Nano/Zen `$BNRDD`, AtomFast, RadiaCode, Safecast, etc.).
- Instant mirror of pelora.org: run `chicha-isotope-map -import-tgz-url https://pelora.org/api/json/weekly.tgz` once — it fetches the weekly archive, fills your database, and quits so the next launch starts fully populated.
- Want the archive saved locally first? Download [https://pelora.org/api/json/weekly.tgz](https://pelora.org/api/json/weekly.tgz), point `-import-tgz-path /path/to/weekly.tgz`, and start with your own copy.

### 🗺️ One-command first run with live data
For a completely fresh install, this single command both preloads real-world tracks and serves the map right away:
```bash
chicha-isotope-map -import-tgz-url https://pelora.org/api/json/weekly.tgz
```
After it imports, rerun normally (or keep the same command in a systemd service) — the map opens with real measurements visible at [http://localhost:8765](http://localhost:8765).

### 🛢️ Database choices (pick what fits you)
- **PostgreSQL (`pgx`)** — fastest and best for multiple users. Example: `chicha-isotope-map -db-type pgx -db-conn postgres://USER:PASS@HOST:PORT/DATABASE?sslmode=allow -import-tgz-url https://pelora.org/api/json/weekly.tgz`
- **DuckDB / SQLite / Chai** — simplest file-based setups for single users. Parallel writes by several users can conflict, so prefer them for personal maps. Example: `chicha-isotope-map -db-type duckdb -import-tgz-url https://pelora.org/api/json/weekly.tgz`

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
- The more nodes exist, the harder it is to miss contamination.

Chicha-Isotope-Map was inspired by **Dmitry Ignatenko** and his forward steps in field research, and is deeply influenced by **Rob Oudendijk** and **Safecast**. Open data from the AtomFast and Radiacode communities keeps it useful. If the map spares even one life, it was not in vain.
