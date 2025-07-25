# 🌌 Chicha‑Isotope‑Map — путеводитель по скрытым тропам радиации

Chicha‑Isotope‑Map — это не просто программа, это окно в мир микрочастиц, невидимых глазом, но ощутимых прибором. Прежде их можно было только угадывать, теперь же они нарисованы яркими точками на карте: от спокойного зелёного до тревожного красного.

* **Что и откуда она читает?**

  * Из файлов форматов `.kml`, `.kmz`, `.json`, `.rctrk` (AtomFast, RadiaCode).
  * Сохраняет всё в собственной базе, чтобы вы через годы могли точно узнать: «А 12 марта 2024 года здесь было 4.1 µR/h».

* **От чего отталкиваемся?**

  * От природного фона радиации: в «чистом» месте — примерно 0.8–4 µR/h.
  * Всё, что выше — инородное загрязнение. Вы увидите, как изотопы разлетелись ветром, машинами и людьми, словно следы на свежевыпавшем снегу.

---

### 📸 **Скриншоты**

... В советское время в Кисловодском парке строили открытый бассейн. Возможно, взяли бетон с завода в Пятигорске, где когда-то перерабатывали радиоактивную руду с горы Бештау. И вот, по дороге ездили грузовики, пыль с их колес оседала на асфальте, оставляя свои невидимые метки. Прошли годы, а эти следы всё ещё светятся, словно воспоминания о былом. Пыль, разлетевшаяся вокруг стройки, осела в парке — на карте она жёлтым цветом, словно пятна осенних листьев. Всё остальное в парке — чистое, спокойное, зелёное.
<img src="https://repository-images.githubusercontent.com/870016860/11fd6abc-fe8b-4cd8-95c2-df1c631c8762">

---

### 📸 **Демо**

<a href="https://jutsa.ru" target="_blank"><img width="1156" height="844" alt="Chicha Isotope Map" src="https://github.com/user-attachments/assets/8d806377-671f-47a0-b918-f2a9afd4123e" /></a>

<a href="https://jutsa.ru" target="_blank">Вот здесь можно посмотреть работу программы в реальном времени.</a>

---

## 🚀 Установка и собственный узел за 5 минут!

### 1. Быстрый старт в Docker

**Почему Docker?**
Docker упаковывает программу и её окружение в «контейнер». Вам не нужно вручную настраивать базы и зависимости — просто запустите готовый образ.

#### Локальный запуск (порт 5000)

```bash
docker run -d \
  --name chicha-isotope-map \
  -e PORT=5000 \
  -p 5000:5000 \
  -v isotope-data:/var/lib/postgresql/data \
  matveynator/chicha-isotope-map:latest
```

Открываете в браузере [http://localhost:5000](http://localhost:5000) и видите карту.

#### В зоне своего домена и с HTTPS

1. Убедитесь, что `domain.com` указывает на IP вашего сервера.
2. Порты 80 и 443 свободны.
3. Запустите команду от **root**:

```bash
docker run -d \
  --name chicha-isotope-map \
  -e DOMAIN=domain.com \
  -p 80:80 -p 443:443 \
  -v isotope-data:/var/lib/postgresql/data \
  matveynator/chicha-isotope-map:latest
```

Программа автоматически получит и будет продлевать SSL‑сертификаты.

#### Дополнительные настройки карты 

Все доступные опции можно увидеть по вызову --help у программы.
По желанию укажите стартовую точку и стиль:

```text
  -e DEFAULT_LAT=51.389      # широта
  -e DEFAULT_LON=30.099      # долгота
  -e DEFAULT_ZOOM=11         # уровень масштабирования
  -e DEFAULT_LAYER="OpenStreetMap" или "Google Satellite"
```

#### Регулярный бэкап (раз в сутки)

Добавьте в `crontab -e`:

```bash
0 3 * * * docker exec chicha-isotope-map pg_dump -U chicha_isotope_map chicha_isotope_map | gzip > /backup/chicha_isotope_map_$(date +\%F).sql.gz
```

#### Восстановление из архива

```bash
docker exec -it chicha-isotope-map psql -U postgres -c "DROP DATABASE IF EXISTS chicha_isotope_map; CREATE DATABASE chicha_isotope_map OWNER chicha_isotope_map;"

zcat /backup/chicha_isotope_map_2025-07-24.sql.gz | docker exec -i chicha-isotope-map psql -U chicha_isotope_map chicha_isotope_map
```

---

### 2. Установка без Docker

Если вы не любите контейнеры, скачайте готовый бинарь и запустите за секунды, это еще проще!

> Выполняйте команды от **root** (`sudo -i` или `sudo ...`).

* **Linux x64**:
```
curl -L https://github.com/matveynator/chicha-isotope-map/releases/download/latest/chicha-isotope-map_linux_amd64 > /usr/local/bin/chicha-isotope-map && chmod +x /usr/local/bin/chicha-isotope-map && chicha-isotope-map
```

* **macOS Intel**:
```
curl -L https://github.com/matveynator/chicha-isotope-map/releases/download/latest/chicha-isotope-map_darwin_amd64 > /usr/local/bin/chicha-isotope-map && chmod +x /usr/local/bin/chicha-isotope-map && chicha-isotope-map
```

* **macOS Apple Silicon**:

```
curl -L https://github.com/matveynator/chicha-isotope-map/releases/download/latest/chicha-isotope-map_darwin_arm64 > /usr/local/bin/chicha-isotope-map && chmod +x /usr/local/bin/chicha-isotope-map && chicha-isotope-map
```

Остальные платформы  Windows / ARM / BSD можно скачать на странице релизов [https://github.com/matveynator/chicha-isotope-map/releases/tag/latest](https://github.com/matveynator/chicha-isotope-map/releases/tag/latest)


После запуска по умолчанию программа слушает порт 8765. Откройте [http://localhost:8765](http://localhost:8765).

---

## 🤝 Зачем свой узел?

* **Независимость:** данные хранятся у вас, не зависят от чьей-то сети.
* **Устойчивость сети:** чем больше узлов, тем сложнее её скомпрометировать.
* **Местная история фона:** сохраните карту радиации вашего региона на годы вперед.

Каждый ваш сервер — ещё один маяк информации. Спасибо, что делаете мир прозрачнее!


Карта изотопов Chicha была создана для Лаборатории радиационных исследований Дмитрия Игнатенко и вдохновлена японским сообществом Safecast — группой граждан‑учёных, которые превратили трагедию в знание. Ища, измеряя и распространяя правду о радиации, вы делаете невидимое — видимым, помогая миру не повторить Чернобыль и Фукусиму. Ваша работа — это свет науки, безопасности и надежды. Спасибо вам за то, что превращаете фоновое излучение из повода для страха в источник понимания, за то, что ищете, измеряете, делитесь — и с мужеством идёте первыми.
