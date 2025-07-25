# 🌌 Chicha‑Isotope‑Map — guide des sentiers cachés de la radiation

Chicha‑Isotope‑Map n’est pas qu’un programme : c’est une fenêtre sur le monde des microparticules, invisibles à l’œil nu mais détectables par les instruments. Autrefois, on ne pouvait que les deviner ; désormais, elles apparaissent en points lumineux sur la carte : du vert apaisant au rouge inquiétant.

* **Que lit-il et d’où vient l’info ?**

  * À partir de fichiers aux formats `.kml`, `.kmz`, `.json`, `.rctrk` (AtomFast, RadiaCode).
  * Il enregistre tout dans sa propre base afin que, des années plus tard, vous puissiez savoir précisément : « Le 12 mars 2024, ici, il y avait 4,1 µR/h ».

* **Sur quoi nous basons-nous ?**

  * Sur le bruit de fond naturel de la radioactivité : dans un endroit « propre », on est autour de 0,8–4 µR/h.
  * Tout ce qui dépasse est une pollution étrangère. Vous verrez comment les isotopes ont été dispersés par le vent, les voitures et les personnes, comme des traces sur une neige fraîchement tombée.

---

### 📸 **Captures d’écran**

… À l’époque soviétique, on construisait une piscine à ciel ouvert dans le parc de Kislovodsk. Il est possible qu’on ait utilisé du béton provenant d’une usine à Piatigorsk, où l’on retraitait autrefois du minerai radioactif de la montagne Bechtau. Des camions circulaient sur la route, et la poussière de leurs roues se déposait sur l’asphalte, y laissant des marques invisibles. Les années ont passé, mais ces traces brillent encore, comme des souvenirs du passé. La poussière envolée autour du chantier s’est déposée dans le parc — sur la carte elle est en jaune, tels des taches de feuilles d’automne. Le reste du parc est propre, calme, vert. <img src="https://repository-images.githubusercontent.com/870016860/11fd6abc-fe8b-4cd8-95c2-df1c631c8762">

---

### 📸 **Démo**

<a href="https://jutsa.ru" target="_blank"><img width="1156" height="844" alt="Chicha Isotope Map" src="https://github.com/user-attachments/assets/8d806377-671f-47a0-b918-f2a9afd4123e" /></a>

<a href="https://jutsa.ru" target="_blank">Vous pouvez voir le programme en fonctionnement en temps réel ici.</a>

---

## 🚀 Installation et nœud personnel en 5 minutes !

### 1. Démarrage rapide avec Docker

**Pourquoi Docker ?**
Docker empaquette le programme et son environnement dans un « conteneur ». Pas besoin de configurer manuellement bases de données et dépendances — vous lancez simplement l’image prête à l’emploi.

#### Lancement local (port 5000)

```bash
docker run -d \
  --name chicha-isotope-map \
  -e PORT=5000 \
  -p 5000:5000 \
  -v isotope-data:/var/lib/postgresql/data \
  matveynator/chicha-isotope-map:latest
```

Ouvrez votre navigateur sur [http://localhost:5000](http://localhost:5000) et admirez la carte.

#### Sur votre propre domaine avec HTTPS

1. Assurez-vous que `domain.com` pointe vers l’IP de votre serveur.
2. Les ports 80 et 443 doivent être libres.
3. Lancez la commande en **root** :

```bash
docker run -d \
  --name chicha-isotope-map \
  -e DOMAIN=domain.com \
  -p 80:80 -p 443:443 \
  -v isotope-data:/var/lib/postgresql/data \
  matveynator/chicha-isotope-map:latest
```

Le programme obtiendra et renouvellera automatiquement les certificats SSL.

#### Réglages supplémentaires de la carte

Toutes les options disponibles sont visibles via --help.
Vous pouvez définir le point de départ et le style :

```text
  -e DEFAULT_LAT=51.389      # latitude
  -e DEFAULT_LON=30.099      # longitude
  -e DEFAULT_ZOOM=11         # niveau de zoom
  -e DEFAULT_LAYER="OpenStreetMap" ou "Google Satellite"
```

#### Sauvegarde régulière (quotidienne)

Ajoutez dans `crontab -e` :

```bash
0 3 * * * docker exec chicha-isotope-map pg_dump -U chicha_isotope_map chicha_isotope_map | gzip > /backup/chicha_isotope_map_$(date +\%F).sql.gz
```

#### Restauration depuis une archive

```bash
docker exec -it chicha-isotope-map psql -U postgres -c "DROP DATABASE IF EXISTS chicha_isotope_map; CREATE DATABASE chicha_isotope_map OWNER chicha_isotope_map;"

zcat /backup/chicha_isotope_map_2025-07-24.sql.gz | docker exec -i chicha-isotope-map psql -U chicha_isotope_map chicha_isotope_map
```

---

### 2. Installation sans Docker

Si vous n’aimez pas les conteneurs, téléchargez le binaire prêt à l’emploi et lancez-le en une seconde, c’est encore plus simple !

> Exécutez les commandes en **root** (`sudo -i` ou `sudo ...`).

* **Linux x64** :

```
curl -L https://github.com/matveynator/chicha-isotope-map/releases/download/latest/chicha-isotope-map_linux_amd64 > /usr/local/bin/chicha-isotope-map && chmod +x /usr/local/bin/chicha-isotope-map && chicha-isotope-map
```

* **macOS Intel** :

```
curl -L https://github.com/matveynator/chicha-isotope-map/releases/download/latest/chicha-isotope-map_darwin_amd64 > /usr/local/bin/chicha-isotope-map && chmod +x /usr/local/bin/chicha-isotope-map && chicha-isotope-map
```

* **macOS Apple Silicon** :

```
curl -L https://github.com/matveynator/chicha-isotope-map/releases/download/latest/chicha-isotope-map_darwin_arm64 > /usr/local/bin/chicha-isotope-map && chmod +x /usr/local/bin/chicha-isotope-map && chicha-isotope-map
```

Les autres plateformes — Windows / ARM / BSD — sont disponibles sur la page des versions : [https://github.com/matveynator/chicha-isotope-map/releases/tag/latest](https://github.com/matveynator/chicha-isotope-map/releases/tag/latest)

Après le lancement, le programme écoute par défaut sur le port 8765. Ouvrez [http://localhost:8765](http://localhost:8765).

---

## 🤝 Pourquoi avoir votre propre nœud ?

* **Indépendance** : vos données restent chez vous, sans dépendre du réseau de quelqu’un d’autre.
* **Résilience du réseau** : plus il y a de nœuds, plus il est difficile de le compromettre.
* **Mémoire locale du fond** : conservez la carte de la radioactivité de votre région pour les années à venir.

Chaque serveur que vous déployez est un phare d’information supplémentaire. Merci de rendre le monde plus transparent !

La carte des isotopes Chicha a été créée pour le Laboratoire de recherches radiologiques de Dmitri Ignatenko et s’inspire de la communauté japonaise Safecast — un groupe de citoyens-scientifiques qui ont transformé une tragédie en savoir. En cherchant, en mesurant et en diffusant la vérité sur la radioactivité, vous rendez l’invisible visible, aidant le monde à ne pas répéter Tchernobyl et Fukushima. Votre travail est la lumière de la science, de la sécurité et de l’espoir. Merci de transformer le rayonnement de fond d’un motif de peur en source de compréhension, de chercher, mesurer, partager — et d’avancer les premiers avec courage.
