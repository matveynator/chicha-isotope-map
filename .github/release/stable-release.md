<div align="center">
  <img src="https://raw.githubusercontent.com/matveynator/chicha-isotope-map/main/public_html/images/chicha-isotope-map-round-logo.png" alt="Chicha Isotope Map logo" width="120" />

# Chicha Isotope Map — Stable Download Page

Download and run in minutes on **Windows**, **macOS**, and **Linux**.
</div>

---

## 🖥️ Desktop app (GUI)

> Best choice for personal use: click, launch, and use the map locally.

### Windows
[⬇️ Download Desktop for Windows (amd64)](https://github.com/matveynator/chicha-isotope-map/releases/download/stable-release/chicha-isotope-map_windows_amd64_desktop.exe)

1. Download the `.exe` file.
2. Double-click to launch.
3. If Windows SmartScreen appears, click **More info → Run anyway**.

### macOS
- [⬇️ Download Desktop for macOS Apple Silicon (arm64)](https://github.com/matveynator/chicha-isotope-map/releases/download/stable-release/chicha-isotope-map_darwin_arm64_desktop)
- [⬇️ Download Desktop for macOS Intel (amd64)](https://github.com/matveynator/chicha-isotope-map/releases/download/stable-release/chicha-isotope-map_darwin_amd64_desktop)

1. Download the right binary for your Mac.
2. Open Terminal in the download folder.
3. Run:

```bash
chmod +x ./chicha-isotope-map_darwin_*_desktop
./chicha-isotope-map_darwin_*_desktop
```

If Gatekeeper blocks launch, open **System Settings → Privacy & Security** and allow the app.

### Linux
- [⬇️ Download Desktop for Linux (amd64, zip)](https://github.com/matveynator/chicha-isotope-map/releases/download/stable-release/chicha-isotope-map_linux_amd64_desktop.zip)
- [⬇️ Download Desktop for Linux (arm64, zip)](https://github.com/matveynator/chicha-isotope-map/releases/download/stable-release/chicha-isotope-map_linux_arm64_desktop.zip)

```bash
curl -fL https://github.com/matveynator/chicha-isotope-map/releases/download/stable-release/chicha-isotope-map_linux_amd64_desktop.zip -o chicha-isotope-map-desktop.zip
unzip chicha-isotope-map-desktop.zip
chmod +x ./chicha-isotope-map_linux_amd64_desktop
./chicha-isotope-map_linux_amd64_desktop
```

---

## 🛠️ Server app (self-hosted node)

> Best choice for VPS/server deployment and always-on public/private node.

### Linux quick install (curl + run)

```bash
curl -fL https://github.com/matveynator/chicha-isotope-map/releases/download/stable-release/chicha-isotope-map_linux_amd64 -o chicha-isotope-map
chmod +x ./chicha-isotope-map
./chicha-isotope-map
```

Then open: http://localhost:8765

### macOS server binary
- [⬇️ Download server for macOS Apple Silicon (arm64)](https://github.com/matveynator/chicha-isotope-map/releases/download/stable-release/chicha-isotope-map_darwin_arm64)
- [⬇️ Download server for macOS Intel (amd64)](https://github.com/matveynator/chicha-isotope-map/releases/download/stable-release/chicha-isotope-map_darwin_amd64)

Run in Terminal:

```bash
chmod +x ./chicha-isotope-map_darwin_*
./chicha-isotope-map_darwin_*
```

### Windows server binary
[⬇️ Download server for Windows (amd64)](https://github.com/matveynator/chicha-isotope-map/releases/download/stable-release/chicha-isotope-map_windows_amd64.exe)

PowerShell quick run:

```powershell
Invoke-WebRequest -Uri "https://github.com/matveynator/chicha-isotope-map/releases/download/stable-release/chicha-isotope-map_windows_amd64.exe" -OutFile "chicha-isotope-map.exe"
.\chicha-isotope-map.exe
```

---

## 📦 More binaries

Need another platform or architecture (arm64, FreeBSD, OpenBSD, etc.)?
Open **Assets** in this release and pick the matching file name.

- Stable URL (always current):
  https://github.com/matveynator/chicha-isotope-map/releases/tag/stable-release

---

## 🚀 First launch tips

- Default local URL: `http://localhost:8765`
- Public domain mode with HTTPS (Let's Encrypt):

```bash
./chicha-isotope-map -domain your-domain.example
```

- Linux guided service setup:

```bash
./chicha-isotope-map -setup
```
