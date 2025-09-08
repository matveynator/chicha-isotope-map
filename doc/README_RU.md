[![Последний стабильный релиз](https://github.com/matveynator/chicha-isotope-map/actions/workflows/release.yml/badge.svg)](https://github.com/matveynator/chicha-isotope-map/actions/workflows/release.yml)

* [🇬🇧 English](/README.md)
* [🇫🇷 Français](/doc/README_FR.md)
* [🇯🇵 日本語](/doc/README_JP.md)
* [🇷🇺 Русский](/doc/README_RU.md)

# chicha-isotope-map — мировая карта ☢️ радиации

---

### 📸 **Живая демонстрация**

<a href="https://pelora.org" target="_blank"><img width="800"  alt="pelora.org chicha-isotope-map пример работы" src="https://github.com/user-attachments/assets/be706959-a2d5-4949-9378-811f4022aa98" /></a>

---

Страница **DeepWiki**, посвящённая *Chicha Isotope Map*, была создана [Rob Ouden](https://github.com/robouden) из проекта **Safecast**, за что мы искренне благодарны.
Эта страница раскрывает внутреннее устройство программы, чтобы разработчики могли понять её основы, проследить логику и продолжить работу, улучшая и расширяя проект.
Благодаря DeepWiki код перестаёт быть просто инструментом — он становится живым проектом, который может расти и развиваться с помощью множества людей.

👉 [DeepWiki: Chicha Isotope Map](https://deepwiki.com/matveynator/chicha-isotope-map)

---

## 🚀 Установка и запуск собственного узла в 2 команды

Никакой лишней информации. Образ уже содержит базу данных (PostgreSQL). Скопируйте команду, запустите — и готово.

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

Когда сертификат будет выпущен, заходите на: [https://example.org](https://example.org)

---

### ⚙️ Настройка через переменные окружения

* `DOMAIN` — включает HTTPS на портах 80/443 с автоматическими сертификатами Let’s Encrypt (для публичного узла).
* `DEFAULT_LAT`, `DEFAULT_LON` — стартовые координаты карты.
* `DEFAULT_ZOOM` — стартовый масштаб (11 удобно для просмотра города).
* `DEFAULT_LAYER` — `OpenStreetMap` или `Google Satellite`.
* `PORT` — внутренний порт приложения (по умолчанию 8765; обычно менять не нужно).

> Совет: храните данные на томе `-v chicha-data:/var/lib/postgresql/data`, чтобы они сохранялись при обновлении контейнера.

---

### 💾 Резервное копирование и восстановление

**Ежедневная резервная копия (03:00):**

```bash
0 3 * * * docker exec chicha-isotope-map pg_dump -U chicha_isotope_map chicha_isotope_map | gzip > /backup/chicha_isotope_map_$(date +\%F).sql.gz
```

**Восстановление из архива:**

```bash
docker exec -it chicha-isotope-map psql -U postgres -c "DROP DATABASE IF EXISTS chicha_isotope_map; CREATE DATABASE chicha_isotope_map OWNER chicha_isotope_map;"

zcat /backup/chicha_isotope_map_2025-07-24.sql.gz | docker exec -i chicha-isotope-map psql -U chicha_isotope_map chicha_isotope_map
```

---

## ⬇️ Скачивание готовых приложений (без Docker)

Возьмите бинарник под свою ОС, сделайте его исполняемым и запустите.

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

Другие платформы (Windows / ARM / BSD) — см. страницу релизов:
[https://github.com/matveynator/chicha-isotope-map/releases/tag/latest](https://github.com/matveynator/chicha-isotope-map/releases/tag/latest)

---

## 🖥 Запуск бинарника (без Docker): флаги и примеры

Если вы запускаете бинарник `chicha-isotope-map` напрямую, вот что важно.

### Основные параметры

* `-domain string` — включает HTTPS и слушает порты 80 и 443 с автоматическими сертификатами Let’s Encrypt. Домен должен указывать на сервер, порты 80/443 быть открыты.

* `-port int` — HTTP-порт (по умолчанию 8765). Удобно для локального запуска.

* `-default-lat float` & `-default-lon float` — начальные координаты карты.

* `-default-zoom int` — стартовый масштаб (обычно 11–13 для города).

* `-default-layer string` — слой карты: `OpenStreetMap` или `Google Satellite`.

### Хранение данных

* `-db-type string` — драйвер базы: `duckdb`, `genji`, `sqlite`, `pgx` (PostgreSQL). По умолчанию `sqlite`.

* `-db-path string` — путь к файлу базы для `duckdb`/`genji`/`sqlite`.

* `-db-host string`, `-db-port int`, `-db-name string`, `-db-user string`, `-db-pass string` — параметры подключения к PostgreSQL (`pgx`).

* `-pg-ssl-mode string` — режим SSL для PostgreSQL: `disable`, `allow`, `prefer` (по умолчанию), `require`, `verify-ca`, `verify-full`.

### Служебные

* `-version` — вывести версию и выйти.

---

## DuckDB

DuckDB — встроенная база данных: один файл, без сервера, написана на C++.
Go-драйвер требует **CGO** и **динамических библиотек**, поэтому сборка выполняется так:

```bash
CGO_ENABLED=1 go build -tags duckdb
```

Запуск:

```bash
./chicha-isotope-map -db-type duckdb
```

*(На macOS можно просто скачать [готовые релизы](https://github.com/matveynator/chicha-isotope-map/releases/tag/latest))*

---

## 🤝 Зачем запускать свой узел?

* **Просто:** своя карта для своей общины.
* **Полезно:** локальная история фоновой радиации для города/района/объекта — и это ваши данные.
* **Полезно сети:** больше узлов → больше прозрачности и устойчивости.

---

Chicha-Isotope-Map — это не просто программа, а окно в мир микрочастиц, невидимых для глаза, но очевидных для прибора.
То, что раньше было догадками, теперь отображается яркими точками на карте: от спокойного зелёного до тревожного красного.

* **Что он читает и откуда?**

  * Файлы `.kml`, `.kmz`, `.json`, `.rctrk`, `.csv`, `.gpx` (AtomFast, RadiaCode, Safecast и др.).
  * Всё сохраняется в базе, чтобы через годы можно было сказать: «12 марта 2008 года здесь было 4,1 µР/ч».

* **Какой фон считать нормой?**

  * Естественный радиационный фон в «чистых» местах — примерно 0,8–4 µР/ч.
  * Всё выше — антропогенное загрязнение. Вы увидите, как изотопы разнеслись ветром, транспортом и людьми — словно следы на свежем снегу.

---

### 📸 **Скриншоты**

… В советское время в Кисловодском парке строили открытый бассейн. Возможно, использовали бетон с завода в Пятигорске, где перерабатывали радиоакционную руду с горы Бештау.
Грузовики возили бетон, пыль с их колёс оседала на асфальте, оставляя невидимые следы.
Годы прошли, но эти следы всё ещё светятся — напоминание о прошлом.
Пыль, разлетавшаяся по стройке, осела в парке; на карте она видна жёлтыми пятнами, словно клочья осенней листвы. Остальная часть парка — чистая, спокойная, зелёная. <img src="https://repository-images.githubusercontent.com/870016860/11fd6abc-fe8b-4cd8-95c2-df1c631c8762">

---

### 📣 Как заявить о своём узле

1. Поднимите узел на домене с HTTPS (`-e DOMAIN=...` + откройте порты `80` и `443`).
2. Добавьте скриншот и краткое описание (город, район, какие измерения охватываете).
3. Оставьте заметку в Issues репозитория проекта.

---

*Chicha Isotope Map* создан для **Лаборатории радиационных исследований Дмитрия Игнатенко** и вдохновлён **Safecast** — удивительным японским сообществом гражданских учёных, превративших трагедию Фукусимы в дар научного знания.

Измеряя, исследуя и открыто делясь правдой о радиации, вы делаете невидимое видимым. Тем самым вы помогаете миру не повторять болезненные уроки Чернобыля и Фукусимы.

Ваша работа — это свет: науки, безопасности и надежды.
Спасибо, что превратили радиационный фон из источника страха в предмет понимания.

Спасибо за смелость: искать, измерять, делиться — и прежде всего, быть первыми, кто сделал шаг вперёд.

Мы также искренне благодарим сообщества **AtomFast** и **Radiacode** за их ценный вклад — за измерения и за щедрый обмен данными с миром.
