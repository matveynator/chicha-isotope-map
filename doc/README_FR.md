“Chicha Isotope Map” a été développée spécialement pour le Laboratoire d'Étude de la Radiation de Dmitry Ignatenko. Merci de rendre visible l’invisible, là où le rayonnement de fond n’est pas une peur, mais une source de savoir. Et merci de chercher, mesurer, partager, et d’oser avancer les premiers.

### 🌌 **chicha-isotope-map** — explorateur des chemins cachés de la radiation

> **«Voir l’invisible.» Ce programme visualise les traces radioactives et transforme les chemins invisibles en cartes vibrantes.**

---

## 📖 **À propos du projet**

**Chicha-Isotope-Map** révèle le monde invisible des particules radioactives. Sous vos pieds, les isotopes laissent des traces — transportées par le vent, les véhicules ou les personnes. Ce programme les affiche sur une carte, du vert (sûr) au rouge (dangereux).

Il lit les fichiers aux formats `.kml`, `.kmz`, `.json` et `.rctrk` (AtomFast et RadiaCode), et stocke les données dans une base. Des années plus tard, vous pourrez voir comment les niveaux de radiation ont évolué.

---

### 🌍 **Inspiré par la nature**

Le programme utilise le **rayonnement naturel de fond** comme référence. Dans les zones non perturbées, il est généralement de **1 à 4 µR/h**. Toute valeur supérieure est considérée comme **contamination radioactive**. Chicha-Isotope-Map suit ces anomalies et rend les empreintes invisibles visibles.

---

### 📸 **Démo en direct**

<a href="https://jutsa.ru" target="_blank">Voir le programme en action ici.</a>

---

### 📸 **Exemple visuel**

À l’époque soviétique, une piscine en plein air a été construite dans le parc de Kislovodsk. Le béton provenait peut-être d’une usine de Piatigorsk qui traitait du minerai radioactif du mont Beshtau. Les camions ont transporté les matériaux en laissant une poussière invisible sur les routes. Des décennies plus tard, ces traces apparaissent encore sur la carte, sous forme de taches jaunes — comme des feuilles d’automne. Le reste du parc est propre, paisible et vert. <img src="https://repository-images.githubusercontent.com/870016860/11fd6abc-fe8b-4cd8-95c2-df1c631c8762">

---

## 📥 **Télécharger et commencer** 📥

### Linux 64-bit amd64 :

Remarque : installez en tant que ROOT.

```bash
curl -L https://github.com/matveynator/chicha-isotope-map/releases/download/latest/chicha-isotope-map_linux_amd64 > /usr/local/bin/chicha-isotope-map; chmod +x /usr/local/bin/chicha-isotope-map; chicha-isotope-map --version;
```

### Mac OS X Intel :

Remarque : installez en tant que ROOT.

```bash
curl -L https://github.com/matveynator/chicha-isotope-map/releases/download/latest/chicha-isotope-map_darwin_amd64 > /usr/local/bin/chicha-isotope-map; chmod +x /usr/local/bin/chicha-isotope-map; chicha-isotope-map --version;
```

### Mac OS X Apple Silicon :

Remarque : installez en tant que ROOT.

```bash
curl -L https://github.com/matveynator/chicha-isotope-map/releases/download/latest/chicha-isotope-map_darwin_amd64 > /usr/local/bin/chicha-isotope-map; chmod +x /usr/local/bin/chicha-isotope-map; chicha-isotope-map --version;
```

[Téléchargement pour d'autres plateformes : Linux, macOS, Windows, FreeBSD, OpenBSD, NetBSD](https://github.com/matveynator/chicha-isotope-map/releases/tag/latest)

---

## 🛠 **Comment l’utiliser ?**

Lancer avec les paramètres par défaut :

```bash
chicha-isotope-map
```

Lancer avec des options personnalisées :

```bash
chicha-isotope-map --port=8765 --db-type=pgx --db-host=localhost --db-port=5432 --db-user=postgres --db-pass=yourpassword --db-name=isotope_db --pg-ssl-mode=prefer
```

### Exemple PostgreSQL (pilote `pgx`) :

```bash
chicha-isotope-map --port=8765 --db-type=pgx --db-host=localhost --db-port=5432 --db-user=postgres --db-pass=my_secure_password --db-name=radiation_data --pg-ssl-mode=require
```

Cette configuration se connecte à une base de données PostgreSQL nommée `radiation_data` sur la machine locale. Remplacez `my_secure_password` par votre mot de passe. Ajustez les autres paramètres si nécessaire.

---

### Interface web

1. Ouvrez [http://localhost:8765](http://localhost:8765) dans votre navigateur.
2. Cliquez sur **Upload** pour ajouter vos fichiers de données.
3. Explorez la carte : survolez les marqueurs pour voir les niveaux de radiation, les dates et les lieux.

---

## ☢️ **Pourquoi c’est important**

La radiation est invisible mais dangereuse. Elle ne reste pas en place — elle s’infiltre dans le sol, l’eau, les plantes, et s’accumule. Ce programme vous aide à voir où la contamination s’est propagée, pour mieux comprendre et agir.

---

> **« Si les isotopes pouvaient raconter leurs histoires, ce programme serait inutile. Mais comme ils ne le peuvent pas, c’est Chicha-Isotope-Map qui parle pour eux. »**
