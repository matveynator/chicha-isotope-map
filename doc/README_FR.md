[![Latest stable release build](https://github.com/matveynator/chicha-isotope-map/actions/workflows/release.yml/badge.svg)](https://github.com/matveynator/chicha-isotope-map/actions/workflows/release.yml)

<img width="30%" align="left" alt="chicha-isotope-map" src="https://github.com/user-attachments/assets/39bfa7b1-03fb-43dd-89bd-8d6c516fd4db" />

- [🇬🇧 English](/README.md)
- [🇫🇷 Français](/doc/README_FR.md)
- [🇯🇵 日本語](/doc/README_JP.md)
- [🇷🇺 Русский](/doc/README_RU.md)

# ☢️ Carte mondiale de la radiation
Cette carte est pensée pour qu’un visiteur sans préparation voie immédiatement si la radiation menace les maisons, champs, forêts ou points d’eau autour de lui. Les lieux sains tournent autour de 2–3 µR/h ; les zones plus sombres viennent presque toujours de l’activité humaine. La carte montre comment les mines d’uranium en Tchéquie, Russie, Kazakhstan ou Mongolie ont laissé de longues traces ; comment Fukushima ressort comme une « tache-tumeur » noir et rouge sur la côte japonaise ; comment Tchernobyl et la région de Briansk marquent le paysage ; comment les filons riches en radon en France, en Tchéquie ou aux Eaux minérales du Caucase augmentent les risques. Le lessivage de l’uranium et des terres rares laisse des sels solubles en profondeur : ils gagnent les nappes phréatiques, puis notre eau et notre nourriture. Si cette carte protège ne serait-ce qu’une personne ou un animal, elle aura servi.

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

Le projet progresse grâce au soutien actif de **Safecast** et de la communauté : beaucoup d’idées précieuses viennent de **Rob Oudendijk** et des passionnés de dosimétrie ouverte dans le monde (merci à Greenpeace et aux autres équipes environnementales).

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
- Sur la carte, cliquez sur le bouton vert **Upload** et déposez vos traces (`.kml`, `.kmz`, `.json`, `.rctrk`, `.csv`, `.gpx`, journaux bGeigie Nano/Zen `$BNRDD`, exports AtomFast, RadiaCode, Safecast, etc.).
- Commencer avec l’archive prête de pelora.org : téléchargez [https://pelora.org/api/json/weekly.tgz](https://pelora.org/api/json/weekly.tgz) et chargez-la avec le même bouton vert, ou lancez une fois le binaire avec `-import-tgz-url https://pelora.org/api/json/weekly.tgz` pour pré-remplir automatiquement puis quitter avant un démarrage normal.

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
- Plus il y a de nœuds, plus il est difficile de rater une contamination.

Chicha-Isotope-Map est inspirée par les travaux de terrain de **Dmitry Ignatenko** et par **Rob Oudendijk** et le projet **Safecast**. Les données ouvertes des communautés AtomFast et Radiacode la rendent utile au quotidien. Si la carte sauve ne serait-ce qu’une vie, ce travail n’aura pas été vain.
