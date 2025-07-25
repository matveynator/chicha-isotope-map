- [🇫🇷 Français](/doc/README_FR.md)
- [🇯🇵 日本語](/doc/README_JP.md)
- [🇷🇺 Русский](/doc/README_RU.md)

### 🌌 **chicha-isotope-map** — explorer of radiation's hidden paths.

> **"See the unseen." This program visualizes radioactive traces, turning invisible pathways into vibrant maps.**

---

## 📖 **About the Project**

**Chicha-Isotope-Map** reveals the invisible world of radioactive particles. Beneath your feet, isotopes leave traces as they travel, carried by wind, vehicles, or people. This program visualizes them on a map, coloring each trace—from green (safe) to red (danger).

It reads data from formats like `.kml`, `.kmz`, `.json`, and `.rctrk` (AtomFast and RadiaCode) and stores it in a database. Years later, you can look back and see how radiation levels changed over time.

---

### 🌍 **Inspired by Nature**

The program uses **natural background radiation** as a baseline. In untouched areas, normal radiation is around **1–4 µR/h**. Anything above this is flagged as **radioactive contamination**. Chicha-Isotope-Map tracks these anomalies, turning invisible footprints into visible warnings.

---

### 📸 **Live Demo**

<a href="https://jutsa.ru" target="_blank">See the program in action here.</a>

---

### 📸 **Visual Example**

In the Soviet era, an open-air swimming pool was built in Kislovodsk Park. The concrete may have come from a factory in Pyatigorsk that processed radioactive ore from Mount Beshtau. Trucks carried materials, leaving invisible dust on the roads. Decades later, these traces still show up on the map as yellow marks—like patches of autumn leaves. The rest of the park remains clean, peaceful, and green.
<img src="https://repository-images.githubusercontent.com/870016860/11fd6abc-fe8b-4cd8-95c2-df1c631c8762">

---

## 📥 **Download and Get Started** 📥

### Linux 64-bit amd64: 
Note: Install as ROOT user. 
```bash
curl -L https://github.com/matveynator/chicha-isotope-map/releases/download/latest/chicha-isotope-map_linux_amd64 > /usr/local/bin/chicha-isotope-map; chmod +x /usr/local/bin/chicha-isotope-map; chicha-isotope-map --version;
```

### Mac OS X Intel:
Note: Install as ROOT user.
```bash
curl -L https://github.com/matveynator/chicha-isotope-map/releases/download/latest/chicha-isotope-map_darwin_amd64 > /usr/local/bin/chicha-isotope-map; chmod +x /usr/local/bin/chicha-isotope-map; chicha-isotope-map --version;
```

### Mac OS X Apple Silicon:
Note: Install as ROOT user.
```bash
curl -L https://github.com/matveynator/chicha-isotope-map/releases/download/latest/chicha-isotope-map_darwin_amd64 > /usr/local/bin/chicha-isotope-map; chmod +x /usr/local/bin/chicha-isotope-map; chicha-isotope-map --version;
```
                
[Download for all other platforms: Linux, macOS, Windows, FreeBSD, OpenBSD, NetBSD](https://github.com/matveynator/chicha-isotope-map/releases/tag/latest)             

---

## 🛠 **How to Use?**

Run the program with default settings:
```bash
chicha-isotope-map
```

#### Chernobyl (1986) — steam explosion & graphite fire; massive fallout across Europe
```
./chicha-isotope-map -default-lat=51.389 -default-lon=30.099 -default-zoom=11 -default-layer="Google Satellite"
```

#### Fukushima Daiichi (2011) — tsunami disabled cooling; core melts & releases to sea/air
```
./chicha-isotope-map -default-lat=37.421 -default-lon=141.033 -default-zoom=12 -default-layer="Google Satellite"
```

#### Kyshtym / Mayak (1957) — waste‑tank explosion; radioactive plume over the Urals
```
./chicha-isotope-map -default-lat=55.700 -default-lon=60.800 -default-zoom=9 -default-layer="Google Satellite"
```

#### Three Mile Island (1979) — partial core meltdown; limited off‑site release
```
./chicha-isotope-map -default-lat=40.153 -default-lon=-76.723 -default-zoom=12 -default-layer="Google Satellite"
```

#### Windscale (1957) — graphite‑reactor fire; iodine‑131 release over UK
```
./chicha-isotope-map -default-lat=54.432 -default-lon=-3.553 -default-zoom=12 -default-layer="Google Satellite"
```

#### Goiânia (1987) — orphaned Cs‑137 source opened; city‑wide contamination
```
./chicha-isotope-map -default-lat=-16.686 -default-lon=-49.264 -default-zoom=13 -default-layer="Google Satellite"
```

#### Pyatigorsk, Mount Beshtau (1940s‑50s) — first USSR atomic bomb uranium mines; district‑wide contamination
```
./chicha-isotope-map -default-lat=44.089 -default-lon=42.976 -default-zoom=11 -default-layer="Google Satellite"
```

Or customize with additional options:
```bash
chicha-isotope-map --port=8765 --db-type=pgx --db-host=localhost --db-port=5432 --db-user=postgres --db-pass=yourpassword --db-name=isotope_db --pg-ssl-mode=prefer
```

### PostgreSQL Example (`pgx` driver):
```bash
chicha-isotope-map --port=8765 --db-type=pgx --db-host=localhost --db-port=5432 --db-user=postgres --db-pass=my_secure_password --db-name=radiation_data --pg-ssl-mode=require
```

This configuration connects to a PostgreSQL database named `radiation_data` on the local machine. Replace `my_secure_password` with your database password. Adjust the host, port, or database name as needed.

---

### Web Interface:

1. Open [http://localhost:8765](http://localhost:8765) in your web browser.
2. Use the **Upload** button to add your data files.
3. Explore the map: hover over markers to view radiation levels, timestamps, and locations.

---

## ☢️ **Why It Matters**

Radiation is invisible but dangerous. It doesn’t just stay in one place—it seeps into soil, water, and plants, accumulating over time. This program helps you see where contamination has spread, making the invisible visible and empowering you to understand and act.

---

> **"If isotopes could tell their stories, they wouldn’t need this program. But since they can’t, Chicha-Isotope-Map speaks for them."**

> *Chicha Isotope Map* was created for **Dmitry Ignatenko’s Radiation Research Laboratory** and is deeply inspired by Japan’s [**Safecast**](https://map.safecast.org) community of citizen‑scientists who turned crisis into knowledge. By searching, measuring, and sharing the truth about radiation, you make the invisible visible and help ensure that tragedies like **Chernobyl** and **Fukushima** remain in the past. Your work lights a path of science, safety, and hope.  Thank you for making the invisible visible, where background radiation is not fear but a source of knowledge — and for seeking, measuring, sharing, and bravely going first.  

