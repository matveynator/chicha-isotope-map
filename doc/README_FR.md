[![Latest stable release build](https://github.com/matveynator/chicha-isotope-map/actions/workflows/release.yml/badge.svg)](https://github.com/matveynator/chicha-isotope-map/actions/workflows/release.yml)

<img width="30%" align="left" alt="chicha-isotope-map" src="https://github.com/user-attachments/assets/39bfa7b1-03fb-43dd-89bd-8d6c516fd4db" />

- [🇬🇧 English](/README.md)
- [🇫🇷 Français](/doc/README_FR.md)
- [🇯🇵 日本語](/doc/README_JP.md)
- [🇷🇺 Русский](/doc/README_RU.md)
- [🇨🇭 Schwiizerdütsch](/doc/README_DE_CH.md)
- [🇮🇹 Italiano](/doc/README_IT.md)
- [🇨🇳 中文](/doc/README_ZH.md)
- [🇮🇳 हिन्दी](/doc/README_HI.md)
- [🇮🇷 فارسی](/doc/README_FA.md)
- [🇲🇳 Монгол](/doc/README_MN.md)
- [🇰🇿 Қазақша](/doc/README_KK.md)

# ☢️ Carte mondiale de la radiation
Cette carte est conçue pour que chacun voie rapidement si l’endroit où il vit ou travaille est sûr. Beaucoup cultivent des légumes, élèvent des animaux ou boivent l’eau des sources sans toujours savoir si l’environnement est sain.

Le fond naturel reste faible. Le danger n’apparaît que là où les niveaux montent nettement — à cause de l’activité humaine ou des spécificités locales. Dans ces lieux, l’eau, l’air et le sol peuvent finir par affecter la santé : poumons, estomac et autres organes.

Si cette carte protège ne serait-ce qu’une personne ou un animal, elle aura été utile. Qu’elle serve de repère simple et clair pour choisir un chemin plus sûr.

Démo en ligne : [https://pelora.org/](https://pelora.org/) — votre nœud aura le même aspect.

👉 [Page de téléchargement unique](https://github.com/matveynator/chicha-isotope-map/releases) (toutes plateformes, dernières versions)

👉 [DeepWiki : Chicha Isotope Map](https://deepwiki.com/matveynator/chicha-isotope-map)

---

### 📸 Exemple
<a href="https://pelora.org" target="_blank"><img width="800" alt="pelora.org chicha-isotope-map exemple" src="https://github.com/user-attachments/assets/be706959-a2d5-4949-9378-811f4022aa98" /></a>

---

## 🧭 Ce que contient la carte
- La carte rassemble les mesures de nombreux instruments ; les couches sont séparées selon la vitesse de déplacement — à pied, en voiture ou en vol.
- Vous pouvez téléverser vos propres traces : de nouveaux points apparaissent immédiatement sur la carte pour éclairer la situation.
- Importez des archives par URL ou fichier, et sauvegardez vos données en archive (pratique pour la sauvegarde).
- Suivez comment la radiation a évolué dans un lieu précis — si la situation s’améliore ou se dégrade.
- Créez un lien court vers n’importe quelle zone de la carte.
- Mode impression : marquez les zones dangereuses avec des QR codes pour qu’une personne puisse scanner et voir aussitôt le niveau exact sur ce point. C’est utile pour signaler les risques environnementaux où il vaut mieux éviter de boire, de rester longtemps ou d’exploiter la terre. Les écologues, spécialistes du suivi et services d’alerte peuvent ainsi prévenir efficacement.
- La carte dispose d’une API pour intégrer ses données dans des services externes sous licence CC ouverte.

Le projet progresse grâce au soutien attentif de la communauté **Safecast**, à l’énorme travail de **Rob Oudendijk** et aux efforts de nombreuses personnes dans le monde engagées dans la dosimétrie ouverte. Nous remercions Safecast, AtomFast, Radiacode, DoseMap et d’autres initiatives pour leurs contributions et leur participation.

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
- Miroir instantané de pelora.org : exécutez `chicha-isotope-map -import-tgz-url https://pelora.org/api/json/weekly.tgz` une seule fois — il récupère l’archive hebdomadaire, remplit votre base puis s’arrête pour que le lancement suivant démarre déjà avec des données réelles.
- Vous préférez télécharger l’archive avant ? Téléchargez [https://pelora.org/api/json/weekly.tgz](https://pelora.org/api/json/weekly.tgz), indiquez `-import-tgz-path /chemin/vers/weekly.tgz` et démarrez avec votre propre copie locale.

### 🗺️ Premier démarrage en une commande avec des données réelles
Pour un poste tout neuf, cette commande charge les mesures existantes puis sert la carte immédiatement :
```bash
chicha-isotope-map -import-tgz-url https://pelora.org/api/json/weekly.tgz
```
Après l’import, relancez normalement (ou gardez la même commande dans un service systemd) — la carte s’ouvre avec des mesures visibles sur [http://localhost:8765](http://localhost:8765).

### 🛢️ Choisir sa base pour l’import et l’usage courant
- **PostgreSQL (`pgx`)** — la plus rapide et la plus confortable avec plusieurs utilisateurs. Exemple : `chicha-isotope-map -db-type pgx -db-conn postgres://USER:PASS@HOST:PORT/DATABASE?sslmode=allow -import-tgz-url https://pelora.org/api/json/weekly.tgz`
- **DuckDB / SQLite / Chai** — bases fichiers simples pour un seul utilisateur. Des écritures concurrentes peuvent entrer en conflit, réservez-les donc aux cartes personnelles. Exemple : `chicha-isotope-map -db-type duckdb -import-tgz-url https://pelora.org/api/json/weekly.tgz`

## 📤 Exporter
- Trace unique : `/api/track/{trackID}.json` (les anciens `.cim` fonctionnent aussi).
- Archive planifiée : `/api/json/weekly.tgz` (ou `/daily.tgz`, `/monthly.tgz`, `/yearly.tgz`). À l’intérieur : un JSON par trace.

---

## 🧠 Options avancées
- Bases de données : SQLite intégrée par défaut ; possibilité de passer à DuckDB, Chai, ClickHouse ou PostgreSQL (`pgx`).
- Import : via URL ou fichier ; vous pouvez fournir directement une archive.
- Export : archives JSON, trace unique, anciens `.cim` pris en charge.
- Apparence : coordonnées et couche de départ (`-default-*`).

---

## 🤝 Pourquoi héberger son nœud et un peu d’histoire
- Nous voulions que chacun, sans formation, voie si la radiation menace l’endroit où il vit, cultive ou puise l’eau.
- Plus il y a de nœuds, plus il est difficile de rater une contamination.

Chicha-Isotope-Map est inspirée par les travaux de terrain de **Dmitry Ignatenko** et par **Rob Oudendijk** et le projet **Safecast**. Les données ouvertes des communautés AtomFast et Radiacode la rendent utile au quotidien. Si la carte sauve ne serait-ce qu’une vie, ce travail n’aura pas été vain.
