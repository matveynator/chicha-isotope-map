[![Dernière version stable](https://github.com/matveynator/chicha-isotope-map/actions/workflows/release.yml/badge.svg)](https://github.com/matveynator/chicha-isotope-map/actions/workflows/release.yml)

<img align="left" width="20%" alt="chicha-isotope-map" src="https://github.com/user-attachments/assets/cff18b8a-860c-46f2-80e8-3d7943863382" />

* [🇬🇧 English](/README.md)
* [🇫🇷 Français](/doc/README_FR.md)
* [🇯🇵 日本語](/doc/README_JP.md)
* [🇷🇺 Русский](/doc/README_RU.md)

# ☢️ Сarte Mondiale de la Radioactivité.

Démo en direct: <a href="https://pelora.org" target="_blank">https://pelora.org</a>

---

### 📸 **Démo en direct**

<a href="https://pelora.org" target="_blank"><img width="800"  alt="pelora.org chicha-isotope-map exemple de démo" src="https://github.com/user-attachments/assets/be706959-a2d5-4949-9378-811f4022aa98" /></a>

---

La page **DeepWiki** consacrée à *Chicha Isotope Map* a été réalisée avec générosité par [Rob Ouden](https://github.com/robouden) du projet **Safecast**, à qui nous exprimons toute notre gratitude.
Elle dévoile la structure interne de notre programme, permettant aux développeurs d’en comprendre les bases, d’en suivre la logique et de poursuivre le travail en l’améliorant et en l’élargissant.
Grâce à DeepWiki, le code n’est plus seulement un outil : c’est un projet vivant, qui peut grandir et évoluer grâce à de nombreuses mains.

👉 [DeepWiki : Chicha Isotope Map](https://deepwiki.com/matveynator/chicha-isotope-map)

---

## 🚀 Installer & lancer votre propre nœud en 2 commandes

Sans fioritures. L’image Docker est livrée avec la base de données (PostgreSQL) intégrée. Copiez la commande, lancez-la — et c’est terminé.

#### 🔥 Local (port 8765)

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

Ouvrez : [http://localhost:8765](http://localhost:8765)

#### 🔥 Nœud public avec HTTPS sur votre domaine

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

Une fois le certificat émis, accédez à : [https://example.org](https://example.org)

---

### ⚙️ Configuration via variables d’environnement (l’essentiel)

* `DOMAIN` — active HTTPS sur les ports 80/443 avec certificats Let’s Encrypt automatiques (pour un nœud public).
* `DEFAULT_LAT`, `DEFAULT_LON` — coordonnées initiales de la carte.
* `DEFAULT_ZOOM` — niveau de zoom initial (11 = pratique pour la vue d’une ville).
* `DEFAULT_LAYER` — `OpenStreetMap` ou `Google Satellite`.
* `PORT` — port interne de l’application (par défaut 8765 ; inutile de le modifier sauf cas particulier).

> Astuce : conservez vos données sur le volume `-v chicha-data:/var/lib/postgresql/data` pour les garder lors des mises à jour du conteneur.

---

### 💾 Sauvegarde & restauration (simple)

**Sauvegarde quotidienne (03:00) :**

```bash
0 3 * * * docker exec chicha-isotope-map pg_dump -U chicha_isotope_map chicha_isotope_map | gzip > /backup/chicha_isotope_map_$(date +\%F).sql.gz
```

**Restauration depuis une archive :**

```bash
docker exec -it chicha-isotope-map psql -U postgres -c "DROP DATABASE IF EXISTS chicha_isotope_map; CREATE DATABASE chicha_isotope_map OWNER chicha_isotope_map;"

zcat /backup/chicha_isotope_map_2025-07-24.sql.gz | docker exec -i chicha-isotope-map psql -U chicha_isotope_map chicha_isotope_map
```

---

## ⬇️ Télécharger les applications précompilées (sans Docker)

Récupérez le binaire pour votre système, rendez-le exécutable, et lancez-le.

**Linux x64**

```bash
sudo curl -L https://github.com/matveynator/chicha-isotope-map/releases/download/latest/chicha-isotope-map_linux_amd64 \
  -o /usr/local/bin/chicha-isotope-map \
  && sudo chmod +x /usr/local/bin/chicha-isotope-map \
  && chicha-isotope-map
```

**macOS Intel (x86\_64)**

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

Autres plateformes (Windows / ARM / BSD) — voir la page des releases :
[https://github.com/matveynator/chicha-isotope-map/releases/tag/latest](https://github.com/matveynator/chicha-isotope-map/releases/tag/latest)

---

## 🖥 Exécuter le binaire (sans Docker) : options & exemples

Si vous lancez directement le binaire `chicha-isotope-map`, voici les options importantes.

### Essentielles

* `-domain string` — active HTTPS et lie les ports 80 et 443 avec certificats Let’s Encrypt automatiques. Votre domaine doit pointer vers le serveur, et les ports 80/443 être ouverts.

  * Exemple : `sudo chicha-isotope-map -domain maps.example.org -default-lat 44.08832 -default-lon 42.97577 -default-zoom 11 -default-layer "OpenStreetMap"`

* `-port int` — port HTTP (par défaut 8765). Pratique pour un usage local sans domaine.

  * Exemple : `chicha-isotope-map -port 8765`

* `-default-lat float` & `-default-lon float` — latitude et longitude initiales.

  * Exemple : `-default-lat 44.08832 -default-lon 42.97577`

* `-default-zoom int` — niveau de zoom initial (ville = 11–13).

  * Exemple : `-default-zoom 11`

* `-default-layer string` — couche de fond : `OpenStreetMap` ou `Google Satellite`.

  * Exemple : `-default-layer "Google Satellite"`

### Stockage (si nécessaire)

* `-db-type string` — pilote DB : `duckdb`, `genji`, `sqlite`, `pgx` (PostgreSQL). Par défaut : `sqlite`.

* `-db-path string` — chemin du fichier DB pour `duckdb`/`genji`/`sqlite` (par défaut : répertoire courant).

  * Exemple : `-db-type sqlite -db-path /var/lib/chicha/chicha.sqlite`

* `-db-host string`, `-db-port int` (par défaut 5432), `-db-name string`, `-db-user string`, `-db-pass string` — paramètres de connexion PostgreSQL pour `pgx`.

  * Exemple : `-db-type pgx -db-host 127.0.0.1 -db-port 5432 -db-name chicha_isotope_map -db-user postgres -db-pass secret`

* `-pg-ssl-mode string` — mode SSL PostgreSQL : `disable`, `allow`, `prefer` (par défaut), `require`, `verify-ca`, `verify-full`.

  * Exemple : `-pg-ssl-mode require`

### Utilitaires

* `-version` — affiche la version puis quitte.

### Exemples rapides

* **Local, sans HTTPS :**

  ```bash
  chicha-isotope-map \
    -port 8765 \
    -default-lat 44.08832 -default-lon 42.97577 \
    -default-zoom 11 \
    -default-layer "OpenStreetMap"
  ```

* **Serveur public avec HTTPS sur 80/443 :**

  ```bash
  sudo chicha-isotope-map \
    -domain maps.example.org \
    -default-lat 44.08832 -default-lon 42.97577 \
    -default-zoom 11 \
    -default-layer "OpenStreetMap"
  ```

* **Stockage fichier unique (SQLite) :**

  ```bash
  chicha-isotope-map \
    -db-type sqlite -db-path /var/lib/chicha-isotope-map.sqlite \
    -port 8765 \
    -default-lat 44.08832 -default-lon 42.97577 -default-zoom 11
  ```

* **Connexion PostgreSQL :**

  ```bash
  chicha-isotope-map \
    -db-type pgx \
    -db-host 127.0.0.1 -db-port 5432 \
    -db-name chicha_isotope_map -db-user postgres -db-pass secret \
    -pg-ssl-mode require \
    -default-lat 44.08832 -default-lon 42.97577 -default-zoom 11
  ```

---

## DuckDB

DuckDB est une base de données embarquée : un seul fichier, sans serveur, écrite en C++.
Le pilote Go dépend de **CGO** et de **librairies partagées**, donc on la compile ainsi :

```bash
CGO_ENABLED=1 go build -tags duckdb
```

Exécution :

```bash
./chicha-isotope-map -db-type duckdb
```

*(Sur macOS, vous pouvez simplement télécharger les [releases précompilées](https://github.com/matveynator/chicha-isotope-map/releases/tag/latest))*

---

## 🤝 Pourquoi héberger votre propre nœud ?

* **Simple :** votre communauté, votre carte.
* **Utile :** un historique local des niveaux de fond pour votre ville/zone/site — à vous de le conserver.
* **Bon pour le réseau :** plus de nœuds → plus de transparence et de résilience.

---

Chicha-Isotope-Map n’est pas seulement un logiciel — c’est une fenêtre sur un monde de microparticules, invisibles à l’œil nu mais évidentes pour un instrument.
Ce qui n’était jadis que supposition apparaît maintenant en points lumineux sur une carte, du vert rassurant au rouge inquiétant.

* **Que lit-il, et depuis quelles sources ?**

  * Fichiers `.kml`, `.kmz`, `.json`, `.rctrk`, `.csv`, `.gpx` (AtomFast, RadiaCode, Safecast et autres).
  * Tout est sauvegardé dans sa propre base, afin que, des années plus tard, vous puissiez dire : « Le 12 mars 2008, ce point affichait 4,1 µR/h. »

* **Quel est notre niveau de référence ?**

  * Le rayonnement naturel de fond dans une zone “propre” est d’environ 0,8–4 µR/h.
  * Au-delà, il s’agit d’une contamination d’origine humaine. Vous verrez comment les isotopes ont été dispersés par le vent, la circulation et les gens — comme des empreintes dans la neige fraîche.

---

### 📸 **Captures d’écran**

... À l’époque soviétique, on construisait une piscine en plein air dans le parc de Kislovodsk.
Il se peut qu’on ait utilisé du béton provenant d’une usine de Piatigorsk, qui traitait autrefois du minerai radioactif du mont Bechtau.
Les camions passaient, la poussière de leurs roues se déposait sur l’asphalte, laissant des marques invisibles.
Les années ont passé, mais ces traces brillent encore — souvenirs de ce qui fut.
La poussière soulevée sur le chantier s’est déposée dans le parc ; sur la carte, elle apparaît en jaune, comme des taches de feuilles d’automne.
Le reste du parc est propre, calme, et vert. <img src="https://repository-images.githubusercontent.com/870016860/11fd6abc-fe8b-4cd8-95c2-df1c631c8762">

---

### 📣 Comment annoncer votre nœud à la communauté

1. Montez un nœud sur un domaine avec HTTPS (`-e DOMAIN=...` + ouvrez `80` et `443`).
2. Ajoutez une capture d’écran et un bref descriptif (ville, zone, étendue de vos mesures).
3. Laissez une note dans la section Issues du dépôt GitHub du projet.

---

*Chicha Isotope Map* a été créé pour le **Laboratoire Dmitry Ignatenko de recherche sur la radioactivité**, et s’inspire de **Safecast** — cette remarquable communauté japonaise de citoyens-scientifiques qui a transformé la tragédie de Fukushima en un legs de connaissance scientifique.

En cherchant, en mesurant et en partageant ouvertement la vérité sur la radioactivité, vous rendez visible l’invisible.
Ce faisant, vous aidez le monde à éviter de répéter les douloureuses leçons de Tchernobyl et Fukushima.

Votre travail est une lumière — de science, de sécurité, et d’espoir.
Merci de transformer la radioactivité de fond, jadis objet de crainte, en objet de compréhension.

Merci pour votre courage : chercher, mesurer, partager — et surtout, être les premiers à vous avancer.

Nous exprimons aussi notre profonde gratitude aux communautés **AtomFast** et **Radiacode** pour leurs contributions précieuses — pour avoir mesuré, et pour avoir généreusement partagé leurs données avec le monde.

