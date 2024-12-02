### 🌌 **chicha-isotope-map** — explorateur des chemins invisibles de la radiation.

> **« Qui peut voir l'invisible ? Personne ? Ce programme le peut. Il traque les traces radioactives, comme un chaman qui lit les cendres, et les fait apparaître à l'écran — vives, lumineuses, vibrantes. »**

---

## 📖 **À propos du projet**

**Chicha-Isotope-Map** révèle le monde invisible des particules radioactives. Sous vos pieds, les isotopes laissent des traces lorsqu'ils voyagent, portés par le vent, les véhicules ou les passants. Ce programme les visualise sur une carte, colorant chaque trace — du vert (sécurité) au rouge (danger).

Il lit des données issues de formats comme `.kml`, `.kmz`, `.json`, et `.rctrk` (AtomFast et RadiaCode) et les stocke dans une base de données. Des années plus tard, vous pourrez consulter l'évolution des niveaux de radiation.

---

### 🌍 **Inspiré par la nature**

Le programme utilise le **fond naturel de radiation** comme référence. Dans des zones vierges, la radiation normale se situe autour de **1 à 4 µR/h**. Tout ce qui dépasse ce seuil est identifié comme **contamination radioactive**. Chicha-Isotope-Map suit ces anomalies, transformant des empreintes invisibles en avertissements visibles.

---

### 📸 **Démo en direct**

<a href="https://jutsa.ru" target="_blank">Découvrez le programme en action ici.</a>

---

### 📸 **Exemple visuel**

À l'époque soviétique, une piscine extérieure a été construite dans le parc de Kislovodsk. Le béton aurait pu provenir d'une usine à Pyatigorsk, où du minerai radioactif de la montagne Beshtau était traité. Les camions transportaient les matériaux, laissant une poussière invisible sur les routes. Des décennies plus tard, ces traces apparaissent encore sur la carte comme des marques jaunes, semblables à des feuilles d'automne. Le reste du parc reste propre, paisible et verdoyant.  
<img src="https://repository-images.githubusercontent.com/870016860/11fd6abc-fe8b-4cd8-95c2-df1c631c8762">

---

## 📥 **Téléchargement et démarrage** 📥

Installez sur Linux 64-bit x86 :  
```bash
sudo curl https://files.zabiyaka.net/chicha-isotope-map/latest/no-gui/linux/amd64/chicha-isotope-map > /usr/local/bin/chicha-isotope-map; sudo chmod +x /usr/local/bin/chicha-isotope-map; chicha-isotope-map -v;
```

Choisissez votre plateforme pour commencer à explorer les traces des isotopes :

| Plateforme | Lien de téléchargement                                                                                 |
|------------|--------------------------------------------------------------------------------------------------------|
| AIX        | [Télécharger pour AIX](http://files.zabiyaka.net/chicha-isotope-map/latest/no-gui/aix/)                 |
| Android    | [Télécharger pour Android](http://files.zabiyaka.net/chicha-isotope-map/latest/no-gui/android/)          |
| Dragonfly  | [Télécharger pour Dragonfly](http://files.zabiyaka.net/chicha-isotope-map/latest/no-gui/dragonfly/)      |
| FreeBSD    | [Télécharger pour FreeBSD](http://files.zabiyaka.net/chicha-isotope-map/latest/no-gui/freebsd/)          |
| Illumos    | [Télécharger pour Illumos](http://files.zabiyaka.net/chicha-isotope-map/latest/no-gui/illumos/)          |
| JavaScript | [Télécharger pour JavaScript](http://files.zabiyaka.net/chicha-isotope-map/latest/no-gui/js/)            |
| Linux      | [Télécharger pour Linux](http://files.zabiyaka.net/chicha-isotope-map/latest/no-gui/linux/)              |
| macOS      | [Télécharger pour macOS](http://files.zabiyaka.net/chicha-isotope-map/latest/no-gui/mac/)                |
| NetBSD     | [Télécharger pour NetBSD](http://files.zabiyaka.net/chicha-isotope-map/latest/no-gui/netbsd/)            |
| OpenBSD    | [Télécharger pour OpenBSD](http://files.zabiyaka.net/chicha-isotope-map/latest/no-gui/openbsd/)          |
| Plan9      | [Télécharger pour Plan9](http://files.zabiyaka.net/chicha-isotope-map/latest/no-gui/plan9/)              |
| Solaris    | [Télécharger pour Solaris](http://files.zabiyaka.net/chicha-isotope-map/latest/no-gui/solaris/)          |
| Windows    | [Télécharger pour Windows](http://files.zabiyaka.net/chicha-isotope-map/latest/no-gui/windows/)          |

---

## 🛠 **Comment utiliser ?**

Exécutez le programme avec les paramètres par défaut :  
```bash
chicha-isotope-map
```

Ou personnalisez avec des options supplémentaires :  
```bash
chicha-isotope-map --port=8765 --db-type=pgx --db-host=localhost --db-port=5432 --db-user=postgres --db-pass=yourpassword --db-name=isotope_db --pg-ssl-mode=prefer
```

### Exemple PostgreSQL (`pgx`) :
```bash
chicha-isotope-map --port=8765 --db-type=pgx --db-host=localhost --db-port=5432 --db-user=postgres --db-pass=mon_mot_de_passe --db-name=radiation_data --pg-ssl-mode=require
```

Cette configuration connecte le programme à une base de données PostgreSQL nommée `radiation_data`. Remplacez `mon_mot_de_passe` par le mot de passe de votre base de données. Adaptez le nom de l'hôte, le port ou le nom de la base selon vos besoins.

---

### Interface Web :

1. Ouvrez [http://localhost:8765](http://localhost:8765) dans votre navigateur.  
2. Utilisez le bouton **Upload** pour ajouter vos fichiers de données.  
3. Explorez la carte : survolez les marqueurs pour voir les niveaux de radiation, les horodatages et les emplacements.

---

## ☢️ **Pourquoi est-ce important ?**

La radiation est invisible mais dangereuse. Elle ne reste pas en surface : elle pénètre dans le sol, l'eau et les plantes, s'accumulant avec le temps. Ce programme vous aide à voir où la contamination s'est propagée, rendant l'invisible visible et vous donnant les moyens d'agir.

---

> **« Si les isotopes pouvaient raconter leur histoire, ils n'auraient pas besoin de ce programme. Mais comme ils ne peuvent pas, Chicha-Isotope-Map le fait pour eux. »**
