[![Latest stable release build](https://github.com/matveynator/chicha-isotope-map/actions/workflows/release.yml/badge.svg)](https://github.com/matveynator/chicha-isotope-map/actions/workflows/release.yml)

- [🇬🇧 English](/README.md)
- [🇫🇷 Français](/doc/README_FR.md)
- [🇯🇵 日本語](/doc/README_JP.md)
- [🇷🇺 Русский](/doc/README_RU.md)

#  chicha‑isotope‑map is a world ☢️ radiation map.

---

### 📸 **Live demo**

<a href="https://pelora.org" target="_blank"><img width="800"  alt="pelora.org chicha-isotope-map example demo" src="https://github.com/user-attachments/assets/be706959-a2d5-4949-9378-811f4022aa98" /></a>


---

The **DeepWiki** page for *Chicha Isotope Map* was kindly created by [Rob Ouden](https://github.com/robouden) from the **Safecast** project, to whom we are deeply grateful.  This page opens the inner structure of our program, so that developers may understand its foundations, follow its logic, and continue the work by improving and extending it.  Thanks to DeepWiki, the code is not just a tool, but a living project that can grow and evolve with the help of many hands.  

👉 [DeepWiki: Chicha Isotope Map](https://deepwiki.com/matveynator/chicha-isotope-map)

---


## 🚀 Install & run your own node in 2 commands

No fluff. The image ships with the database (PostgreSQL) built in. Copy the command, run it — you’re done.

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

#### 🔥 Public node with HTTPS on your own domain

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

Once the certificate is issued, go to: [https://example.org](https://example.org)

---

### ⚙️ Configure via environment variables (just what you need)

* `DOMAIN` — enables HTTPS on ports 80/443 with automatic Let’s Encrypt certificates (for a public node).
* `DEFAULT_LAT`, `DEFAULT_LON` — initial map coordinates.
* `DEFAULT_ZOOM` — initial zoom (11 is a convenient city level).
* `DEFAULT_LAYER` — `OpenStreetMap` or `Google Satellite`.
* `PORT` — app’s internal port (defaults to 8765; you usually don’t change this).

> Tip: keep your data on the volume `-v chicha-data:/var/lib/postgresql/data` so it survives container updates.

---

### 💾 Backup & restore (simple)

**Daily backup (03:00):**

```bash
0 3 * * * docker exec chicha-isotope-map pg_dump -U chicha_isotope_map chicha_isotope_map | gzip > /backup/chicha_isotope_map_$(date +\%F).sql.gz
```

**Restore from archive:**

```bash
docker exec -it chicha-isotope-map psql -U postgres -c "DROP DATABASE IF EXISTS chicha_isotope_map; CREATE DATABASE chicha_isotope_map OWNER chicha_isotope_map;"

zcat /backup/chicha_isotope_map_2025-07-24.sql.gz | docker exec -i chicha-isotope-map psql -U chicha_isotope_map chicha_isotope_map
```

---

## ⬇️ Download prebuilt apps (no Docker)

Grab the binary for your OS, make it executable, and run.

**Linux x64**

```bash
sudo curl -L https://github.com/matveynator/chicha-isotope-map/releases/download/latest/chicha-isotope-map_linux_amd64 \
  -o /usr/local/bin/chicha-isotope-map \
  && sudo chmod +x /usr/local/bin/chicha-isotope-map \
  && chicha-isotope-map
```

**macOS Intel (x86\_64)**

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

Other platforms (Windows / ARM / BSD) — see the releases page:
[https://github.com/matveynator/chicha-isotope-map/releases/tag/latest](https://github.com/matveynator/chicha-isotope-map/releases/tag/latest)

---

## 🖥 Running the binary (no Docker): flags & examples

If you’re launching the `chicha-isotope-map` binary directly, here’s what matters. First the essentials; more options below.

### Essentials

* `-domain string` — enables HTTPS and binds to ports 80 and 443 with automatic Let’s Encrypt certificates. Your domain must point to your server, and ports 80/443 must be open.

  * Example: `sudo chicha-isotope-map -domain maps.example.org -default-lat 44.08832 -default-lon 42.97577 -default-zoom 11 -default-layer "OpenStreetMap"`

* `-port int` — HTTP server port (defaults to 8765). Handy for local runs without a domain.

  * Example: `chicha-isotope-map -port 8765`

* `-default-lat float` & `-default-lon float` — initial map latitude and longitude.

  * Example: `-default-lat 44.08832 -default-lon 42.97577`

* `-default-zoom int` — initial zoom level (city is usually 11–13).

  * Example: `-default-zoom 11`

* `-default-layer string` — base layer: `OpenStreetMap` or `Google Satellite`.

  * Example: `-default-layer "Google Satellite"`

### Storage (if you need it)

* `-db-type string` — DB driver: `duckdb`, `genji`, `sqlite`, `pgx` (PostgreSQL). Default is `sqlite`.

* `-db-path string` — DB file path for `duckdb`/`genji`/`sqlite` (defaults to current directory if not set).

  * Example: `-db-type sqlite -db-path /var/lib/chicha/chicha.sqlite`

* `-db-host string`, `-db-port int` (default 5432), `-db-name string`, `-db-user string`, `-db-pass string` — PostgreSQL connection params for `pgx`.

  * Example: `-db-type pgx -db-host 127.0.0.1 -db-port 5432 -db-name chicha_isotope_map -db-user postgres -db-pass secret`

* `-pg-ssl-mode string` — PostgreSQL SSL mode: `disable`, `allow`, `prefer` (default), `require`, `verify-ca`, `verify-full`.

  * Example: `-pg-ssl-mode require`

### Utility

* `-version` — print version and exit.

### Quick examples

* **Local, no HTTPS:**

  ```bash
  chicha-isotope-map \
    -port 8765 \
    -default-lat 44.08832 -default-lon 42.97577 \
    -default-zoom 11 \
    -default-layer "OpenStreetMap"
  ```

* **Public server with HTTPS on 80/443:**

  ```bash
  sudo chicha-isotope-map \
    -domain maps.example.org \
    -default-lat 44.08832 -default-lon 42.97577 \
    -default-zoom 11 \
    -default-layer "OpenStreetMap"
  ```

* **Single‑file storage (SQLite):**

  ```bash
  chicha-isotope-map \
    -db-type sqlite -db-path /var/lib/chicha-isotope-map.sqlite \
    -port 8765 \
    -default-lat 44.08832 -default-lon 42.97577 -default-zoom 11
  ```

* **Connect to PostgreSQL:**

  ```bash
  chicha-isotope-map \
    -db-type pgx \
    -db-host 127.0.0.1 -db-port 5432 \
    -db-name chicha_isotope_map -db-user postgres -db-pass secret \
    -pg-ssl-mode require \
    -default-lat 44.08832 -default-lon 42.97577 -default-zoom 11
  ```

---

## DuckDB  

DuckDB is an embedded database: one file, no server, written in C++. The Go driver uses **CGO** and DuckDB **shared libraries**, so we have to build it every time with:

```bash
CGO_ENABLED=1 go build -tags duckdb
```

Run with:

```bash
./chicha-isotope-map -db-type duckdb -db-path /data.duckdb
```

### DuckDB Platforms

| Platform            | How to use                                            |
| ------------------- | ----------------------------------------------------- |
| Linux (amd64)       | Build from source with `CGO_ENABLED=1` and `-tags duckdb`                |
| macOS (amd64/arm64) | Either build from source or download prebuilt stable release |

---

## 🤝 Why run your own node?

* **Simple:** your community, your map.
* **Useful:** a local history of background levels for your city/area/facility — and it’s yours to keep.
* **Good for the network:** more nodes → more transparency and resilience.

---

Chicha‑Isotope‑Map isn’t just software — it’s a window into a world of microparticles, invisible to the eye but obvious to an instrument. What used to be guesswork is now drawn as bright dots on a map, from calm greens to alarming reds.

* **What does it read, and from where?**

  * Files in `.kml`, `.kmz`, `.json`, `.rctrk`, `.csv`, `.gpx` formats (AtomFast, RadiaCode, Safecast and others).
  * Everything is saved to its own database so that years later you can say, “On March 12, 2008, this spot measured 4.1 µR/h.”

* **What’s our baseline?**

  * Natural background radiation in a “clean” area is about 0.8–4 µR/h.
  * Anything above that is anthropogenic contamination. You’ll see how isotopes were scattered by wind, traffic, and people — like footprints on fresh snow.

---

### 📸 **Screenshots**

... Back in the Soviet era, an open‑air pool was being built in Kislovodsk Park. They may have used concrete from a plant in Pyatigorsk that once processed radioactive ore from Mount Beshtau. Trucks drove along the road; dust from their wheels settled on the asphalt, leaving invisible marks. Years have passed, yet those traces still glow — memories of what once was. Dust that blew around the construction site settled in the park; on the map it shows up yellow, like patches of autumn leaves. The rest of the park is clean, calm, and green. <img src="https://repository-images.githubusercontent.com/870016860/11fd6abc-fe8b-4cd8-95c2-df1c631c8762">

---

### 📣 How to announce your node to the community

1. Bring up a node on a domain with HTTPS (`-e DOMAIN=...` + expose `80` and `443`).
2. Add a screenshot and a brief blurb (city, area, what your measurements cover).
3. Leave a note in the project repository’s Issues.

---

The *Chicha Isotope Map* was created for the **Dmitry Ignatenko Radiation Research Lab**, and finds its inspiration in **Safecast** — the remarkable Japanese community of citizen-scientists who transformed the tragedy of Fukushima into a gift of scientific knowledge.

By searching, measuring, and openly sharing the truth about radiation, you make the invisible visible. In doing so, you help the world avoid repeating the painful lessons of Chernobyl and Fukushima.

Your work is a light — of science, of safety, and of hope. Thank you for turning background radiation from something feared into something understood.

Thank you for your courage: for seeking, measuring, sharing, and — above all — for being the first to step forward.

We also extend our heartfelt gratitude to the **AtomFast** and **Radiacode** communities for their invaluable contributions — for measuring, and for generously sharing their data with the world.







