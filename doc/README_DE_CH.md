[![Letzter stabiler Build](https://github.com/matveynator/chicha-isotope-map/actions/workflows/release.yml/badge.svg)](https://github.com/matveynator/chicha-isotope-map/actions/workflows/release.yml)

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

# ☢️ Wältwyyti Strahlungskarte
D Karte isch gmacht, dass jedi Persone ohni grosses Vorwissen grad gseht, ob Strahlig d Huus, d Fäld, d Wälder oder d Waasserstellig i dr Nöchi gföhrdet. Gsundi Orte ligged bi 2–3 µR/h; d dunklere Flecke chöme fasch immer vo menschliche Aktivität. D Karte zeigt, wie d Uranmine i Tschechie, Russland, Kasachstan und Mongolei langi Spure hingerlah hend; wie Fukushima als schwarz-rot „Tumor“ an dr japanische Chüscht ussticht; wie Tschernobyl und s Bryansk-Gbiet s Land präge; wie Radon-Aderä i Frankriich, Tschechie und bi dr Kaukasische Mineralquälle s Risiko erhöhe. Uslaug vo Uran und Rare Earths hinerlat löslichä Salze, wo i d Grundwaasser göh und denn i üses Trinkwasser und s Ässe cho. Wenn die Karte au nume ein Mensch oder Tier schützt, het sich s Boue glonnt.

Live-Demo: [https://pelora.org/](https://pelora.org/) — dis Nödli luegt glich uus.

👉 [Eini Download-Site](https://github.com/matveynator/chicha-isotope-map/releases) (alli Plattformä, aktuellsti Builds)

👉 [DeepWiki: Chicha Isotope Map](https://deepwiki.com/matveynator/chicha-isotope-map)

---

### 📸 Bsügg
<a href="https://pelora.org" target="_blank"><img width="800" alt="pelora.org chicha-isotope-map bsügg" src="https://github.com/user-attachments/assets/be706959-a2d5-4949-9378-811f4022aa98" /></a>

---

## 🧭 Was het s dinne
- Live-Karte mit Messige vo vilä Detektore; nimm dr Layer, wo der passt.
- Lade dini Tracks uuf; frischi Punkt chömed grad rund um s Gbiet, wo du a luegsch.
- Import via URL oder Datei, Export als Archiv.
- Läuft als einzelne Nöd oder im Netzwerk: meh Nöd = meh Transparenz.

S Projekt wachst dank em Support vo **Safecast** und dr Community: viu gueti Idee chömed vo **Rob Oudendijk** und vo Lüüt umeg dum, wo offeni Dosimetrie mache (merci, Greenpeace und anderi Umwältteams).

---

## 🚀 Schnäu Start (für Iistieger)
Am schnällste: s Binary hole. Kei Docker, kei Datenbank, kei Extra-Wärchzüg — abelade, starte, fertig.

### Option 1. Binary (empfohle)
1) Öffne d [Release-Site](https://github.com/matveynator/chicha-isotope-map/releases) und lad d Version für dis System ab.
2) Mach s ausführbar und starte:
```bash
chmod +x ./chicha-isotope-map
./chicha-isotope-map
```
3) Mach [http://localhost:8765](http://localhost:8765) uuf — d Karte isch scho parat.

Nützligi Stellschraube:
- `-port 8765` — lokali Port.
- `-domain maps.example.org` — HTTPS mit Let’s Encrypt (brucht 80/443).
- `-default-lat` / `-default-lon` / `-default-zoom` / `-default-layer` — Start-Ansicht.
- Speicher: `-db-type sqlite|duckdb|chai|clickhouse|pgx`, `-db-path` für Datei-DBs, `-db-conn` für Netzwerk.

### Option 2. Öffentleche Nöd mit Domain
1) Starte s Binary mit diner Domain:
```bash
./chicha-isotope-map -domain example.org
```
2) Lueg, dass d Ports 80/443 frei sind für Let’s Encrypt. Nachher isch d Site uf [https://example.org](https://example.org).

### Option 3. Docker (alles verpackt)
1) Docker installiere (Desktop oder CLI).
2) Suech **matveynator/chicha-isotope-map** uf Docker Hub und druck **Run** (oder bruch dä Befehl):
```bash
docker run -d -p 8765:8765 --name chicha-isotope-map matveynator/chicha-isotope-map:latest
```
3) Öffne [http://localhost:8765](http://localhost:8765) — fertig.

---

## 📥 Date importiere
- Uuf dr Kartä-Site dr grüne **Upload**-Button drucke und dini Tracks lade (`.kml`, `.kmz`, `.json`, `.rctrk`, `.csv`, `.gpx`, bGeigie Nano/Zen `$BNRDD`, AtomFast-Export, RadiaCode, Safecast, usw.).
- Wotsch es Spiegel vo pelora.org? Einisch `chicha-isotope-map -import-tgz-url https://pelora.org/api/json/weekly.tgz` laufe lah — läd s wöchentlechi Archiv, füllt d DB und hört uf, dass dr nöchsti Start grad fertig isch.
- Lieber s Archiv vorgängig hole? Nimm [https://pelora.org/api/json/weekly.tgz](https://pelora.org/api/json/weekly.tgz), starte mit `-import-tgz-path /pfad/zu/weekly.tgz` und laufe mit diner lokali Kopie.

### 🗺️ Erschte Start mit live Date i enem Befehl
Uf ere frische Maschine langt dä Befehl:
```bash
chicha-isotope-map -import-tgz-url https://pelora.org/api/json/weekly.tgz
```
Nach em Import nomol normal starte (oder dr gleiche Befehl i systemd) — d Karte isch uf [http://localhost:8765](http://localhost:8765) grad voll mit realä Messige.

### 🛢️ Datenbank-Wahl fürs Import und dr Alltag
- **PostgreSQL (`pgx`)** — am schnällste und guet, wenn meh Lüüt schribe. Bispiel: `chicha-isotope-map -db-type pgx -db-conn postgres://USER:PASS@HOST:PORT/DATABASE?sslmode=allow -import-tgz-url https://pelora.org/api/json/weekly.tgz`
- **DuckDB / SQLite / Chai** — eifachi Datei-Lösung für ein Benutzer. Glichziitig schribe cha konfliktiere, drum am beschte für persöndligi Karte. Bispiel: `chicha-isotope-map -db-type duckdb -import-tgz-url https://pelora.org/api/json/weekly.tgz`

## 📤 Exportiere
- Einzlane Track: `/api/track/{trackID}.json` (au s alti `.cim`).
- Geplanti Archive: `/api/json/weekly.tgz` (oder `/daily.tgz`, `/monthly.tgz`, `/yearly.tgz`). Inehalt: pro Track es JSON.

---

## 🧠 Erweitereti Optione
- Datenbanke: standardmässig SQLite i-baut; wächsle uf DuckDB, Chai, ClickHouse oder PostgreSQL (`pgx`).
- Import: via URL oder Datei, au als Archiv.
- Export: JSON-Archive, einzlane Track, `.cim` kompatibel.
- Uuslueg: Start-Koordinäte und Layer (`-default-*`).

---

## 🤝 Warum din eigenä Nöd und chli Gschicht
- Mir wend, dass jede ohni Spezialwüsse cha gseh, ob Strahlig dort isch, wo er wohnt, pflanzt oder Waasser holt.
- Meh Nöd gäbed es zuverlässigers Gsammtbild und chöi Verschmutzig besser entdecke.

Chicha‑Isotope‑Map isch inspiriert vo de Schrit vom **Dmitry Ignatenko** i dr Fäldforschig und stark beeinflusst vo **Rob Oudendijk** und **Safecast**. Offeni Date vo de Communities AtomFast und Radiacode mache d Karte nützli. Wenn d Karte e Läbe rette cha, isch si nöd umesüsch gmacht worde.
