[![Ultima build stabile](https://github.com/matveynator/chicha-isotope-map/actions/workflows/release.yml/badge.svg)](https://github.com/matveynator/chicha-isotope-map/actions/workflows/release.yml)

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

# ☢️ Mappa mondiale della radiazione
Questa mappa è pensata perché chiunque, senza preparazione, possa capire subito se la radiazione minaccia case, campi, foreste o punti d’acqua vicini. I luoghi puliti stanno intorno a 2–3 µR/h; le macchie più scure arrivano quasi sempre da attività umana. La mappa mostra come le miniere d’uranio in Cecoslovacchia, Russia, Kazakistan e Mongolia abbiano lasciato lunghe tracce; come Fukushima risalti come un “tumore” nero e rosso sulla costa giapponese; come Černobyl' e la regione di Bryansk segnino il paesaggio; come le vene di radon in Francia, Cecoslovacchia e nelle Acque Minerali del Caucaso aumentino i rischi. Il lisciviamento dell’uranio e delle terre rare lascia sali solubili che penetrano nelle falde e poi nella nostra acqua e nel cibo. Se questa mappa protegge anche una sola persona o animale, è valsa la pena costruirla.

Demo online: [https://pelora.org/](https://pelora.org/) — il tuo nodo avrà lo stesso aspetto.

👉 [Pagina unica di download](https://github.com/matveynator/chicha-isotope-map/releases) (tutte le piattaforme, ultime versioni)

👉 [DeepWiki: Chicha Isotope Map](https://deepwiki.com/matveynator/chicha-isotope-map)

---

### 📸 Esempio
<a href="https://pelora.org" target="_blank"><img width="800" alt="pelora.org chicha-isotope-map esempio" src="https://github.com/user-attachments/assets/be706959-a2d5-4949-9378-811f4022aa98" /></a>

---

## 🧭 Cosa c’è dentro
- Mappa dal vivo con misure da molti rilevatori; scegli il layer che preferisci.
- Carica le tue tracce; i punti nuovi appaiono subito intorno all’area visualizzata.
- Importa via URL o file, esporta come archivio.
- Funziona come nodo singolo o in rete: più nodi ⇒ più trasparenza.

Il progetto cresce grazie al supporto di **Safecast** e della comunità: molte idee preziose arrivano da **Rob Oudendijk** e dagli appassionati di dosimetria aperta nel mondo (grazie, Greenpeace e altre squadre ambientali).

---

## 🚀 Avvio rapido (per chi inizia)
Percorso più veloce: scarica il binario. Niente Docker, niente database o strumenti extra — scarichi, avvii, è pronto.

### Opzione 1. Binario (consigliata)
1) Apri la [pagina delle release](https://github.com/matveynator/chicha-isotope-map/releases) e scarica la build per il tuo sistema.
2) Rendi il file eseguibile e avvia:
```bash
chmod +x ./chicha-isotope-map
./chicha-isotope-map
```
3) Apri [http://localhost:8765](http://localhost:8765) — la mappa è già online.

Opzioni utili:
- `-port 8765` — porta locale.
- `-domain maps.example.org` — HTTPS con Let’s Encrypt (richiede 80/443).
- `-default-lat` / `-default-lon` / `-default-zoom` / `-default-layer` — vista iniziale della mappa.
- Storage: `-db-type sqlite|duckdb|chai|clickhouse|pgx`, `-db-path` per DB su file, `-db-conn` per quelli di rete.

### Opzione 2. Nodo pubblico con dominio
1) Avvia il binario con il tuo dominio:
```bash
./chicha-isotope-map -domain example.org
```
2) Lascia aperte le porte 80/443 per Let’s Encrypt. Dopo l’emissione il sito sarà su [https://example.org](https://example.org).

### Opzione 3. Docker (tutto impacchettato)
1) Installa Docker (Desktop o CLI).
2) Cerca **matveynator/chicha-isotope-map** su Docker Hub e clicca **Run** (o esegui un comando):
```bash
docker run -d -p 8765:8765 --name chicha-isotope-map matveynator/chicha-isotope-map:latest
```
3) Apri [http://localhost:8765](http://localhost:8765) — fatto.

---

## 📥 Importa dati
- Nella pagina della mappa clicca il pulsante verde **Upload** e carica le tue tracce (`.kml`, `.kmz`, `.json`, `.rctrk`, `.csv`, `.gpx`, log bGeigie Nano/Zen `$BNRDD`, export AtomFast, RadiaCode, Safecast, ecc.).
- Vuoi uno specchio di pelora.org? Esegui una volta `chicha-isotope-map -import-tgz-url https://pelora.org/api/json/weekly.tgz` — scarica l’archivio settimanale, riempie il database ed esce così il prossimo avvio parte già con i dati.
- Preferisci scaricare l’archivio prima? Prendi [https://pelora.org/api/json/weekly.tgz](https://pelora.org/api/json/weekly.tgz), lancia con `-import-tgz-path /percorso/a/weekly.tgz` e avvia con la tua copia locale.

### 🗺️ Primo avvio con dati reali in un comando
Su un sistema pulito basta questo:
```bash
chicha-isotope-map -import-tgz-url https://pelora.org/api/json/weekly.tgz
```
Dopo l’import riavvia normalmente (o lascia lo stesso comando in un servizio systemd) — la mappa si apre su [http://localhost:8765](http://localhost:8765) già piena di misure reali.

### 🛢️ Scelte di database per import e uso quotidiano
- **PostgreSQL (`pgx`)** — più veloce e ideale con più utenti. Esempio: `chicha-isotope-map -db-type pgx -db-conn postgres://USER:PASS@HOST:PORT/DATABASE?sslmode=allow -import-tgz-url https://pelora.org/api/json/weekly.tgz`
- **DuckDB / SQLite / Chai** — soluzioni su file per un utente. Scritture parallele possono confliggere, quindi usale per mappe personali. Esempio: `chicha-isotope-map -db-type duckdb -import-tgz-url https://pelora.org/api/json/weekly.tgz`

## 📤 Esporta
- Traccia singola: `/api/track/{trackID}.json` (anche il vecchio `.cim`).
- Archivio pianificato: `/api/json/weekly.tgz` (o `/daily.tgz`, `/monthly.tgz`, `/yearly.tgz`). Dentro: un JSON per traccia.

---

## 🧠 Opzioni avanzate
- Database: di default SQLite integrato; puoi passare a DuckDB, Chai, ClickHouse o PostgreSQL (`pgx`).
- Import: via URL o file, anche come archivio.
- Export: archivi JSON, traccia singola, compatibilità `.cim`.
- Aspetto: coordinate e layer iniziali (`-default-*`).

---

## 🤝 Perché avere il tuo nodo e un po’ di storia
- Volevamo che chiunque, senza formazione, vedesse se la radiazione minaccia dove vive, coltiva o prende acqua.
- Più nodi esistono, più affidabile è il quadro e minori le possibilità di perdere contaminazione.

Chicha‑Isotope‑Map è ispirata ai passi di **Dmitry Ignatenko** nella ricerca sul campo ed è influenzata da **Rob Oudendijk** e **Safecast**. I dati aperti delle comunità AtomFast e Radiacode la rendono utile. Se la mappa salva anche una sola vita, non è stata creata invano.
