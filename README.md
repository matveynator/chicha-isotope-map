- [🇫🇷 Français](/doc/README_FR.md)
- [🇯🇵 日本語](/doc/README_JP.md)
- [🇷🇺 Русский](/doc/README_RU.md)

## 🚀 Installation & your own node in 5 minutes!

### 1. Quick start with Docker

**Why Docker?**
Docker bundles the programme and its environment into a “container”. No faffing about with databases and dependencies — just run the ready-made image.

#### Local run (port 5000)

```bash
docker run -d \
  --name chicha-isotope-map \
  -e PORT=5000 \
  -p 5000:5000 \
  -v isotope-data:/var/lib/postgresql/data \
  matveynator/chicha-isotope-map:latest
```

Open [http://localhost:5000](http://localhost:5000) in your browser and you’ll see the map.

#### On your own domain with HTTPS

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

#### Extra map settings

See all available options by calling `--help` on the programme.
Optionally set a starting point and style:

```text
  -e DEFAULT_LAT=51.389      # latitude
  -e DEFAULT_LON=30.099      # longitude
  -e DEFAULT_ZOOM=11         # zoom level
  -e DEFAULT_LAYER="OpenStreetMap" or "Google Satellite"
```

#### Daily backups (once a day)

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

If you’re not fond of containers, grab the ready-made binary and run it — even quicker!

> Run commands as **root** (`sudo -i` or `sudo ...`).

* **Linux x64**:

```
curl -L https://github.com/matveynator/chicha-isotope-map/releases/download/latest/chicha-isotope-map_linux_amd64 > /usr/local/bin/chicha-isotope-map && chmod +x /usr/local/bin/chicha-isotope-map && chicha-isotope-map
```

* **macOS Intel**:

```
curl -L https://github.com/matveynator/chicha-isotope-map/releases/download/latest/chicha-isotope-map_darwin_amd64 > /usr/local/bin/chicha-isotope-map && chmod +x /usr/local/bin/chicha-isotope-map && chicha-isotope-map
```

* **macOS Apple Silicon**:

```
curl -L https://github.com/matveynator/chicha-isotope-map/releases/download/latest/chicha-isotope-map_darwin_arm64 > /usr/local/bin/chicha-isotope-map && chmod +x /usr/local/bin/chicha-isotope-map && chicha-isotope-map
```

Other platforms — Windows / ARM / BSD — can be downloaded from the releases page: [https://github.com/matveynator/chicha-isotope-map/releases/tag/latest](https://github.com/matveynator/chicha-isotope-map/releases/tag/latest)

By default the programme listens on port 8765. Open [http://localhost:8765](http://localhost:8765).

---

## 🤝 Why run your own node?

* **Independence:** your data stays with you; you’re not reliant on someone else’s network.
* **Network resilience:** the more nodes there are, the harder it is to compromise.
* **Local background history:** preserve your region’s radiation map for years to come.

Every one of your servers is another beacon of information. Thank you for making the world that bit clearer!

--- 

# 🌌 Chicha‑Isotope‑Map — your guide to radiation’s hidden trails

Chicha‑Isotope‑Map isn’t just a bit of software; it’s a window onto a world of microscopic particles — invisible to the eye, yet loud and clear to an instrument. Once you could only guess at them; now they’re splashed across the map as bright dots: from calm greens to alarming reds.

* **What does it read, and from where?**

  * Files in `.kml`, `.kmz`, `.json`, `.rctrk` (AtomFast, RadiaCode) formats.
  * Everything is stored in its own database, so years later you can say with certainty: “On 12 March 2024 it was 4.1 µR/h here.”

* **What’s our point of reference?**

  * The natural background: in a “clean” spot it’s roughly 0.8–4 µR/h.
  * Anything above that is alien contamination. You’ll see how isotopes were scattered by wind, cars and people — like footprints in freshly fallen snow.

---

### 📸 **Screenshots**

... Back in Soviet times they were building an open-air swimming pool in Kislovodsk Park. Perhaps they used concrete from a plant in Pyatigorsk, where radioactive ore from Mount Beshtau had once been processed. Lorries trundled along the road, dust from their tyres settled on the tarmac, leaving invisible marks. Years have passed, yet those traces still glow, like memories of the past. Dust blown around the building site settled in the park — on the map it shows up yellow, like splashes of autumn leaves. Everything else in the park is clean, calm, green. <img src="https://repository-images.githubusercontent.com/870016860/11fd6abc-fe8b-4cd8-95c2-df1c631c8762">

---

### 📸 **Demo**

<a href="https://jutsa.ru" target="_blank"><img width="1156" height="844" alt="Chicha Isotope Map" src="https://github.com/user-attachments/assets/8d806377-671f-47a0-b918-f2a9afd4123e" /></a>

<a href="https://jutsa.ru" target="_blank">Here you can see the programme running in real time.</a>

---

The Chicha Isotope Map was created for Dmitry Ignatenko’s Radiation Research Laboratory and inspired by Japan’s Safecast community — citizen scientists who turned tragedy into knowledge. By seeking, measuring and sharing the truth about radiation, you make the invisible visible, helping the world avoid another Chernobyl or Fukushima. Your work is the light of science, safety and hope. Thank you for turning background radiation from a cause for fear into a source of understanding — for searching, measuring, sharing, and stepping forward with courage.



