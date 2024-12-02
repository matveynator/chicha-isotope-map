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

Install on Linux 64-bit x86:  
```bash
sudo curl https://files.zabiyaka.net/chicha-isotope-map/latest/no-gui/linux/amd64/chicha-isotope-map > /usr/local/bin/chicha-isotope-map; sudo chmod +x /usr/local/bin/chicha-isotope-map; chicha-isotope-map -v;
```

Choose your platform to start exploring isotope trails:

| Platform   | Download Link                                                                                           |
|------------|--------------------------------------------------------------------------------------------------------|
| AIX        | [Download for AIX](http://files.zabiyaka.net/chicha-isotope-map/latest/no-gui/aix/)                      |
| Android    | [Download for Android](http://files.zabiyaka.net/chicha-isotope-map/latest/no-gui/android/)               |
| Dragonfly  | [Download for Dragonfly](http://files.zabiyaka.net/chicha-isotope-map/latest/no-gui/dragonfly/)           |
| FreeBSD    | [Download for FreeBSD](http://files.zabiyaka.net/chicha-isotope-map/latest/no-gui/freebsd/)               |
| Illumos    | [Download for Illumos](http://files.zabiyaka.net/chicha-isotope-map/latest/no-gui/illumos/)               |
| JavaScript | [Download for JavaScript](http://files.zabiyaka.net/chicha-isotope-map/latest/no-gui/js/)                 |
| Linux      | [Download for Linux](http://files.zabiyaka.net/chicha-isotope-map/latest/no-gui/linux/)                   |
| macOS      | [Download for macOS](http://files.zabiyaka.net/chicha-isotope-map/latest/no-gui/mac/)                     |
| NetBSD     | [Download for NetBSD](http://files.zabiyaka.net/chicha-isotope-map/latest/no-gui/netbsd/)                 |
| OpenBSD    | [Download for OpenBSD](http://files.zabiyaka.net/chicha-isotope-map/latest/no-gui/openbsd/)               |
| Plan9      | [Download for Plan9](http://files.zabiyaka.net/chicha-isotope-map/latest/no-gui/plan9/)                   |
| Solaris    | [Download for Solaris](http://files.zabiyaka.net/chicha-isotope-map/latest/no-gui/solaris/)               |
| Windows    | [Download for Windows](http://files.zabiyaka.net/chicha-isotope-map/latest/no-gui/windows/)               |

---

## 🛠 **How to Use?**

Run the program with default settings:
```bash
chicha-isotope-map
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
