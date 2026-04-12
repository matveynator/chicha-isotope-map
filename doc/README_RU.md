[![Последний стабильный релиз](https://github.com/matveynator/chicha-isotope-map/actions/workflows/release.yml/badge.svg)](https://github.com/matveynator/chicha-isotope-map/actions/workflows/release.yml)

<img width="30%" align="left" alt="chicha-isotope-map" src="https://github.com/user-attachments/assets/39bfa7b1-03fb-43dd-89bd-8d6c516fd4db" />

* [🇬🇧 English](/README.md)
* [🇫🇷 Français](/doc/README_FR.md)
* [🇯🇵 日本語](/doc/README_JP.md)
* [🇷🇺 Русский](/doc/README_RU.md)
* [🇨🇭 Schwiizerdütsch](/doc/README_DE_CH.md)
* [🇮🇹 Italiano](/doc/README_IT.md)
* [🇨🇳 中文](/doc/README_ZH.md)
* [🇮🇳 हिन्दी](/doc/README_HI.md)
* [🇮🇷 فارسی](/doc/README_FA.md)
* [🇲🇳 Монгол](/doc/README_MN.md)
* [🇰🇿 Қазақша](/doc/README_KK.md)

# ☢️ Мировая карта радиации
Мы хотели создать карту, которая помогла бы каждому человеку сразу понять, безопасно ли место, где он живёт или работает. Ведь многие выращивают овощи, держат скот, пьют воду из родников — и не всегда знают, благополучна ли окружающая среда.

Обычный природный уровень радиации невысок. Опасность возникает лишь там, где показатели заметно превышены — из-за техногенных причин или особенностей местности. В таких местах вода, воздух и почва могут со временем повлиять на здоровье: вызвать болезни лёгких, желудка и других органов.

Если эта карта убережёт хотя бы одного человека или одно живое существо, значит, она создана не напрасно. Пусть она послужит простым и понятным ориентиром, помогающим выбирать безопасный путь.

Живая демо: [https://pelora.org/](https://pelora.org/) — ваш узел будет выглядеть так же.

👉 [Страница Stable Release](https://github.com/matveynator/chicha-isotope-map/releases/tag/stable-release) (постоянный URL, всегда ведёт на актуальные стабильные бинарники)

👉 [DeepWiki: Chicha Isotope Map](https://deepwiki.com/matveynator/chicha-isotope-map)

---

### 📸 Пример
<p>
  <a href="https://pelora.org" target="_blank"><img width="100%" alt="Fukushima view in chicha-isotope-map" src="https://github.com/user-attachments/assets/617a0ced-4280-41c2-9320-de1cfd33a61f" /></a><br />
  <a href="https://pelora.org" target="_blank"><img width="100%" alt="Safecast realtime radiation sensors in chicha-isotope-map" src="https://github.com/user-attachments/assets/13256b23-744d-4d02-a26c-ae9aef5b0d87" /></a><br />
  <a href="https://pelora.org" target="_blank"><img width="100%" alt="Air flights radiation in chicha-isotope-map" src="https://github.com/user-attachments/assets/cf0189c9-534f-4ff5-9d7a-ed5836e91ef5" /></a>
</p>

---

Вот весь текст целиком, включая обновлённый пункт — в спокойном, культурном и понятном стиле:

---

## 🧭 Что внутри

* Карта собирает измерения с разных приборов; слои аккуратно разделены по скорости движения — пешком, на машине, в полёте.
* Можно загрузить свои треки: новые точки сразу появляются на карте и помогают лучше понять обстановку.
* Можно импортировать архив данных по ссылке или файлом, а также сохранить свои данные в архив (удобно для бекапа).
* Есть возможность проследить, как менялся уровень радиации в выбранном месте со временем — становилась ситуация лучше или хуже.
* Можно создать короткую ссылку на нужный участок карты.
* Есть режим печати: можно отмечать опасные участки QR-кодами. Это удобно для предупреждающих знаков — человек сканирует код и сразу видит уровень радиации в конкретной точке. Такой подход помогает обозначать места экологического неблагополучия, где нежелательно пить воду, длительно находиться или вести хозяйство. Это полезно экологам, специалистам по мониторингу и службам, отвечающим за предупреждение людей об опасности.
* У Карты есть API для интеграции ее данных во внешние сервисы на условии открытой лицензии CC.

Проект развивается благодаря внимательной поддержке сообщества **Safecast**, огромной работе **Rob Oudendijk**, а также усилиям людей по всему миру, занимающихся открытой дозиметрией. Мы благодарим Safecast, AtomFast, Radiacode, DoseMap и другие  инициативы за их вклад и участие.


---

## 🚀 Быстрый старт
Самый быстрый путь — готовый бинарник. Никакой Docker, БД или внешние инструменты не нужны: скачали, запустили, карта готова.

### Вариант 1. Готовый бинарник (рекомендуется)
1. Откройте [страницу Stable Release](https://github.com/matveynator/chicha-isotope-map/releases/tag/stable-release) и скачайте файл под свою систему.

Постоянные ссылки на серверные бинарники (не меняются между релизами):
- FreeBSD amd64: `https://github.com/matveynator/chicha-isotope-map/releases/download/stable-release/chicha-isotope-map_freebsd_amd64`
- FreeBSD arm64: `https://github.com/matveynator/chicha-isotope-map/releases/download/stable-release/chicha-isotope-map_freebsd_arm64`
- OpenBSD amd64: `https://github.com/matveynator/chicha-isotope-map/releases/download/stable-release/chicha-isotope-map_openbsd_amd64`
- OpenBSD arm64: `https://github.com/matveynator/chicha-isotope-map/releases/download/stable-release/chicha-isotope-map_openbsd_arm64`
- macOS amd64: `https://github.com/matveynator/chicha-isotope-map/releases/download/stable-release/chicha-isotope-map_darwin_amd64`
- macOS arm64: `https://github.com/matveynator/chicha-isotope-map/releases/download/stable-release/chicha-isotope-map_darwin_arm64`
2. Сделайте файл исполняемым и запустите:
   ```bash
   chmod +x ./chicha-isotope-map
   ./chicha-isotope-map
   ```
3. Откройте [http://localhost:8765](http://localhost:8765) — карта уже работает.

Минимальная настройка (по желанию):
- `-port 8765` — порт для локального запуска.
- `-domain maps.example.org` — подключить домен и HTTPS (потребуются 80/443).
- `-default-lat` / `-default-lon` / `-default-zoom` / `-default-layer` — стартовый вид карты.
- Хранилище: `-db-type sqlite|duckdb|chai|clickhouse|pgx`, `-db-path` для файловых баз, `-db-conn` для сетевых.

### Вариант 2. Публичный узел с доменом
1. Запустите бинарник с вашим доменом:
   ```bash
   ./chicha-isotope-map -domain example.org
   ```
2. Порты 80/443 понадобятся для сертификата Let’s Encrypt. После выпуска карта будет по адресу [https://example.org](https://example.org).

### Вариант 3. Docker (всё уже упаковано)
1. Установите приложение Docker (Desktop или CLI).
2. Найдите образ **matveynator/chicha-isotope-map** на Docker Hub и нажмите **Run** (или выполните одну команду):
   ```bash
   docker run -d -p 8765:8765 --name chicha-isotope-map matveynator/chicha-isotope-map:latest
   ```
3. Откройте [http://localhost:8765](http://localhost:8765) — готово.

---

## 📥 Как наполнить данными
- На странице карты нажмите зелёную кнопку **Upload** и загрузите свои треки (`.kml`, `.kmz`, `.json`, `.rctrk`, `.csv`, `.gpx`, логи bGeigie Nano/Zen `$BNRDD`, экспорт AtomFast, RadiaCode, Safecast и др.).
- Хотите полную копию pelora.org? Один раз выполните `chicha-isotope-map -import-tgz-url https://pelora.org/api/json/weekly.tgz` — он скачает еженедельный архив, заполнит базу и завершится, чтобы следующий запуск сразу показывал живые точки.
- Если удобнее скачать архив заранее: возьмите [https://pelora.org/api/json/weekly.tgz](https://pelora.org/api/json/weekly.tgz), запустите с `-import-tgz-path /путь/к/weekly.tgz` и поднимайте карту с локальной копией.

### 🗺️ Первый запуск с живыми данными одной командой
На чистой системе введите только это — и карта сразу наполнится и откроется:
```bash
chicha-isotope-map -import-tgz-url https://pelora.org/api/json/weekly.tgz
```
После импорта перезапустите обычным способом (или оставьте эту же команду в сервисе systemd). Откройте [http://localhost:8765](http://localhost:8765) — на карте сразу видны реальные измерения.

### 🛢️ Вариции с базами данных при импорте и в обычной работе:
- **PostgreSQL (`pgx`)** — самая быстрая и удобная при нескольких пользователях. Пример: `chicha-isotope-map -db-type pgx -db-conn postgres://USER:PASS@HOST:PORT/DATABASE?sslmode=allow -import-tgz-url https://pelora.org/api/json/weekly.tgz`
- **DuckDB / SQLite / Chai** — простые файловые базы для одного пользователя. При параллельной записи возможны конфликты, поэтому оставляйте их для персональных карт. Пример: `chicha-isotope-map -db-type duckdb -import-tgz-url https://pelora.org/api/json/weekly.tgz`

## 📤 Экспорт
- Один трек: `/api/track/{trackID}.json` (старые `.cim` тоже работают).
- Архив по расписанию: `/api/json/weekly.tgz` (или `/daily.tgz`, `/monthly.tgz`, `/yearly.tgz`). Внутри — отдельный JSON-файл на каждый трек.

---

## 🧠 Продвинутые опции
- Базы данных: по умолчанию встроенная SQLite; можно переключить на DuckDB, Chai, ClickHouse или PostgreSQL (`pgx`).
- Импорт: по URL или файлом, можно подавать сразу архив.
- Экспорт: JSON-архивы, один трек, старые `.cim` поддерживаются.
- Настройка внешнего вида: стартовые координаты и слой (`-default-*`).

---

## 🤝 Зачем свой узел и немного истории
- Мы хотели, чтобы человек без подготовки мог увидеть: есть ли радиация там, где он живёт, растит еду или берёт воду.
- Чем больше узлов, тем надёжнее общая картина и меньше шанс пропустить загрязнение.

Chicha‑Isotope‑Map вдохновлена шагами **Дмитрия Игнатенко** в полевых исследованиях, а также идеями **Rob Oudendijk** и проекта **Safecast**. Открытые данные сообществ AtomFast и Radiacode помогают делать карту полезной. Если карта поможет спасти чью-то жизнь, значит, она сделана не зря.
