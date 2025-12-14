[![Latest stable release build](https://github.com/matveynator/chicha-isotope-map/actions/workflows/release.yml/badge.svg)](https://github.com/matveynator/chicha-isotope-map/actions/workflows/release.yml)

<img width="30%" align="left" alt="chicha-isotope-map" src="https://github.com/user-attachments/assets/39bfa7b1-03fb-43dd-89bd-8d6c516fd4db" />

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

# ☢️ World Radiation Map
We built this map so anyone can quickly understand whether the place they live or work is safe. Many grow food, keep livestock, or drink from springs without knowing if the environment is healthy.

Natural background radiation stays low. Danger appears only where the levels rise well above that — because of human activity or specific local geology. In such places, water, air, and soil can eventually affect health: harming lungs, stomach, and other organs.

If this map protects even one person or animal, building it was worth it. Let it serve as a simple, clear guide for choosing a safer path.

Live demo: [https://pelora.org/](https://pelora.org/) — your node will look the same.

👉 [Unified download page](https://github.com/matveynator/chicha-isotope-map/releases) (all platforms, latest builds)

👉 [DeepWiki: Chicha Isotope Map](https://deepwiki.com/matveynator/chicha-isotope-map)

---

### 📸 Example view
<p>
  <a href="https://pelora.org" target="_blank"><img width="100%" alt="Fukushima view in chicha-isotope-map" src="https://github.com/user-attachments/assets/617a0ced-4280-41c2-9320-de1cfd33a61f" /></a><br />
  <a href="https://pelora.org" target="_blank"><img width="100%" alt="Safecast realtime radiation sensors in chicha-isotope-map" src="https://github.com/user-attachments/assets/13256b23-744d-4d02-a26c-ae9aef5b0d87" /></a><br />
  <a href="https://pelora.org" target="_blank"><img width="100%" alt="Air flights radiation in chicha-isotope-map" src="https://github.com/user-attachments/assets/cf0189c9-534f-4ff5-9d7a-ed5836e91ef5" /></a>
</p>

---

## 🧭 What’s inside
- The map gathers measurements from many instruments; layers are neatly separated by movement speed — on foot, by car, or in flight.
- You can upload your own tracks: new points immediately appear on the map to clarify the situation.
- Import archives by URL or file, and save your own data as an archive (handy for backups).
- Track how radiation changed over time in a chosen place — getting better or worse.
- Create a short link to any area of the map.
- Use print mode: mark hazardous spots with QR codes so a person can scan and instantly see the radiation level for that exact point. This helps highlight environmental risks where drinking water, long stays, or farming are undesirable. It is useful for ecologists, monitoring specialists, and teams that must warn people about danger.
- The Map offers an API for integrating its data into external services under the open CC license.

The project grows thanks to careful support from the **Safecast** community, the huge work of **Rob Oudendijk**, and countless people worldwide working in open dosimetry. We thank Safecast, AtomFast, Radiacode, DoseMap, and other initiatives for their contribution and involvement.

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

### Option 4. Guided Linux service setup
Prefer a colourful guided install? Launch the wizard and it will ask for port, domain, database path/URI, and support e-mail, then write a port-specific systemd unit and try to enable it automatically:

```bash
./chicha-isotope-map -setup
```

The wizard prints follow-up commands so you immediately know how to start, restart, stop, and tail logs via `systemctl` / `journalctl` or the generated `chicha-isotope-map-<port>.log` file. User sessions install to `~/.config/systemd/user`; running as root targets `/etc/systemd/system`, and each unit is named `chicha-isotope-map-<port>.service` so multiple ports can coexist.

When it asks for a database:
- `sqlite` / `chai` suggest `/var/lib/<db-type>-<port>/database.<ext>` and create directories automatically.
- `pgx` is PostgreSQL with defaults `localhost:5432`, user `postgres`, empty password, and DB name `chicha` — the wizard builds the URI for you.
- `duckdb` only appears if the binary was built with DuckDB enabled.

You can type `restart` at the review step to redo answers with your previous choices prefilled, or `cancel` to pause and rerun later.

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

### 🛢️ Database options for import and regular use
- **PostgreSQL (`pgx`)** — the fastest and most convenient with several users. Example: `chicha-isotope-map -db-type pgx -db-conn postgres://USER:PASS@HOST:PORT/DATABASE?sslmode=allow -import-tgz-url https://pelora.org/api/json/weekly.tgz`
- **DuckDB / SQLite / Chai** — simple file databases for a single user. Concurrent writes can conflict, so keep them for personal maps. Example: `chicha-isotope-map -db-type duckdb -import-tgz-url https://pelora.org/api/json/weekly.tgz`

## 📤 Export
- Single track: `/api/track/{trackID}.json` (legacy `.cim` also works).
- Scheduled archive: `/api/json/weekly.tgz` (or `/daily.tgz`, `/monthly.tgz`, `/yearly.tgz`). Inside: one JSON per track.

---

## 🧠 Advanced options
- Databases: built-in SQLite by default; you can switch to DuckDB, Chai, ClickHouse, or PostgreSQL (`pgx`).
- Import: by URL or file; you can feed an archive directly.
- Export: JSON archives, a single track, legacy `.cim` files supported.
- Appearance: starting coordinates and layer (`-default-*`).
- DuckDB startup slow? See the [performance notes](doc/DUCKDB_PERFORMANCE.md) for checkpoint/Parquet guidance.

---

## 🤝 Why your own node & a bit of history
- We wanted anyone, without training, to see if radiation threatens where they live, grow food, or collect water.
- The more nodes exist, the harder it is to miss contamination.

Chicha-Isotope-Map was inspired by **Dmitry Ignatenko** and his forward steps in field research, and is deeply influenced by **Rob Oudendijk** and **Safecast**. Open data from the AtomFast and Radiacode communities keeps it useful. If the map spares even one life, it was not in vain.
