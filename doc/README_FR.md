- [🇬🇧 English](/README.md)
- [🇫🇷 Français](/doc/README_FR.md)
- [🇯🇵 日本語](/doc/README_JP.md)
- [🇷🇺 Русский](/doc/README_RU.md)

# ☢️ Chicha‑Isotope‑Map — carte personnelle de la radioactivité

---

La page **DeepWiki** de *Chicha Isotope Map* a été réalisée avec bienveillance par [Rob Ouden](https://github.com/robouden) du projet **Safecast**, à qui nous exprimons notre profonde gratitude.  Elle révèle la structure intime du programme, permettant aux développeurs de comprendre ses fondations, de suivre sa logique et de continuer à l’améliorer.  Grâce à DeepWiki, ce code n’est pas figé : il devient un projet vivant, ouvert à l’évolution et à l’enrichissement collectif.  

👉 [DeepWiki : Chicha Isotope Map](https://deepwiki.com/matveynator/chicha-isotope-map)

---

## 🚀 Installation et nœud auto‑hébergé en 2 commandes

Rien de superflu. L’image embarque déjà la base (PostgreSQL). Copiez la commande, lancez‑la — et c’est parti.

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

Après l’émission du certificat, rendez‑vous sur : [https://example.org](https://example.org)

---

### ⚙️ Réglages via variables d’environnement (le strict nécessaire)

* `DOMAIN` — active le HTTPS sur 80/443 et les certificats automatiques Let’s Encrypt (pour un nœud public).
* `DEFAULT_LAT`, `DEFAULT_LON` — coordonnées initiales de la carte.
* `DEFAULT_ZOOM` — niveau de zoom initial (11 — pratique pour une vue urbaine).
* `DEFAULT_LAYER` — `OpenStreetMap` ou `Google Satellite`.
* `PORT` — port interne de l’appli (8765 par défaut ; en général on ne le change pas).

> Astuce : conservez les données sur le volume `-v chicha-data:/var/lib/postgresql/data` afin qu’elles survivent aux mises à jour du conteneur.

---

### 💾 Sauvegarde et restauration (simple)

**Sauvegarde quotidienne (03:00) :**

```bash
0 3 * * * docker exec chicha-isotope-map pg_dump -U chicha_isotope_map chicha_isotope_map | gzip > /backup/chicha_isotope_map_$(date +\%F).sql.gz
```

**Restauration depuis l’archive :**

```bash
docker exec -it chicha-isotope-map psql -U postgres -c "DROP DATABASE IF EXISTS chicha_isotope_map; CREATE DATABASE chicha_isotope_map OWNER chicha_isotope_map;"

zcat /backup/chicha_isotope_map_2025-07-24.sql.gz | docker exec -i chicha-isotope-map psql -U chicha_isotope_map chicha_isotope_map
```

---

## ⬇️ Télécharger les exécutables prêts à l’emploi (sans Docker)

Téléchargez le binaire pour votre système, rendez‑le exécutable et lancez‑le.

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

Autres plateformes (Windows / ARM / BSD) — sur la page des releases :
[https://github.com/matveynator/chicha-isotope-map/releases/tag/latest](https://github.com/matveynator/chicha-isotope-map/releases/tag/latest)

---

## 🖥 Lancement classique (sans Docker) : options et exemples

Si vous lancez directement le binaire `chicha-isotope-map`, voici l’essentiel. D’abord le plus important ; plus bas, des paramètres supplémentaires.

### À savoir

* `-domain string` — active le HTTPS et l’écoute sur les ports 80 et 443 avec certificats Let’s Encrypt automatiques. Le domaine doit pointer vers votre serveur et les ports 80/443 doivent être libres.

  * Exemple : `sudo chicha-isotope-map -domain maps.example.org -default-lat 44.08832 -default-lon 42.97577 -default-zoom 11 -default-layer "OpenStreetMap"`

* `-port int` — port du serveur HTTP (8765 par défaut). Pratique pour un lancement local sans domaine.

  * Exemple : `chicha-isotope-map -port 8765`

* `-default-lat float` et `-default-lon float` — latitude et longitude initiales de la carte.

  * Exemple : `-default-lat 44.08832 -default-lon 42.97577`

* `-default-zoom int` — niveau de zoom initial (en ville : 11–13).

  * Exemple : `-default-zoom 11`

* `-default-layer string` — couche de base : `OpenStreetMap` ou `Google Satellite`.

  * Exemple : `-default-layer "Google Satellite"`

### Stockage (si besoin)

* `-db-type string` — pilote de BD : `duckdb`, `genji`, `sqlite`, `pgx` (PostgreSQL). Par défaut : `genji`.

* `-db-path string` — chemin du fichier BD pour `duckdb`/`genji`/`sqlite` (si omis — répertoire courant).

  * Exemple : `-db-type sqlite -db-path /var/lib/chicha/chicha.sqlite`

* `-db-host string`, `-db-port int` (5432 par défaut), `-db-name string`, `-db-user string`, `-db-pass string` — paramètres de connexion PostgreSQL pour `pgx`.

  * Exemple : `-db-type pgx -db-host 127.0.0.1 -db-port 5432 -db-name chicha_isotope_map -db-user postgres -db-pass secret`

* `-pg-ssl-mode string` — mode SSL pour PostgreSQL : `disable`, `allow`, `prefer` (par défaut), `require`, `verify-ca`, `verify-full`.

  * Exemple : `-pg-ssl-mode require`

### Service

* `-version` — afficher la version et quitter.

### Exemples rapides

* **En local, sans HTTPS :**

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

* **Stockage dans un seul fichier (SQLite) :**

  ```bash
  chicha-isotope-map \
    -db-type sqlite -db-path /var/lib/chicha-isotope-map.sqlite \
    -port 8765 \
    -default-lat 44.08832 -default-lon 42.97577 -default-zoom 11
  ```

* **Connexion à PostgreSQL :**

  ```bash
  chicha-isotope-map \
    -db-type pgx \
    -db-host 127.0.0.1 -db-port 5432 \
    -db-name chicha_isotope_map -db-user postgres -db-pass secret \
    -pg-ssl-mode require \
    -default-lat 44.08832 -default-lon 42.97577 -default-zoom 11
  ```

---

## 🤝 Pourquoi héberger votre propre nœud ?

* **Simple :** vous avez votre communauté — vous avez votre carte.
* **Utile :** l’historique local du fond de votre ville/quartier/site reste chez vous, pour toujours.
* **Important pour le réseau :** plus de nœuds → plus de transparence et de résilience.

---

Chicha‑Isotope‑Map n’est pas qu’un logiciel, c’est une fenêtre sur le monde des microparticules, invisibles à l’œil nu mais bien réelles pour l’instrument. Hier, on ne pouvait que les deviner ; aujourd’hui, elles s’affichent en points lumineux sur la carte, du vert apaisé au rouge inquiétant.

* **Que lit‑elle et d’où ?**

  * À partir de fichiers aux formats `.kml`, `.kmz`, `.json`, `.rctrk`, `.csv`, `.gpx` formats (AtomFast, RadiaCode, Safecast еtc.).
  * Elle enregistre tout dans sa propre base pour qu’avec le temps vous puissiez savoir précisément : « Le 12 mars 2008, ici, il y avait 4,1 µR/h. »

* **Sur quoi s’appuie‑t‑on ?**

  * Sur le fond naturel de la radioactivité : dans un endroit « propre » — environ 0,8–4 µR/h.
  * Tout ce qui dépasse indique une contamination étrangère. Vous verrez comment les isotopes se dispersent avec le vent, les voitures et les gens, comme des traces sur une neige fraîche.

---

### 📸 **Captures d’écran**

... À l’époque soviétique, on construisait une piscine à ciel ouvert dans le parc de Kislovodsk. Peut‑être a‑t‑on utilisé du béton venant de l’usine de Piatigorsk, où l’on traitait autrefois du minerai radioactif extrait du mont Bechtau. Sur la route, les camions passaient ; la poussière de leurs roues se déposait sur l’asphalte, laissant des marques invisibles. Les années ont passé, mais ces traces brillent encore, comme des souvenirs du passé. La poussière dispersée autour du chantier s’est posée dans le parc — sur la carte elle apparaît en jaune, comme des taches de feuilles d’automne. Tout le reste du parc est propre, calme, vert. <img src="https://repository-images.githubusercontent.com/870016860/11fd6abc-fe8b-4cd8-95c2-df1c631c8762">

---

### 📸 **Démo**

<a href="https://pelora.org" target="_blank"><img width="1156" height="844" alt="Chicha Isotope Map" src="https://github.com/user-attachments/assets/8d806377-671f-47a0-b918-f2a9afd4123e" /></a>

<a href="https://pelora.org" target="_blank">Vous pouvez voir le programme à l’œuvre en temps réel ici.</a>

---

### 📣 Annoncer votre nœud à la communauté

1. Déployez un nœud sur un domaine avec HTTPS (`-e DOMAIN=...` + redirection des ports `80` et `443`).
2. Ajoutez une capture d’écran et une courte description (ville, quartier, zone couverte par vos mesures).
3. Signalez‑vous dans une Issue du dépôt du projet.

---

La carte *Chicha Isotope Map* a été créée pour le **Laboratoire de recherche sur la radioprotection Dmitry Ignatenko**, avec l’inspiration profonde du projet japonais **Safecast** — cette communauté de scientifiques citoyens qui a su transformer la tragédie de Fukushima en savoir scientifique.

En cherchant, en mesurant et en partageant avec courage la vérité sur la radiation, vous rendez visible l’invisible, et contribuez ainsi à éviter que le monde ne répète les erreurs de Tchernobyl et de Fukushima.

Votre travail est une lumière — celle de la science, de la sécurité, et de l’espérance.

Merci d’avoir fait de la radioactivité naturelle non plus une source de crainte, mais un objet de compréhension.

Merci de chercher, de mesurer, de partager, et surtout — d’oser être les premiers.

Nous adressons aussi notre gratitude aux communautés **AtomFast** et **Radiacode** pour leurs précieuses mesures et leur générosité dans le partage des données.

