[![Latest stable release build](https://github.com/matveynator/chicha-isotope-map/actions/workflows/release.yml/badge.svg)](https://github.com/matveynator/chicha-isotope-map/actions/workflows/release.yml)

<img width="30%" align="left" alt="chicha-isotope-map" src="https://github.com/user-attachments/assets/39bfa7b1-03fb-43dd-89bd-8d6c516fd4db" />

- [🇬🇧 English](/README.md)
- [🇫🇷 Français](/doc/README_FR.md)
- [🇯🇵 日本語](/doc/README_JP.md)
- [🇷🇺 Русский](/doc/README_RU.md)

# ☢️ World Radiation Map
Live demo: [https://pelora.org/](https://pelora.org/) — your own node looks the same.

👉 [DeepWiki: Chicha Isotope Map](https://deepwiki.com/matveynator/chicha-isotope-map)

---

### 📸 Example view
<a href="https://pelora.org" target="_blank"><img width="800" alt="pelora.org chicha-isotope-map example" src="https://github.com/user-attachments/assets/be706959-a2d5-4949-9378-811f4022aa98" /></a>

---

## 🚀 Run with Docker (fastest)
Both commands ship with the built-in PostgreSQL database. Copy, paste, done.

#### 🔥 Local (port 8765)
```bash
docker run -d \
  --name chicha-isotope-map \
  -p 8765:8765 \
  -v chicha-data:/var/lib/postgresql/data \
  -e DEFAULT_LAT=44.08832 \
  -e DEFAULT_LON=42.97577 \
  -e DEFAULT_ZOOM=11 \
  -e DEFAULT_LAYER="OpenStreetMap" \
  --restart unless-stopped \
  matveynator/chicha-isotope-map:latest
```
Open: [http://localhost:8765](http://localhost:8765)

#### 🔥 Public node with HTTPS
```bash
docker run -d \
  --name chicha-isotope-map \
  -p 80:80 -p 443:443 \
  -v chicha-data:/var/lib/postgresql/data \
  -e DOMAIN=example.org \
  -e DEFAULT_LAT=44.08832 \
  -e DEFAULT_LON=42.97577 \
  -e DEFAULT_ZOOM=11 \
  -e DEFAULT_LAYER="OpenStreetMap" \
  --restart unless-stopped \
  matveynator/chicha-isotope-map:latest
```
After Let’s Encrypt finishes: [https://example.org](https://example.org)

**Env vars:**
`DOMAIN` for HTTPS, `DEFAULT_LAT` / `DEFAULT_LON` / `DEFAULT_ZOOM` / `DEFAULT_LAYER` for the opening map view, `PORT` for the internal listener. Keep data on `-v chicha-data:/var/lib/postgresql/data` so container upgrades do not wipe history.

---

## ⬇️ Download a binary (no Docker)
Grab the build for your OS, make it executable, run it.

**Linux x64**
```bash
sudo curl -L https://github.com/matveynator/chicha-isotope-map/releases/download/latest/chicha-isotope-map_linux_amd64 \
  -o /usr/local/bin/chicha-isotope-map \
  && sudo chmod +x /usr/local/bin/chicha-isotope-map \
  && chicha-isotope-map
```

**macOS Intel (x86_64)**
```bash
sudo curl -L https://github.com/matveynator/chicha-isotope-map/releases/download/latest/chicha-isotope-map_darwin_amd64 \
  -o /usr/local/bin/chicha-isotope-map \
  && sudo chmod +x /usr/local/bin/chicha-isotope-map \
  && chicha-isotope-map
```

**macOS Apple Silicon (arm64)**
```bash
sudo curl -L https://github.com/matveynator/chicha-isotope-map/releases/download/latest/chicha-isotope-map_darwin_arm64 \
  -o /usr/local/bin/chicha-isotope-map \
  && sudo chmod +x /usr/local/bin/chicha-isotope-map \
  && chicha-isotope-map
```

Other platforms (Windows / ARM / BSD): [latest release](https://github.com/matveynator/chicha-isotope-map/releases/tag/latest).

---

## 🖥 Running the binary
Minimal flags for a clean start:
- `-domain maps.example.org` — enable HTTPS on 80/443 (Let’s Encrypt).
- `-port 8765` — HTTP port when running locally.
- `-default-lat` / `-default-lon` / `-default-zoom` / `-default-layer` — initial map view.
- Storage: `-db-type sqlite|duckdb|pgx|chai|clickhouse`, `-db-path` for file databases, or `-db-conn` for network drivers.
- Utility: `-version` prints version.

DuckDB builds require `CGO_ENABLED=1 go build -tags duckdb` and then `./chicha-isotope-map -db-type duckdb`.

---

## 📥 Import data
- Supported uploads: `.kml`, `.kmz`, `.json`, `.rctrk`, `.csv`, `.gpx`, bGeigie Nano/Zen `$BNRDD` logs (`.log` / `.txt`), AtomFast, RadiaCode, Safecast and similar exports.
- Web UI: open your node → click **Upload** → pick files → the newest imported track opens automatically.
- API: `curl -F 'files[]=@/path/to/file.log' http://localhost:8765/upload` (diagnostics: `/upload_diag`).
- Latest markers near a point: `/api/latest?lat=...&lon=...&radius_m=1500&limit=20` streams fresh readings.

---

## 📤 Export data
- **Per track:** download `/api/track/{trackID}.json` (legacy `.cim` works). Optional `from`/`to` narrow marker IDs. Browsers save it as `{trackID}.json`.
- **Bulk archive:** fetch `/api/json/weekly.tgz` (or `/daily.tgz`, `/monthly.tgz`, `/yearly.tgz` if configured). The bundle contains one JSON file per track.
- **JSON schema:**
  - Top-level: `trackID`, `trackIndex` (1-based position), `apiURL`, `firstID`, `lastID`, `markerCount`, `disclaimers`, `markers`.
  - Each marker: `id`, `timeUnix`, `timeUTC` (RFC3339), `lat`, `lon`, optional `altitudeM`, `temperatureC`, `humidityPercent`, derived speeds (`speedMS`, `speedKMH`), dose rates (`doseRateMicroSvH`, `doseRateMicroRh`), `countRateCPS`, and optional detector info (`detectorType`, `detectorName`, `radiationTypes`).
  - We keep `disclaimers` in multiple languages inside every export.
- **Coming next:** the same JSON will likely embed per-point spectrometric data once we start recording it, so archives remain forward-compatible.

---

## 💾 Backup & restore
- **Daily backup (03:00):** `0 3 * * * docker exec chicha-isotope-map pg_dump -U chicha_isotope_map chicha_isotope_map | gzip > /backup/chicha_isotope_map_$(date +\%F).sql.gz`
- **Restore:**
  ```bash
  docker exec -it chicha-isotope-map psql -U postgres -c "DROP DATABASE IF EXISTS chicha_isotope_map; CREATE DATABASE chicha_isotope_map OWNER chicha_isotope_map;"
  zcat /backup/chicha_isotope_map_2025-07-24.sql.gz | docker exec -i chicha-isotope-map psql -U chicha_isotope_map chicha_isotope_map
  ```

---

## 🤝 Why host your own node?
- Your community, your measurements, your history.
- Local baseline of natural background (roughly 0.8–4 µR/h) stays with you.
- More nodes → more transparency and resilience for everyone.

Chicha‑Isotope‑Map was built for the **Dmitry Ignatenko Radiation Research Lab** and inspired by **Safecast**. Thank you to the AtomFast and Radiacode communities for openly sharing their data.
