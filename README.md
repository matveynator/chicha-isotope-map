- [🇫🇷 Français](/doc/README_FR.md)
- [🇯🇵 日本語](/doc/README_JP.md)
- [🇷🇺 Русский](/doc/README_RU.md)

### 🌌 **chicha-isotope-map: Explore the Invisible**

**"See the unseen."** This program visualizes radioactive traces, turning invisible pathways into vibrant maps.

---

### 📖 **What Is It?**

**Chicha-Isotope-Map** uncovers the hidden dance of radioactive isotopes. Using data from formats like `.kml`, `.kmz`, `.json`, and `.rctrk`, it creates maps where radiation levels come alive in colors—green for safe, red for caution. It's built on the natural radiation baseline (3-4 µR/h) to identify contamination.

---

### 📥 **Download and Get Started** 📥

Choose the version for your platform and start tracking isotope trails:

| Platform   | Download Link                                                                                           |
|------------|--------------------------------------------------------------------------------------------------------|
| AIX        | [Download for AIX](http://files.zabiyaka.net/chicha-isotope-map/latest/no-gui/aix/)                      |
|                                                                                                                        |
| Android    | [Download for Android](http://files.zabiyaka.net/chicha-isotope-map/latest/no-gui/android/)               |
|                                                                                                                        |
| Dragonfly  | [Download for Dragonfly](http://files.zabiyaka.net/chicha-isotope-map/latest/no-gui/dragonfly/)           |
|                                                                                                                        |
| FreeBSD    | [Download for FreeBSD](http://files.zabiyaka.net/chicha-isotope-map/latest/no-gui/freebsd/)               |
|                                                                                                                        |
| Illumos    | [Download for Illumos](http://files.zabiyaka.net/chicha-isotope-map/latest/no-gui/illumos/)               |
|                                                                                                                        |
| JavaScript | [Download for JavaScript](http://files.zabiyaka.net/chicha-isotope-map/latest/no-gui/js/)                 |
|                                                                                                                        |
| Linux      | [Download for Linux](http://files.zabiyaka.net/chicha-isotope-map/latest/no-gui/linux/)                   |
|                                                                                                                        |
| macOS      | [Download for macOS](http://files.zabiyaka.net/chicha-isotope-map/latest/no-gui/mac/)                     |
|                                                                                                                        |
| NetBSD     | [Download for NetBSD](http://files.zabiyaka.net/chicha-isotope-map/latest/no-gui/netbsd/)                 |
|                                                                                                                        |
| OpenBSD    | [Download for OpenBSD](http://files.zabiyaka.net/chicha-isotope-map/latest/no-gui/openbsd/)               |
|                                                                                                                        |
| Plan9      | [Download for Plan9](http://files.zabiyaka.net/chicha-isotope-map/latest/no-gui/plan9/)                   |
|                                                                                                                        |
| Solaris    | [Download for Solaris](http://files.zabiyaka.net/chicha-isotope-map/latest/no-gui/solaris/)               |
|                                                                                                                        |
| Windows    | [Download for Windows](http://files.zabiyaka.net/chicha-isotope-map/latest/no-gui/windows/)               |
|                                                                                                                        |

Install on Linux 64bit x68:  
```bash
sudo curl https://files.zabiyaka.net/chicha-isotope-map/latest/no-gui/linux/amd64/chicha-isotope-map > /usr/local/bin/chicha-isotope-map; sudo chmod +x /usr/local/bin/chicha-isotope-map; chicha-isotope-map -v;
```

---

### 🛠 **How to Use?**

Run with default settings:
```bash
chicha-isotope-map
```

Customize with options for Postgres:
```bash
chicha-isotope-map --port=8765 --db-type=pgx --db-user postgres --dp-password SECRET --db-host=localhost --db-name=isotope_db
```

Access via browser at [http://localhost:8765](http://localhost:8765).

---

### ☢️ **Why It Matters**

Radiation is silent but impactful. This tool lets you see its footprints—on roads, in parks, and beyond. Understand, visualize, and track radiation to keep informed and safe.

---

> **"Isotopes can't talk, but this program tells their story."**
