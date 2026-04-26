<div align="center">
  <img src="https://raw.githubusercontent.com/matveynator/chicha-isotope-map/main/public_html/images/chicha-isotope-map-round-logo.png" alt="Chicha Isotope Map logo" width="120" />

# Chicha Isotope Map

Download page for the latest stable build: <a href="https://matveynator.github.io/chicha-isotope-map/">matveynator.github.io/chicha-isotope-map</a> <br>
Radiacode, AtomFast, BGeigie Safecast devices supported.

[![Latest stable release build](https://github.com/matveynator/chicha-isotope-map/actions/workflows/release.yml/badge.svg)](https://github.com/matveynator/chicha-isotope-map/actions/workflows/release.yml)


[Live demo](https://pelora.org/)

</div>


## Downloads (Stable Release)

- **Smart download page (recommended):** https://matveynator.github.io/chicha-isotope-map/
- **Stable Release tag (all artifacts):** https://github.com/matveynator/chicha-isotope-map/releases/tag/stable-release

### Desktop app (GUI)

Pick your OS first, then architecture.

| OS | Architecture / Variant | Artifact |
|---|---|---|
| macOS | Universal (Intel + Apple Silicon) | [chicha-isotope-map_darwin_universal_desktop.dmg](https://github.com/matveynator/chicha-isotope-map/releases/download/stable-release/chicha-isotope-map_darwin_universal_desktop.dmg) |
| Windows | amd64 | [chicha-isotope-map_windows_amd64_desktop.zip](https://github.com/matveynator/chicha-isotope-map/releases/download/stable-release/chicha-isotope-map_windows_amd64_desktop.zip) |
| Windows | arm64 | [chicha-isotope-map_windows_arm64_desktop.zip](https://github.com/matveynator/chicha-isotope-map/releases/download/stable-release/chicha-isotope-map_windows_arm64_desktop.zip) |
| Linux GTK 4.0 (Ubuntu 22.04 / Mint 21.x) | amd64 | [chicha-isotope-map_linux_amd64_desktop_gtk40.zip](https://github.com/matveynator/chicha-isotope-map/releases/download/stable-release/chicha-isotope-map_linux_amd64_desktop_gtk40.zip) |
| Linux GTK 4.0 (Ubuntu 22.04 / Mint 21.x) | arm64 | [chicha-isotope-map_linux_arm64_desktop_gtk40.zip](https://github.com/matveynator/chicha-isotope-map/releases/download/stable-release/chicha-isotope-map_linux_arm64_desktop_gtk40.zip) |
| Linux GTK 4.1 (Ubuntu 24.04+ / Mint 22+) | amd64 | [chicha-isotope-map_linux_amd64_desktop_gtk41.zip](https://github.com/matveynator/chicha-isotope-map/releases/download/stable-release/chicha-isotope-map_linux_amd64_desktop_gtk41.zip) |
| Linux GTK 4.1 (Ubuntu 24.04+ / Mint 22+) | arm64 | [chicha-isotope-map_linux_arm64_desktop_gtk41.zip](https://github.com/matveynator/chicha-isotope-map/releases/download/stable-release/chicha-isotope-map_linux_arm64_desktop_gtk41.zip) |

### Server binaries (self-hosted)

| OS | Architecture | Artifact |
|---|---|---|
| Linux | amd64 | [chicha-isotope-map_linux_amd64](https://github.com/matveynator/chicha-isotope-map/releases/download/stable-release/chicha-isotope-map_linux_amd64) |
| Linux | arm64 | [chicha-isotope-map_linux_arm64](https://github.com/matveynator/chicha-isotope-map/releases/download/stable-release/chicha-isotope-map_linux_arm64) |
| Windows | amd64 | [chicha-isotope-map_windows_amd64.exe](https://github.com/matveynator/chicha-isotope-map/releases/download/stable-release/chicha-isotope-map_windows_amd64.exe) |
| Windows | arm64 | [chicha-isotope-map_windows_arm64.exe](https://github.com/matveynator/chicha-isotope-map/releases/download/stable-release/chicha-isotope-map_windows_arm64.exe) |
| macOS | amd64 | [chicha-isotope-map_darwin_amd64](https://github.com/matveynator/chicha-isotope-map/releases/download/stable-release/chicha-isotope-map_darwin_amd64) |
| macOS | arm64 | [chicha-isotope-map_darwin_arm64](https://github.com/matveynator/chicha-isotope-map/releases/download/stable-release/chicha-isotope-map_darwin_arm64) |
| FreeBSD | amd64 / arm64 | [Stable Release assets](https://github.com/matveynator/chicha-isotope-map/releases/tag/stable-release) |
| OpenBSD | amd64 / arm64 | [Stable Release assets](https://github.com/matveynator/chicha-isotope-map/releases/tag/stable-release) |

### Quick run

Windows:
1. Download a `.zip` desktop build (or `.exe` server build).
2. Extract if needed.
3. Run the binary. If SmartScreen appears: **More info → Run anyway**.

macOS (server binary example):
```bash
chmod +x ./chicha-isotope-map_darwin_*
./chicha-isotope-map_darwin_*
```

Linux desktop (GTK 4.1 amd64 example):
```bash
curl -fL https://github.com/matveynator/chicha-isotope-map/releases/download/stable-release/chicha-isotope-map_linux_amd64_desktop_gtk41.zip -o chicha-isotope-map-desktop.zip
unzip chicha-isotope-map-desktop.zip
chmod +x ./chicha-isotope-map_linux_amd64_desktop_gtk41
./chicha-isotope-map_linux_amd64_desktop_gtk41
```

Linux server quick install:
```bash
sudo curl -fL https://github.com/matveynator/chicha-isotope-map/releases/download/stable-release/chicha-isotope-map_linux_amd64 -o /usr/local/bin/chicha-isotope-map
sudo chmod +x /usr/local/bin/chicha-isotope-map
/usr/local/bin/chicha-isotope-map
```
Open: http://localhost:8765

### Autostart service scripts (copy/paste)
The commands below follow the stable download page conventions (Linux systemd setup and FreeBSD/OpenBSD quick install) and use stable-release artifact names.

#### Linux + systemd (amd64)
```bash
sudo bash -c 'set -euo pipefail
curl -fL https://github.com/matveynator/chicha-isotope-map/releases/download/stable-release/chicha-isotope-map_linux_amd64 -o /usr/local/bin/chicha-isotope-map
chmod +x /usr/local/bin/chicha-isotope-map
cat >/etc/systemd/system/chicha-isotope-map.service <<"UNIT"
[Unit]
Description=Chicha Isotope Map
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=/usr/local/bin/chicha-isotope-map -port 8765
Restart=always
RestartSec=2

[Install]
WantedBy=multi-user.target
UNIT
systemctl daemon-reload
systemctl enable --now chicha-isotope-map
systemctl status --no-pager chicha-isotope-map || true'
```

#### Linux + systemd (arm64)
```bash
sudo bash -c 'set -euo pipefail
curl -fL https://github.com/matveynator/chicha-isotope-map/releases/download/stable-release/chicha-isotope-map_linux_arm64 -o /usr/local/bin/chicha-isotope-map
chmod +x /usr/local/bin/chicha-isotope-map
cat >/etc/systemd/system/chicha-isotope-map.service <<"UNIT"
[Unit]
Description=Chicha Isotope Map
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=/usr/local/bin/chicha-isotope-map -port 8765
Restart=always
RestartSec=2

[Install]
WantedBy=multi-user.target
UNIT
systemctl daemon-reload
systemctl enable --now chicha-isotope-map
systemctl status --no-pager chicha-isotope-map || true'
```

#### FreeBSD (rc.d, amd64/arm64)
```bash
# amd64: change URL to *_arm64 for arm64 hosts
sudo sh -c 'set -eu
fetch -o /usr/local/bin/chicha-isotope-map https://github.com/matveynator/chicha-isotope-map/releases/download/stable-release/chicha-isotope-map_freebsd_amd64
chmod +x /usr/local/bin/chicha-isotope-map
cat >/usr/local/etc/rc.d/chicha_isotope_map <<"RC"
#!/bin/sh
# PROVIDE: chicha_isotope_map
# REQUIRE: NETWORKING
# KEYWORD: shutdown

. /etc/rc.subr

name="chicha_isotope_map"
rcvar="${name}_enable"
command="/usr/local/bin/chicha-isotope-map"
command_args="-port 8765 >> /var/log/chicha-isotope-map.log 2>&1"
pidfile="/var/run/${name}.pid"

load_rc_config $name
: ${chicha_isotope_map_enable:=NO}

run_rc_command "$1"
RC
chmod +x /usr/local/etc/rc.d/chicha_isotope_map
sysrc chicha_isotope_map_enable=YES
service chicha_isotope_map start'
```

#### OpenBSD (rc.d, amd64/arm64)
```bash
# amd64: change URL to *_arm64 for arm64 hosts
sudo sh -c 'set -eu
ftp -o /usr/local/bin/chicha-isotope-map https://github.com/matveynator/chicha-isotope-map/releases/download/stable-release/chicha-isotope-map_openbsd_amd64
chmod +x /usr/local/bin/chicha-isotope-map
cat >/etc/rc.d/chicha_isotope_map <<"RC"
#!/bin/ksh

daemon="/usr/local/bin/chicha-isotope-map"
daemon_flags="-port 8765"

. /etc/rc.d/rc.subr

rc_bg=YES
rc_cmd $1
RC
chmod +x /etc/rc.d/chicha_isotope_map
rcctl enable chicha_isotope_map
rcctl start chicha_isotope_map'
```

#### NetBSD (rc.d, amd64/arm64)
```bash
# Build binary on NetBSD first (replace GOARCH with arm64 when needed):
# GOOS=netbsd GOARCH=amd64 go build -o chicha-isotope-map .

sudo sh -c 'set -eu
install -m 0755 ./chicha-isotope-map /usr/pkg/bin/chicha-isotope-map
cat >/etc/rc.d/chicha_isotope_map <<"RC"
#!/bin/sh
# PROVIDE: chicha_isotope_map
# REQUIRE: NETWORKING
# KEYWORD: shutdown

. /etc/rc.subr

name="chicha_isotope_map"
rcvar=$name
command="/usr/pkg/bin/chicha-isotope-map"
command_args="-port 8765"
pidfile="/var/run/${name}.pid"

load_rc_config $name
: ${chicha_isotope_map:=NO}

run_rc_command "$1"
RC
chmod +x /etc/rc.d/chicha_isotope_map
if ! grep -q '^chicha_isotope_map=YES' /etc/rc.conf; then echo chicha_isotope_map=YES >> /etc/rc.conf; fi
service chicha_isotope_map start'
```

---

## Program screenshots

Screenshots are taken from the official stable download page:

<img src="https://matveynator.github.io/chicha-isotope-map/chicha-isotope-map-macosx-universal.png">
<img src="https://matveynator.github.io/chicha-isotope-map/windows-google-light.png">
<img src="https://matveynator.github.io/chicha-isotope-map/linux-amd64-desktop.png">

---
## Configuration and deployment

### Useful flags
- `-port 8765`
- `-domain your-domain.example`
- `-default-lat`, `-default-lon`, `-default-zoom`, `-default-layer`
- `-mapbox-token YOUR_TOKEN`
- `-setup` (Linux only)
- `-import-tgz-url URL`
- `-import-tgz-path /path/to/file.tgz`

### Database flags
- `-db-type sqlite|duckdb|chai|clickhouse|pgx`
- `-db-path /path/to/file`
- `-db-conn CONNECTION_STRING`

### PostgreSQL example
```bash
./chicha-isotope-map \
  -db-type pgx \
  -db-conn 'postgres://USER:PASSWORD@HOST:5432/chicha?sslmode=allow' \
  -port 8765
```

### Public HTTPS domain
```bash
./chicha-isotope-map -domain your-domain.example
```
Ports `80/443` must be open.

### Preload real tracks once
```bash
./chicha-isotope-map -import-tgz-url https://pelora.org/api/json/weekly.tgz
```

### Build from source

Desktop WebView builds require CGO.

```bash
CGO_ENABLED=1 go build -tags desktop .
./chicha-isotope-map -desktop
```

Server-only binary (no embedded desktop window):
```bash
CGO_ENABLED=0 go build .
./chicha-isotope-map
```

---

## Universal isotope catalog and spectrum drivers

The project now has a device-agnostic spectrum pipeline in `pkg/spectrum/`:

- Common `Driver` interface for instrument parsers (`CanParse` + `Parse`).
- Built-in Radiacode XML driver as the first implementation.
- Shared `SpectrumMeasurement` model so AtomSpectra and other devices can be plugged in without rewriting isotope analysis.
- Shared analyzer that detects peaks and maps alpha/beta/gamma energies to nuclides from one common catalog.
- Composite-spectrum estimator that ranks two- and three-nuclide mixtures when peaks overlap in scintillation detectors.

Catalog scope in the built-in dataset:
- Cosmogenic and primordial isotopes (`H-3`, `C-14`, `K-40`, etc.).
- Full natural decay families used in field dosimetry:
  - Thorium-232 chain,
  - Uranium-238 chain,
  - Uranium-235 (actinium) chain,
  - Neptunium-237 family.
- Common environmental/industrial/medical/fallout isotopes (`Cs-137`, `Co-60`, `I-131`, `Am-241`, `Eu-152`, ...).
- Heavy transuranics up to `Og-294` for completeness of identifier parsing.

This gives one universal parser entrypoint and one extensible isotope catalog, while keeping drivers isolated by format.

---

## Acknowledgements

This project was conceived to grant people a clear and immediate understanding of radiation safety in the very places they inhabit—where they reside, labor, cultivate the land, and draw water.

The Chicha Isotope Map finds its roots in the field research of [Dmitry Ignatenko](https://www.youtube.com/@MrDrimogemon) and has been profoundly shaped by the insights of Rob Oudendijk and the [Safecast community](https://safecast.org). We extend our sincere appreciation to [Safecast](https://simplemap.safecast.org), [AtomFast](https://atomfast.net), [Radiacode](https://radiacode.com), [DoseMap](https://dosemap.org), and the many contributors to open dosimetry whose efforts made this possible.

Should this work serve to safeguard even a single living being, its purpose shall be fully justified.
