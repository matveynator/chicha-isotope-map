[![Dernière version stable](https://github.com/matveynator/chicha-isotope-map/actions/workflows/release.yml/badge.svg)](https://github.com/matveynator/chicha-isotope-map/actions/workflows/release.yml)

<img width="30%" align="left" alt="chicha-isotope-map" src="https://github.com/user-attachments/assets/39bfa7b1-03fb-43dd-89bd-8d6c516fd4db" />

* [🇬🇧 English](/README.md)
* [🇫🇷 Français](/doc/README_FR.md)
* [🇯🇵 日本語](/doc/README_JP.md)
* [🇷🇺 Русский](/doc/README_RU.md)

# ☢️ Carte mondiale de la radioactivité
Démo en ligne : [https://pelora.org/](https://pelora.org/) — votre nœud ressemble à cela.

👉 [DeepWiki : Chicha Isotope Map](https://deepwiki.com/matveynator/chicha-isotope-map)

---

### 📸 Exemple
<a href="https://pelora.org" target="_blank"><img width="800" alt="pelora.org chicha-isotope-map" src="https://github.com/user-attachments/assets/be706959-a2d5-4949-9378-811f4022aa98" /></a>

---

## 🚀 Lancer avec Docker (le plus simple)
L’image contient déjà PostgreSQL. Copier, coller, c’est parti.

#### 🔥 En local (port 8765)
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
Ouvrir : [http://localhost:8765](http://localhost:8765)

#### 🔥 Nœud public avec HTTPS
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
Après l’émission Let’s Encrypt : [https://example.org](https://example.org)

**Variables :** `DOMAIN` pour HTTPS, `DEFAULT_LAT` / `DEFAULT_LON` / `DEFAULT_ZOOM` / `DEFAULT_LAYER` pour la vue initiale, `PORT` pour le port interne. Stockez les données sur `-v chicha-data:/var/lib/postgresql/data` pour garder l’historique lors des mises à jour du conteneur.

---

## ⬇️ Binaries prêts à l’emploi (sans Docker)
Téléchargez, rendez exécutable, lancez.

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

Autres plateformes (Windows / ARM / BSD) : [dernière version](https://github.com/matveynator/chicha-isotope-map/releases/tag/latest).

---

## 🖥 Exécuter le binaire
- `-domain maps.example.org` — HTTPS sur 80/443 (Let’s Encrypt).
- `-port 8765` — port HTTP pour un lancement local.
- `-default-lat` / `-default-lon` / `-default-zoom` / `-default-layer` — vue initiale.
- Stockage : `-db-type sqlite|duckdb|pgx|chai|clickhouse`, `-db-path` pour les bases fichiers, `-db-conn` pour les bases réseau.
- Outil : `-version` affiche la version.

DuckDB : `CGO_ENABLED=1 go build -tags duckdb`, puis `./chicha-isotope-map -db-type duckdb`.

---

## 📥 Importer des données
- Formats acceptés : `.kml`, `.kmz`, `.json`, `.rctrk`, `.csv`, `.gpx`, journaux bGeigie Nano/Zen `$BNRDD` (`.log` / `.txt`), exports AtomFast, RadiaCode, Safecast, etc.
- Interface web : ouvrir le nœud → **Upload** → choisir les fichiers → le dernier tracé importé s’ouvre automatiquement.
- API : `curl -F 'files[]=@/path/to/file.log' http://localhost:8765/upload` (diagnostics : `/upload_diag`).
- Points récents autour d’une position : `/api/latest?lat=...&lon=...&radius_m=1500&limit=20`.

---

## 📤 Exporter des données
- **Par tracé :** `/api/track/{trackID}.json` (les anciennes URLs `.cim` fonctionnent). `from`/`to` pour limiter les IDs.
- **Archive :** `/api/json/weekly.tgz` (ou `/daily.tgz`, `/monthly.tgz`, `/yearly.tgz` si configuré). Chaque tracé a son fichier JSON.
- **Schéma JSON :**
  - Niveau racine : `trackID`, `trackIndex` (position à partir de 1), `apiURL`, `firstID`, `lastID`, `markerCount`, `disclaimers`, `markers`.
  - Marqueur : `id`, `timeUnix`, `timeUTC` (RFC3339), `lat`, `lon`, options `altitudeM`, `temperatureC`, `humidityPercent`, vitesses (`speedMS`, `speedKMH`), doses (`doseRateMicroSvH`, `doseRateMicroRh`), `countRateCPS`, et le cas échéant `detectorType`, `detectorName`, `radiationTypes`.
  - Les `disclaimers` multilingues accompagnent chaque export.
- **À venir :** le même JSON accueillera probablement des données spectrométriques par point dès que nous commencerons à les stocker.

---

## 💾 Sauvegarde et restauration
- **Sauvegarde quotidienne (03:00)** : `0 3 * * * docker exec chicha-isotope-map pg_dump -U chicha_isotope_map chicha_isotope_map | gzip > /backup/chicha_isotope_map_$(date +\%F).sql.gz`
- **Restauration :**
  ```bash
  docker exec -it chicha-isotope-map psql -U postgres -c "DROP DATABASE IF EXISTS chicha_isotope_map; CREATE DATABASE chicha_isotope_map OWNER chicha_isotope_map;"
  zcat /backup/chicha_isotope_map_2025-07-24.sql.gz | docker exec -i chicha-isotope-map psql -U chicha_isotope_map chicha_isotope_map
  ```

---

## 🤝 Pourquoi héberger votre nœud ?
- Vos mesures, votre historique, pour votre communauté.
- Suivre l’évolution du bruit de fond (≈0,8–4 µR/h) localement.
- Plus de nœuds → plus de transparence et de résilience.

Chicha‑Isotope‑Map est créée pour le **Laboratoire Dmitry Ignatenko** et inspirée par **Safecast**. Merci aux communautés AtomFast et Radiacode pour le partage de leurs données.
