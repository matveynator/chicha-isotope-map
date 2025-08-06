- [🇬🇧 English](/README.md)
- [🇫🇷 Français](/doc/README_FR.md)
- [🇯🇵 日本語](/doc/README_JP.md)
- [🇷🇺 Русский](/doc/README_RU.md)


# ☢️ Chicha‑Isotope‑Map — персональная радиационная карта.

---

## 🚀 Установка и собственный узел за 2 команды

Ничего лишнего. В образ уже встроена база (PostgreSQL). Скопируйте команду, выполните — и готово.

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

Откройте: [http://localhost:8765](http://localhost:8765)

#### 🔥 Публичный узел с HTTPS на своём домене

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

После выпуска сертификата зайдите: [https://example.org](https://example.org)

---

### ⚙️ Настройки через переменные окружения (минимум нужного)

* `DOMAIN` — включает HTTPS на 80/443 и автосертификаты Let’s Encrypt (для публичного узла).
* `DEFAULT_LAT`, `DEFAULT_LON` — стартовые координаты карты.
* `DEFAULT_ZOOM` — стартовый зум (11 — удобный городской уровень).
* `DEFAULT_LAYER` — `OpenStreetMap` или `Google Satellite`.
* `PORT` — внутренний порт приложения (по умолчанию 8765; обычно не меняем).

> Совет: держите данные на томе `-v chicha-data:/var/lib/postgresql/data`, чтобы они переживали обновления контейнера.

---

### 💾 Бэкап и восстановление (просто)

**Ежедневный бэкап (03:00):**

```bash
0 3 * * * docker exec chicha-isotope-map pg_dump -U chicha_isotope_map chicha_isotope_map | gzip > /backup/chicha_isotope_map_$(date +\%F).sql.gz
```

**Восстановление из архива:**

```bash
docker exec -it chicha-isotope-map psql -U postgres -c "DROP DATABASE IF EXISTS chicha_isotope_map; CREATE DATABASE chicha_isotope_map OWNER chicha_isotope_map;"

zcat /backup/chicha_isotope_map_2025-07-24.sql.gz | docker exec -i chicha-isotope-map psql -U chicha_isotope_map chicha_isotope_map
```

---


## ⬇️ Скачать готовые программы (без Docker)

Скачайте бинарник под вашу систему, дайте права на запуск и стартуйте.

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

Остальные платформы (Windows / ARM / BSD) — на странице релизов:
[https://github.com/matveynator/chicha-isotope-map/releases/tag/latest](https://github.com/matveynator/chicha-isotope-map/releases/tag/latest)

---

## 🖥 Обычный запуск (без Docker): флаги и примеры

Если запускаете бинарник `chicha-isotope-map` напрямую, вот главное. Сначала самое важное, ниже — дополнительные параметры.

### Важное

* `-domain string` — включает HTTPS и работу на портах 80 и 443 с автосертификатами Let’s Encrypt. Требуется, чтобы домен указывал на ваш сервер и порты 80/443 были свободны.

  * Пример: `sudo chicha-isotope-map -domain maps.example.org -default-lat 44.08832 -default-lon 42.97577 -default-zoom 11 -default-layer "OpenStreetMap"`

* `-port int` — порт HTTP‑сервера (по умолчанию 8765). Удобно для локального запуска без домена.

  * Пример: `chicha-isotope-map -port 8765`

* `-default-lat float` и `-default-lon float` — стартовая широта и долгота карты.

  * Пример: `-default-lat 44.08832 -default-lon 42.97577`

* `-default-zoom int` — стартовый масштаб (город обычно 11–13).

  * Пример: `-default-zoom 11`

* `-default-layer string` — базовый слой: `OpenStreetMap` или `Google Satellite`.

  * Пример: `-default-layer "Google Satellite"`

### Хранилище (если нужно)

* `-db-type string` — драйвер БД: `genji`, `sqlite`, `pgx` (PostgreSQL). По умолчанию `genji`.

* `-db-path string` — путь к файлу БД для `genji`/`sqlite` (если не указан — текущая папка).

  * Пример: `-db-type sqlite -db-path /var/lib/chicha/chicha.sqlite`

* `-db-host string`, `-db-port int` (по умолчанию 5432), `-db-name string`, `-db-user string`, `-db-pass string` — параметры подключения к PostgreSQL для `pgx`.

  * Пример: `-db-type pgx -db-host 127.0.0.1 -db-port 5432 -db-name chicha_isotope_map -db-user postgres -db-pass secret`

* `-pg-ssl-mode string` — SSL‑режим для PostgreSQL: `disable`, `allow`, `prefer` (по умолчанию), `require`, `verify-ca`, `verify-full`.

  * Пример: `-pg-ssl-mode require`

### Сервисные

* `-version` — показать версию и выйти.

### Быстрые примеры

* **Локально, без HTTPS:**

  ```bash
  chicha-isotope-map \
    -port 8765 \
    -default-lat 44.08832 -default-lon 42.97577 \
    -default-zoom 11 \
    -default-layer "OpenStreetMap"
  ```

* **Публичный сервер с HTTPS на 80/443:**

  ```bash
  sudo chicha-isotope-map \
    -domain maps.example.org \
    -default-lat 44.08832 -default-lon 42.97577 \
    -default-zoom 11 \
    -default-layer "OpenStreetMap"
  ```

* **Хранение в одном файле (SQLite):**

  ```bash
  chicha-isotope-map \
    -db-type sqlite -db-path /var/lib/chicha-isotope-map.sqlite \
    -port 8765 \
    -default-lat 44.08832 -default-lon 42.97577 -default-zoom 11
  ```

* **Подключение к PostgreSQL:**

  ```bash
  chicha-isotope-map \
    -db-type pgx \
    -db-host 127.0.0.1 -db-port 5432 \
    -db-name chicha_isotope_map -db-user postgres -db-pass secret \
    -pg-ssl-mode require \
    -default-lat 44.08832 -default-lon 42.97577 -default-zoom 11
  ```

---

## 🤝 Зачем поднимать свой узел?

* **Просто:** у вас свое сообщество — и у вас своя карта.
* **Полезно:** локальная история фона города/района/предприятия — навсегда у вас.
* **Важно для сети:** больше узлов → больше прозрачности и устойчивости.

---

Chicha‑Isotope‑Map — это не просто программа, это окно в мир микрочастиц, невидимых глазом, но ощутимых прибором. Прежде их можно было только угадывать, теперь же они нарисованы яркими точками на карте: от спокойного зелёного до тревожного красного.

* **Что и откуда она читает?**

  * Из файлов форматов `.kml`, `.kmz`, `.json`, `.rctrk`, `.csv`, `.gpx` форматы (AtomFast, RadiaCode, Safecast  и другие).
  * Сохраняет всё в собственной базе, чтобы вы через годы могли точно узнать: «А 12 марта 2008 года здесь было 4.1 µR/h».

* **От чего отталкиваемся?**

  * От природного фона радиации: в «чистом» месте — примерно 0.8–4 µR/h.
  * Всё, что выше — инородное загрязнение. Вы увидите, как изотопы разлетелись ветром, машинами и людьми, словно следы на свежевыпавшем снегу.

---

### 📸 **Скриншоты**

... В советское время в Кисловодском парке строили открытый бассейн. Возможно, взяли бетон с завода в Пятигорске, где когда-то перерабатывали радиоактивную руду с горы Бештау. И вот, по дороге ездили грузовики, пыль с их колес оседала на асфальте, оставляя свои невидимые метки. Прошли годы, а эти следы всё ещё светятся, словно воспоминания о былом. Пыль, разлетевшаяся вокруг стройки, осела в парке — на карте она жёлтым цветом, словно пятна осенних листьев. Всё остальное в парке — чистое, спокойное, зелёное. <img src="https://repository-images.githubusercontent.com/870016860/11fd6abc-fe8b-4cd8-95c2-df1c631c8762">

---

### 📸 **Демо**

<a href="https://jutsa.ru" target="_blank"><img width="1156" height="844" alt="Chicha Isotope Map" src="https://github.com/user-attachments/assets/8d806377-671f-47a0-b918-f2a9afd4123e" /></a>

<a href="https://jutsa.ru" target="_blank">Вот здесь можно посмотреть работу программы в реальном времени.</a>

---

### 📣 Как анонсировать свой узел сообществу

1. Поднимите узел на домене с HTTPS (`-e DOMAIN=...` + проброс `80` и `443`).
2. Добавьте скриншот и краткое описание (город, район, что покрываете измерениями).
3. Отметьтесь в Issue репозитория проекта.

Карта изотопов Chicha была создана для **Лаборатории радиационных исследований Дмитрия Игнатенко** и вдохновлена **японским сообществом Safecast** — группой граждан‑учёных, которые превратили трагедию Фукусимы в научное знание. Ища, измеряя и распространяя правду о радиации, вы делаете невидимое — видимым, помогая миру не повторить Чернобыль и Фукусиму. Ваша работа — это свет науки, безопасности и надежды. Спасибо вам за то, что превращаете фоновое излучение из повода для страха в источник понимания, за то, что ищете, измеряете, делитесь — и с мужеством идёте первыми.
