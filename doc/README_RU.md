[![Последний стабильный релиз](https://github.com/matveynator/chicha-isotope-map/actions/workflows/release.yml/badge.svg)](https://github.com/matveynator/chicha-isotope-map/actions/workflows/release.yml)

<img width="30%" align="left" alt="chicha-isotope-map" src="https://github.com/user-attachments/assets/39bfa7b1-03fb-43dd-89bd-8d6c516fd4db" />

* [🇬🇧 English](/README.md)
* [🇫🇷 Français](/doc/README_FR.md)
* [🇯🇵 日本語](/doc/README_JP.md)
* [🇷🇺 Русский](/doc/README_RU.md)

# ☢️ Мировая карта радиации
Живая демо: [https://pelora.org/](https://pelora.org/) — ваш узел будет выглядеть так же.

👉 [DeepWiki: Chicha Isotope Map](https://deepwiki.com/matveynator/chicha-isotope-map)

---

### 📸 Пример
<a href="https://pelora.org" target="_blank"><img width="800" alt="pelora.org chicha-isotope-map пример" src="https://github.com/user-attachments/assets/be706959-a2d5-4949-9378-811f4022aa98" /></a>

---

## 🚀 Запуск через Docker (самое быстрое)
Образ уже содержит PostgreSQL. Копируйте и запускайте.

#### 🔥 Локально (порт 8765)
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
Открыть: [http://localhost:8765](http://localhost:8765)

#### 🔥 Публичный узел с HTTPS
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
После выпуска сертификата: [https://example.org](https://example.org)

**Переменные:** `DOMAIN` для HTTPS, `DEFAULT_LAT` / `DEFAULT_LON` / `DEFAULT_ZOOM` / `DEFAULT_LAYER` для стартового вида, `PORT` для внутреннего порта. Данные храните на `-v chicha-data:/var/lib/postgresql/data`, чтобы обновления контейнера не стирали историю.

---

## ⬇️ Готовые бинарники (без Docker)
Скачайте, сделайте исполняемым, запустите.

**Linux x64**
```bash
sudo curl -L https://github.com/matveynator/chicha-isotope-map/releases/download/latest/chicha-isotope-map_linux_amd64 \
  -o /usr/local/bin/chicha-isotope-map \
  && sudo chmod +x /usr/local/bin/chicha-isotope-map \
  && chicha-isotope-map
```

**macOS Intel (x86_64)**
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

Другие платформы (Windows / ARM / BSD): [страница релиза](https://github.com/matveynator/chicha-isotope-map/releases/tag/latest).

---

## 🖥 Запуск бинарника
Кратко о флагах:
- `-domain maps.example.org` — HTTPS на 80/443 (Let’s Encrypt).
- `-port 8765` — порт для локального запуска.
- `-default-lat` / `-default-lon` / `-default-zoom` / `-default-layer` — стартовый вид карты.
- Хранилище: `-db-type sqlite|duckdb|pgx|chai|clickhouse`, `-db-path` для файловых баз, `-db-conn` для сетевых.
- Служебный: `-version` выводит версию.

DuckDB: `CGO_ENABLED=1 go build -tags duckdb`, затем `./chicha-isotope-map -db-type duckdb`.

---

## 📥 Импорт данных
- Поддерживаются `.kml`, `.kmz`, `.json`, `.rctrk`, `.csv`, `.gpx`, логи bGeigie Nano/Zen `$BNRDD` (`.log` / `.txt`), экспорт AtomFast, RadiaCode, Safecast и др.
- Веб: открыть узел → **Upload** → выбрать файлы → последний импортированный трек откроется сам.
- API: `curl -F 'files[]=@/path/to/file.log' http://localhost:8765/upload` (диагностика: `/upload_diag`).
- Свежие точки рядом: `/api/latest?lat=...&lon=...&radius_m=1500&limit=20`.

---

## 📤 Экспорт данных
- **По треку:** `/api/track/{trackID}.json` (старые `.cim` тоже работают). Параметры `from`/`to` сужают диапазон ID.
- **Архив:** `/api/json/weekly.tgz` (или `/daily.tgz`, `/monthly.tgz`, `/yearly.tgz`, если настроено). Внутри — отдельный JSON-файл на каждый трек.
- **Формат JSON:**
  - Верхний уровень: `trackID`, `trackIndex` (позиция с 1), `apiURL`, `firstID`, `lastID`, `markerCount`, `disclaimers`, `markers`.
  - Точка: `id`, `timeUnix`, `timeUTC` (RFC3339), `lat`, `lon`, опционально `altitudeM`, `temperatureC`, `humidityPercent`, скорости (`speedMS`, `speedKMH`), дозы (`doseRateMicroSvH`, `doseRateMicroRh`), `countRateCPS`, при наличии `detectorType`, `detectorName`, `radiationTypes`.
  - В `disclaimers` лежат уведомления на разных языках.
- **Планы:** в этот же JSON со временем добавим спектрометрические данные по каждой точке.

---

## 💾 Резервные копии
- **Ежедневно (03:00):** `0 3 * * * docker exec chicha-isotope-map pg_dump -U chicha_isotope_map chicha_isotope_map | gzip > /backup/chicha_isotope_map_$(date +\%F).sql.gz`
- **Восстановление:**
  ```bash
  docker exec -it chicha-isotope-map psql -U postgres -c "DROP DATABASE IF EXISTS chicha_isotope_map; CREATE DATABASE chicha_isotope_map OWNER chicha_isotope_map;"
  zcat /backup/chicha_isotope_map_2025-07-24.sql.gz | docker exec -i chicha-isotope-map psql -U chicha_isotope_map chicha_isotope_map
  ```

---

## 🤝 Зачем свой узел?
- Своя карта, свои измерения и история.
- Видно, как фон (обычно 0.8–4 µR/h) менялся во времени.
- Больше узлов → больше прозрачности и устойчивости для всех.

Chicha‑Isotope‑Map создана для **лаборатории Дмитрия Игнатенко** и вдохновлена **Safecast**. Спасибо сообществам AtomFast и Radiacode за открытые данные.
