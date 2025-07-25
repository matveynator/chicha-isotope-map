- [🇫🇷 Français](/doc/README_FR.md)
- [🇯🇵 日本語](/doc/README_JP.md)
- [🇷🇺 Русский](/doc/README_RU.md)


# 🌌 Chicha‑Isotope‑Map — a guide to the hidden paths of radiation

Chicha‑Isotope‑Map isn’t just software; it’s a window into the world of micro‑particles, invisible to the eye yet obvious to an instrument. Before, you could only guess at them; now they’re drawn as bright dots on a map, from calm green to alarming red.

* **What does it read, and from where?**

  * From `.kml`, `.kmz`, `.json`, `.rctrk` files (AtomFast, RadiaCode).
  * It saves everything in its own database so that years from now you can say: “On 12 March 2024 it was 4.1 µR/h here.”

* **What’s our starting point?**

  * Natural background radiation: in a “clean” area it’s roughly 0.8–4 µR/h.
  * Anything higher is foreign contamination. You’ll see how isotopes were scattered by wind, cars and people, like footprints on freshly fallen snow.

---

### 📸 **Screenshots**

... Back in the Soviet era they were building an open-air swimming pool in Kislovodsk Park. Perhaps they used concrete from a plant in Pyatigorsk, where radioactive ore from Mount Beshtau had once been processed. Lorries drove along the road and dust from their wheels settled on the asphalt, leaving invisible marks. Years have passed, yet these traces still glow, like memories of the past. The dust that blew around the construction site settled in the park — on the map it’s yellow, like patches of autumn leaves. Everything else in the park is clean, calm, green. <img src="https://repository-images.githubusercontent.com/870016860/11fd6abc-fe8b-4cd8-95c2-df1c631c8762">

---

### 📸 **Demo**

<a href="https://jutsa.ru" target="_blank">You can see the programme working in real time here.</a>

---

## 🚀 Installation and your own node in 5 minutes!

### 1. Quick start with Docker

**Why Docker?**
Docker packs the programme and its environment into a “container”. You don’t need to configure databases and dependencies by hand — just run the ready-made image.

#### Local run (port 5000)

```bash
docker run -d \
  --name chicha-isotope-map \
  -e PORT=5000 \
  -p 5000:5000 \
  -v isotope-data:/var/lib/postgresql/data \
  matveynator/chicha-isotope-map:latest
```

Open [http://localhost:5000](http://localhost:5000) in your browser to see the map.

#### Under your own domain with HTTPS

1. Make sure `domain.com` points to your server’s IP.
2. Ports 80 and 443 are free.
3. Run the command as **root**:

```bash
docker run -d \
  --name chicha-isotope-map \
  -e DOMAIN=domain.com \
  -p 80:80 -p 443:443 \
  -v isotope-data:/var/lib/postgresql/data \
  matveynator/chicha-isotope-map:latest
```

The programme will automatically obtain and renew SSL certificates.

#### Additional map settings (check --help for available options)

Optionally set a starting point and style:

```text
  -e DEFAULT_LAT=51.389      # latitude
  -e DEFAULT_LON=30.099      # longitude
  -e DEFAULT_ZOOM=11         # zoom level
  -e DEFAULT_LAYER="OpenStreetMap" or "Google Satellite"
```

#### Regular backups (once a day)

Add to `crontab -e`:

```bash
0 3 * * * docker exec chicha-isotope-map pg_dump -U chicha_isotope_map chicha_isotope_map | gzip > /backup/chicha_isotope_map_$(date +\%F).sql.gz
```

#### Restoring from an archive

```bash
docker exec -it chicha-isotope-map psql -U postgres -c "DROP DATABASE IF EXISTS chicha_isotope_map; CREATE DATABASE chicha_isotope_map OWNER chicha_isotope_map;"

zcat /backup/chicha_isotope_map_2025-07-24.sql.gz | docker exec -i chicha-isotope-map psql -U chicha_isotope_map chicha_isotope_map
```

---

### 2. Installing without Docker

If you’re not keen on containers, download a ready-made binary and launch it in seconds — even easier!

> Run the commands as **root** (`sudo -i` or `sudo ...`).

* **Linux x64**:

  ```bash
  curl -L https://github.com/matveynator/chicha-isotope-map/releases/download/latest/chicha-isotope-map_linux_amd64 \
    > /usr/local/bin/chicha-isotope-map \
    && chmod +x /usr/local/bin/chicha-isotope-map \
    && chicha-isotope-map
  ```

* **macOS Intel**:

  ```bash
  curl -L https://github.com/matveynator/chicha-isotope-map/releases/download/latest/chicha-isotope-map_darwin_amd64 \
    > /usr/local/bin/chicha-isotope-map \
    && chmod +x /usr/local/bin/chicha-isotope-map \
    && chicha-isotope-map
  ```

* **macOS Apple Silicon**:

  ```bash
  curl -L https://github.com/matveynator/chicha-isotope-map/releases/download/latest/chicha-isotope-map_darwin_arm64 \
    > /usr/local/bin/chicha-isotope-map \
    && chmod +x /usr/local/bin/chicha-isotope-map \
    && chicha-isotope-map
  ```

By default the programme listens on port 8765. Open [http://localhost:8765](http://localhost:8765).

---

## 🤝 Why run your own node?

* **Independence:** your data stays with you and doesn’t depend on someone else’s network.
* **Network resilience:** the more nodes, the harder it is to compromise.
* **Local background history:** preserve your region’s radiation map for years to come.

Every server you add is another beacon of information. Thank you for making the world more transparent!

The Chicha Isotope Map was created for Dmitry Ignatenko’s Radiation Research Laboratory and inspired by the Japanese Safecast community — a group of citizen scientists who turned tragedy into knowledge. By seeking, measuring and sharing the truth about radiation, you make the invisible visible, helping the world avoid another Chernobyl or Fukushima. Your work is the light of science, safety and hope. Thank you for turning background radiation from a cause for fear into a source of understanding, for searching, measuring, sharing — and for having the courage to go first.


