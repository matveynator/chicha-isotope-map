[![Latest stable release build](https://github.com/matveynator/chicha-isotope-map/actions/workflows/release.yml/badge.svg)](https://github.com/matveynator/chicha-isotope-map/actions/workflows/release.yml)

<img width="30%" align="left" alt="chicha-isotope-map" src="https://github.com/user-attachments/assets/39bfa7b1-03fb-43dd-89bd-8d6c516fd4db" />

- [🇬🇧 English](/README.md)
- [🇫🇷 Français](/doc/README_FR.md)
- [🇯🇵 日本語](/doc/README_JP.md)
- [🇷🇺 Русский](/doc/README_RU.md)

# ☢️ Carte mondiale de la radiation
Nous gardons cette carte simple et sobre, dans l’esprit de Dmitri Likhatchov : un débutant doit voir immédiatement s’il y a de la radiation près de chez lui, là où il vit, cultive, cueille des champignons et des herbes, fait paître le bétail ou puise l’eau. Dans la nature, la plupart des forêts, champs et rivières restent autour de 2–3 µR/h ; ce qui dépasse vient le plus souvent de l’activité humaine. On voit comment les mines d’uranium en Tchéquie, Russie, Kazakhstan ou Mongolie ont laissé de longues cicatrices ; comment Fukushima a créé une tache sombre ; comment Tchernobyl et la région de Briansk sont devenues des « tumeurs » sur la carte ; comment les filons riches en radon en France, en Tchéquie ou aux Eaux minérales du Caucase augmentent le risque de cancer du poumon et de l’estomac. Le lessivage de l’uranium et des terres rares laisse des sels solubles en profondeur ; ils gagnent les nappes phréatiques, puis notre eau et notre nourriture. Si cette carte protège ne serait-ce qu’une personne ou un animal, elle aura servi.

Démo en ligne : [https://pelora.org/](https://pelora.org/) — votre nœud aura le même aspect.

👉 [Page de téléchargement unique](https://github.com/matveynator/chicha-isotope-map/releases) (toutes plateformes, dernières versions)

👉 [DeepWiki : Chicha Isotope Map](https://deepwiki.com/matveynator/chicha-isotope-map)

---

### 📸 Exemple
<a href="https://pelora.org" target="_blank"><img width="800" alt="pelora.org chicha-isotope-map exemple" src="https://github.com/user-attachments/assets/be706959-a2d5-4949-9378-811f4022aa98" /></a>

---

## 🧭 Ce que contient la carte
- Carte en direct avec mesures de nombreux détecteurs ; choisissez le fond qui vous plaît.
- Téléversez vos traces ; les points récents apparaissent autour de la zone affichée.
- Import par URL ou fichier, export en archive.
- Fonctionne en nœud unique ou en réseau : plus il y a de nœuds, plus la transparence est grande.

Le projet progresse grâce à la communauté : beaucoup d’idées précieuses viennent de **Rob Alden** et des passionnés de dosimétrie ouverte dans le monde (merci à Greenpeace et aux autres équipes environnementales).

---

## 🚀 Démarrage rapide (débutant)
Le chemin le plus simple : télécharger le binaire. Pas de Docker, pas de base de données, pas d’outils supplémentaires — télécharger, lancer, c’est prêt.

### Option 1. Binaire (recommandé)
1) Ouvrez la [page des versions](https://github.com/matveynator/chicha-isotope-map/releases) et téléchargez le binaire pour votre système.
2) Rendez-le exécutable et lancez-le :
```bash
chmod +x ./chicha-isotope-map
./chicha-isotope-map
```
3) Ouvrez [http://localhost:8765](http://localhost:8765) — la carte est déjà en ligne.

Réglages facultatifs :
- `-port 8765` — port local.
- `-domain maps.example.org` — HTTPS via Let’s Encrypt (ports 80/443 nécessaires).
- `-default-lat` / `-default-lon` / `-default-zoom` / `-default-layer` — vue initiale.
- Stockage : `-db-type sqlite|duckdb|chai|clickhouse|pgx`, `-db-path` pour les bases fichiers, `-db-conn` pour les bases réseau.

### Option 2. Nœud public avec domaine
1) Lancez le binaire avec votre domaine :
```bash
./chicha-isotope-map -domain example.org
```
2) Laissez libres les ports 80/443 pour Let’s Encrypt. Une fois le certificat obtenu, la carte sera sur [https://example.org](https://example.org).

### Option 3. Docker (tout emballé)
1) Installez Docker (Desktop ou CLI).
2) Trouvez **matveynator/chicha-isotope-map** sur Docker Hub et cliquez sur **Run** (ou exécutez une commande) :
```bash
docker run -d -p 8765:8765 --name chicha-isotope-map matveynator/chicha-isotope-map:latest
```
3) Ouvrez [http://localhost:8765](http://localhost:8765) — c’est prêt.

---

## 📥 Importer des données
- Base prête à l’emploi : un archive complète est disponible sur [pelora.org](https://pelora.org/) ; indiquez son URL dans le chargeur ou téléchargez-la puis ajoutez-la via **Upload**.
- Import web : **Upload** → choisissez vos fichiers (`.kml`, `.kmz`, `.json`, `.rctrk`, `.csv`, `.gpx`, journaux bGeigie Nano/Zen `$BNRDD`, exports AtomFast, RadiaCode, Safecast, etc.).
- Import API : `curl -F 'files[]=@/chemin/vers/fichier.log' http://localhost:8765/upload` (diagnostic : `/upload_diag`).

## 📤 Exporter
- Trace unique : `/api/track/{trackID}.json` (les anciens `.cim` fonctionnent aussi).
- Archive planifiée : `/api/json/weekly.tgz` (ou `/daily.tgz`, `/monthly.tgz`, `/yearly.tgz`). À l’intérieur : un JSON par trace.

---

## 🧠 Options avancées
- Bases de données : SQLite intégrée par défaut ; possibilité de passer à DuckDB, Chai, ClickHouse ou PostgreSQL (`pgx`).
- Import : via URL ou fichier, archives acceptées.
- Export : archives JSON, trace unique, anciens `.cim` pris en charge.
- Apparence : coordonnées et couche de départ (`-default-*`).

---

## 🤝 Pourquoi héberger son nœud et un peu d’histoire
- Nous voulions que chacun, sans formation, voie si la radiation menace l’endroit où il vit, cultive ou puise l’eau.
- Votre nœud donne une ligne de base et une histoire (souvent 0,8–4 µR/h), ce qui rend les écarts visibles.
- Plus il y a de nœuds, plus il est difficile de rater une contamination.

Chicha-Isotope-Map a été créée pour le **laboratoire Dmitry Ignatenko**, inspirée par **Safecast**, et portée par les données ouvertes des communautés AtomFast et Radiacode. Si la carte sauve ne serait-ce qu’une vie, ce travail n’aura pas été vain.
