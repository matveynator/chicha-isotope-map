# 🌌 Chicha Isotope Map — увидеть невидимое

> *«Если бы изотопы умели рассказывать свои истории — эта программа не понадобилась бы. Но раз не умеют, за них говорит Chicha‑Isotope‑Map.»*

*Chicha Isotope Map* создан специально для **Лаборатории радиационных исследований Дмитрия Игнатенко** и вдохновлён японской инициативой [**Safecast**](https://map.safecast.org) — мировым сообществом граждан‑учёных. Измеряя и публикуя данные о радиации, мы делаем невидимое видимым, чтобы трагедии **Чернобыля** и **Фукусимы** не повторились.

---

## 📖 О проекте

Chicha‑Isotope‑Map визуализирует радиоактивные частицы на карте: от зелёного (фон 1‑4 μR/h) до красного (опасное загрязнение). Поддерживаются форматы треков `.kml`, `.kmz`, `.json`, `.rctrk` (AtomFast/RadiaCode). Данные сохраняются в БД, чтобы спустя годы видеть историю фона.

---

## 📥 Скачать и установить

> **Установка выполняется от root (или с `sudo`).**

| Платформа               | Команда                                                                                                                                                                                                 |
| ----------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| **Linux amd64**         | `curl -L https://github.com/matveynator/chicha-isotope-map/releases/download/latest/chicha-isotope-map_linux_amd64 -o /usr/local/bin/chicha-isotope-map && chmod +x /usr/local/bin/chicha-isotope-map`  |
| **macOS Intel**         | `curl -L https://github.com/matveynator/chicha-isotope-map/releases/download/latest/chicha-isotope-map_darwin_amd64 -o /usr/local/bin/chicha-isotope-map && chmod +x /usr/local/bin/chicha-isotope-map` |
| **macOS Apple Silicon** | `curl -L https://github.com/matveynator/chicha-isotope-map/releases/download/latest/chicha-isotope-map_darwin_amd64 -o /usr/local/bin/chicha-isotope-map && chmod +x /usr/local/bin/chicha-isotope-map` |
| **Windows**             | Скачайте `chicha-isotope-map_windows_amd64.exe` и запустите в PowerShell.                                                                                                                               |

Другие сборки (FreeBSD, OpenBSD, NetBSD) доступны на [странице релиза](https://github.com/matveynator/chicha-isotope-map/releases/tag/latest).

Проверьте версию:

```bash
chicha-isotope-map --version
```

---

## 🚀 Быстрый старт (локально)

```bash
chicha-isotope-map       # запускает сервер на http://localhost:8765
```

Откройте [http://localhost:8765](http://localhost:8765), нажмите **Upload** и загрузите трек.

*По умолчанию используется встроенная БД Genji — отдельная СУБД не нужна.*

---

## 🌍 Показ при старте зоны известных аварий (пример использования)

```bash
# Чернобыль (1986)
./chicha-isotope-map -default-lat=51.389 -default-lon=30.099 -default-zoom=11 -default-layer="OpenStreetMap"

# Фукусима‑Дайичи (2011)
./chicha-isotope-map -default-lat=37.421 -default-lon=141.033 -default-zoom=12 -default-layer="Google Satellite"
```

---

## ⚙️ Расширенные параметры

```bash
chicha-isotope-map \
  --port=8765 \
  --db-type=pgx \
  --db-host=localhost --db-port=5432 \
  --db-user=postgres --db-pass=secret \
  --db-name=isotope_db --pg-ssl-mode=require
```

Полный список флагов: `chicha-isotope-map --help`.

---

## 🖥 Минимальные требования сервера

| Ресурс | Минимум                             | Комментарий             |
| ------ | ----------------------------------- | ----------------------- |
| CPU    | 1 vCPU                              | Для базовой карты       |
| RAM    | 512 МБ                              | Genji/SQLite            |
| Диск   | 1 ГБ+                               | 1 трек ≈ 50–200 КБ      |
| ОС     | Linux/Unix/macOS (Windows работает) |                         |
| Порты  | 80/443                              | Нужны для Let’s Encrypt |

---

## 🌐 Развёртывание на собственном сервере

### 1. Загрузка и права

```bash
sudo wget -O /usr/local/bin/chicha-isotope-map \
     https://github.com/matveynator/chicha-isotope-map/releases/download/latest/chicha-isotope-map_linux_amd64
sudo chmod +x /usr/local/bin/chicha-isotope-map
```

### 2. Тестовый запуск (HTTP)

```bash
chicha-isotope-map -port=8765
```

Перейдите на `http://<IP‑сервера>:8765`.

### 3. Запуск со своим доменом и HTTPS

```bash
sudo chicha-isotope-map -domain=map.example.org
```

* ACME‑проверка на :80.
* Автоматический сертификат Let’s Encrypt на :443.
* Продление каждые 24 ч.

Убедитесь, что DNS A/AAAA домена смотрит на сервер и порты 80/443 открыты.

### 4. Работа за обратным прокси

* Запустите приложение без `-domain`, например `-port=8765`.
* Настройте Nginx/Caddy и проксируйте на `127.0.0.1:8765`.
* Прокси отвечает за TLS.

### 5. Выбор СУБД

| Драйвер        | Флаг                                   | Когда использовать                           |
| -------------- | -------------------------------------- | -------------------------------------------- |
| **Genji**      | по умолчанию                           | Локальный диск, простая БД в одном файле     |
| **SQLite**     | `-db-type=sqlite`                      | Нужен привычный SQL‑движок одной библиотекой |
| **PostgreSQL** | `-db-type=pgx` + параметры подключения | Много пользователей, репликация, pg\_dump    |

> Файлы Genji/SQLite (`database-<порт>.genji`, `.sqlite`) можно копировать «горячо» для резервного копирования.

### 6. Автозапуск (systemd)

```ini
# /etc/systemd/system/chicha-isotope-map.service
[Unit]
Description=Chicha Isotope Map server
After=network.target

[Service]
User=www-data
WorkingDirectory=/opt/chicha
ExecStart=/opt/chicha/chicha-isotope-map -port=8765
Restart=on-failure

[Install]
WantedBy=multi-user.target
```

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now chicha-isotope-map
```

### 7. Резервное копирование

| СУБД         | Как бэкапить                                        |
| ------------ | --------------------------------------------------- |
| Genji/SQLite | `rsync /opt/chicha/database-* backups/`             |
| PostgreSQL   | `pg_dump -Fc isotope_db > backups/$(date +%F).dump` |

Храните копии в другом регионе/облаке, чтобы данные не потерялись.

---

## 🤝 Присоединяйтесь к распределённой карте

1. Собирайте треки дозиметрами (RadiaCode, AtomFast, др.).
2. Загрузите их на свой узел (`/upload`).
3. Поделитесь ссылкой или откройте issue в репозитории — мы добавим ваш сайт в каталог.

> **Больше независимых узлов → надёжнее данные.**

---

## ❓ FAQ

| Вопрос                         | Ответ                                                                                        |
| ------------------------------ | -------------------------------------------------------------------------------------------- |
| Можно ли запускать на Windows? | Да, через `*.exe`.                                                                           |
| Как обновиться?                | Остановите сервис, скачайте новый файл, замените и запустите. БД обновлять не надо.          |
| Как изменить порт?             | `-port=12345`. С опцией `-domain` — ACME всё равно требует 80/443.                           |

---

## 🔗 Полезные ссылки

* **Демо‑сервер**: [https://jutsa.ru](https://jutsa.ru)
* **Safecast**: [https://map.safecast.org](https://map.safecast.org)
* **Документация PostgreSQL**: [https://postgresql.org](https://postgresql.org)
* **GitHub‑релизы**: [https://github.com/matveynator/chicha-isotope-map/releases](https://github.com/matveynator/chicha-isotope-map/releases)

---

С вопросами пишите в GitHub Issues. Спасибо, что делаете невидимое видимым!
