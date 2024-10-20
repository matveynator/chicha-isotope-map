# 🌐 **Isotope Pathways** — Language Selection

Please choose your preferred language:

- [🇺🇸 English](README.md)
- [🇫🇷 Français](README_FR.md)
- [🇯🇵 日本語](README_JP.md)
- [🇷🇺 Русский](README_RU.md)

---

# 🌌 **Isotope Pathways** — The Explorer of Invisible Roads

> **"Is there anyone who can see the invisible? No? Well, this program can. It takes radioactive traces, just like an old shaman reads from ashes, and brings them to life on the screen — colorful, glowing, alive."**

---

## 📖 **About the Project**

**Isotope Pathways** isn’t just a program; it’s a gateway to a world of invisible particles, now made visible. Imagine walking down the road, and beneath your feet, radioactive isotopes are dancing. This program reveals them. It creates a map where every isotope leaves a trace, from green to red, from calm to warning.

It can read data from AtomFast and RadiaCode formats, such as `.kml`, `.kmz`, `.json`, and `.rctrk`, and store them in its own database. So, years later, you can look back and say, "Back in 2024, right here, the radiation was 4.1 µR/h."

### 🌍 **Based on Nature**

We’ve built this program using the **natural background radiation** as a baseline. If you go to a clean, untouched place, you’ll likely see **3-4 microroentgens per hour**. That’s normal. At different altitudes, radiation levels vary, and the planet dances along with it.

Anything above this baseline is considered **foreign**. That’s what we call **radioactive contamination**. You can see how isotopes scatter across roads, carried by the wind, by people, and by vehicles. These small, invisible traces are like footprints left behind on freshly fallen snow.

---

### 📸 **Demo**

<a href="https://jutsa.ru" target="_blank">Check out the program in real-time here.</a>

---

### 📸 **Screenshots**

... In Soviet times, an open swimming pool was being built in Kislovodsk Park. Maybe they used concrete from a factory in Pyatigorsk, where radioactive ore from Mount Beshtau was once processed. Trucks drove down the roads, and dust from their wheels settled on the asphalt, leaving invisible marks. Years have passed, yet these traces still glow, like memories of the past. The dust that spread around the construction settled in the park — on the map, it shows up in yellow, like patches of autumn leaves. Everything else in the park remains clean, peaceful, and green.
<img src="https://repository-images.githubusercontent.com/870016860/11fd6abc-fe8b-4cd8-95c2-df1c631c8762">

---

## 📥 **Download and Get Started** 📥

Choose the version for your platform and start tracking isotope trails:

| Platform   | Download Link                                                                                           |
|------------|--------------------------------------------------------------------------------------------------------|
| AIX        | [Download for AIX](http://files.zabiyaka.net/isotope-pathways/latest/no-gui/aix/)                      |
| Android    | [Download for Android](http://files.zabiyaka.net/isotope-pathways/latest/no-gui/android/)               |
| Dragonfly  | [Download for Dragonfly](http://files.zabiyaka.net/isotope-pathways/latest/no-gui/dragonfly/)           |
| FreeBSD    | [Download for FreeBSD](http://files.zabiyaka.net/isotope-pathways/latest/no-gui/freebsd/)               |
| Illumos    | [Download for Illumos](http://files.zabiyaka.net/isotope-pathways/latest/no-gui/illumos/)               |
| JavaScript | [Download for JavaScript](http://files.zabiyaka.net/isotope-pathways/latest/no-gui/js/)                 |
| Linux      | [Download for Linux](http://files.zabiyaka.net/isotope-pathways/latest/no-gui/linux/)                   |
| macOS      | [Download for macOS](http://files.zabiyaka.net/isotope-pathways/latest/no-gui/mac/)                     |
| NetBSD     | [Download for NetBSD](http://files.zabiyaka.net/isotope-pathways/latest/no-gui/netbsd/)                 |
| OpenBSD    | [Download for OpenBSD](http://files.zabiyaka.net/isotope-pathways/latest/no-gui/openbsd/)               |
| Plan9      | [Download for Plan9](http://files.zabiyaka.net/isotope-pathways/latest/no-gui/plan9/)                   |
| Solaris    | [Download for Solaris](http://files.zabiyaka.net/isotope-pathways/latest/no-gui/solaris/)               |
| Windows    | [Download for Windows](http://files.zabiyaka.net/isotope-pathways/latest/no-gui/windows/)               |

Or build it yourself:

```bash
git clone https://github.com/matveynator/isotope-pathways.git
cd isotope-pathways
go build isotope-pathways.go
chmod +x ./isotope-pathways
./isotope-pathways
```

---

## 🛠 **How to Use?**

### Run the program:

```bash
./isotope-pathways
```

or with additional settings:

```bash
./isotope-pathways --port=8765 --db-type=genji --db-path=./path-to-database-file.8765.genji
```

- `--port`: The port number for the server to run on. Default is `8765`.  
  _"Want to talk to the program on a different channel? Change the port, and it will speak a new language to you."_

- `--db-type`: The type of database: `genji` or `sqlite`. Default is `genji`.  
  _"Prefer the old-school SQLite or the innovative Genji? The choice is yours."_

- `--db-path`: The path to the database file. If not specified, a file in the current directory is used.  
  _"Where should it hide the trails? Specify the path, and it will call it home."_

- `--version`: Show the program version and exit.  
  _"Like a magician revealing its era, the program will tell you where it comes from."_

### Web Interface:

1. Open <a href="http://localhost:8765" target="new">http://localhost:8765</a> in your browser.
2. Upload your data using the `Upload` button.
3. Hover over a marker — and the invisible world will open before you. Discover the radiation dose, the time of the measurement, and the location where isotopes left their marks.

---

## ☢️ **Radiation and Its Traces**

What is radiation? It’s like a whispering wind in the mountains, something no one hears, yet it’s there. But our program is someone with extraordinary hearing. It sees what you cannot. It will tell you where and when that extra microroentgen appeared. It will show you how isotopes scattered through the city, fell into a quiet pond, or got lost in the old forest. Their danger lies in the fact that they don’t just sit on the ground like forgotten coins. No, they seep into the soil, the water, and the plants. You go about your life, eating apples, drinking water from a well, and isotopes quietly sneak inside. The doses accumulate, like little bunnies multiplying in your body. Quietly, unnoticed. But still dangerous.

---

> **"If isotopes could speak, they would tell you their stories. But since they are silent, our program will speak for them."**

