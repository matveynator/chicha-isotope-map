> *Chicha Isotope Map* создан специально для **Лаборатории радиационных исследований Дмитрия Игнатенко** и вдохновлён японской инициативой [**Safecast**](https://map.safecast.org) — мировым сообществом граждан‑учёных, превративших кризис в знания. Исследуя, измеряя и делясь правдой о радиации, вы делаете невидимое видимым и помогаете тому, чтобы трагедии **Чернобыля** и **Фукусимы** больше не повторились. Ваша работа освещает путь науки, безопасности и надежды. Спасибо за то, что делаете невидимое видимым, где фон — это не страх, а источник знания. И за то, что ищете, меряете, делитесь и не боитесь идти первыми.  


### 🌌 **chicha-isotope-map** — исследователь невидимых путей радиации

> **«Увидеть невидимое». Эта программа визуализирует радиоактивные следы, превращая их в живую карту.**

---

## 📖 **О проекте**

**Chicha-Isotope-Map** показывает невидимый мир радиоактивных частиц. Под вашими ногами изотопы оставляют следы — их переносит ветер, машины или люди. Программа отображает их на карте: от зелёного (безопасно) до красного (опасно).

Она читает файлы форматов `.kml`, `.kmz`, `.json`, `.rctrk` (AtomFast и RadiaCode) и сохраняет данные в базе. Спустя годы можно будет посмотреть, как менялся радиационный фон.

---

### 🌍 **Вдохновлено природой**

Программа использует **естественный радиационный фон** как базовый уровень. В нетронутой природе он составляет **1–4 мкР/ч**. Всё, что выше — считается **радиоактивным загрязнением**. Chicha-Isotope-Map отслеживает такие аномалии, превращая невидимые следы в видимые сигналы.

---

### 📸 **Демонстрация вживую**

<a href="https://jutsa.ru" target="_blank">Посмотреть программу в действии можно здесь.</a>

---

### 📸 **Пример на карте**

В советское время в парке Кисловодска построили открытый бассейн. Бетон могли привезти с завода в Пятигорске, где перерабатывали радиоакционную руду с горы Бештау. Грузовики везли материалы, оставляя невидимую пыль на дорогах. Спустя десятилетия эти следы до сих пор видны на карте жёлтыми пятнами — как осенние листья. Остальная часть парка остаётся чистой, зелёной и спокойной. <img src="https://repository-images.githubusercontent.com/870016860/11fd6abc-fe8b-4cd8-95c2-df1c631c8762">

---

## 📥 **Скачать и начать работу** 📥

### Linux 64-bit amd64:

Примечание: установка от имени ROOT.

```bash
curl -L https://github.com/matveynator/chicha-isotope-map/releases/download/latest/chicha-isotope-map_linux_amd64 > /usr/local/bin/chicha-isotope-map; chmod +x /usr/local/bin/chicha-isotope-map; chicha-isotope-map --version;
```

### Mac OS X Intel:

Примечание: установка от имени ROOT.

```bash
curl -L https://github.com/matveynator/chicha-isotope-map/releases/download/latest/chicha-isotope-map_darwin_amd64 > /usr/local/bin/chicha-isotope-map; chmod +x /usr/local/bin/chicha-isotope-map; chicha-isotope-map --version;
```

### Mac OS X Apple Silicon:

Примечание: установка от имени ROOT.

```bash
curl -L https://github.com/matveynator/chicha-isotope-map/releases/download/latest/chicha-isotope-map_darwin_amd64 > /usr/local/bin/chicha-isotope-map; chmod +x /usr/local/bin/chicha-isotope-map; chicha-isotope-map --version;
```

[Скачать для других платформ: Linux, macOS, Windows, FreeBSD, OpenBSD, NetBSD](https://github.com/matveynator/chicha-isotope-map/releases/tag/latest)

---

## 🛠 **Как пользоваться?**

Запуск с настройками по умолчанию:

```bash
chicha-isotope-map
```

#### Чернобыль (1986) — паровой взрыв и пожар графита; массовое радиоактивное выпадение по всей Европе

```
./chicha-isotope-map -default-lat=51.389 -default-lon=30.099 -default-zoom=11 -default-layer="Google Satellite"
```

#### Фукусима‑Дайичи (2011) — цунами вывело из строя охлаждение; расплавление активной зоны и выбросы в море и атмосферу

```
./chicha-isotope-map -default-lat=37.421 -default-lon=141.033 -default-zoom=12 -default-layer="Google Satellite"
```

#### Кыштым / Маяк (1957) — взрыв ёмкости с отходами; радиоактивный шлейф над Уралом

```
./chicha-isotope-map -default-lat=55.700 -default-lon=60.800 -default-zoom=9 -default-layer="Google Satellite"
```

#### Три‑Майл‑Айленд (1979) — частичное расплавление активной зоны; ограниченный выброс за пределы площадки

```
./chicha-isotope-map -default-lat=40.153 -default-lon=-76.723 -default-zoom=12 -default-layer="Google Satellite"
```

#### Уиндскейл (1957) — пожар графитового реактора; выброс йода‑131 над Великобританией

```
./chicha-isotope-map -default-lat=54.432 -default-lon=-3.553 -default-zoom=12 -default-layer="Google Satellite"
```

#### Гояния (1987) — вскрытый бесхозный источник Cs‑137; загрязнение всего города

```
./chicha-isotope-map -default-lat=-16.686 -default-lon=-49.264 -default-zoom=13 -default-layer="Google Satellite"
```

#### Пятигорск, гора Бештау (1940‑е‑50‑е) — первые урановые рудники для атомной бомбы СССР; загрязнение на уровне всего района

```
./chicha-isotope-map -default-lat=44.089 -default-lon=42.976 -default-zoom=11 -default-layer="Google Satellite"
```

Запуск с параметрами:

```bash
chicha-isotope-map --port=8765 --db-type=pgx --db-host=localhost --db-port=5432 --db-user=postgres --db-pass=yourpassword --db-name=isotope_db --pg-ssl-mode=prefer
```

### Пример подключения к PostgreSQL (`pgx` драйвер):

```bash
chicha-isotope-map --port=8765 --db-type=pgx --db-host=localhost --db-port=5432 --db-user=postgres --db-pass=my_secure_password --db-name=radiation_data --pg-ssl-mode=require
```

Эта конфигурация подключается к базе данных `radiation_data` на локальной машине. Замените `my_secure_password` на свой пароль. При необходимости измените имя базы, хост или порт.

---

### Веб-интерфейс

1. Откройте [http://localhost:8765](http://localhost:8765) в браузере.
2. Нажмите кнопку **Upload**, чтобы загрузить файлы с данными.
3. Исследуйте карту: наведите курсор на маркеры, чтобы увидеть уровень радиации, время и координаты.

---

## ☢️ **Зачем это нужно**

Радиация невидима, но опасна. Она накапливается в почве, воде, растениях. Эта программа помогает увидеть, где распространилось заражение. Она делает невидимое видимым — чтобы вы могли понять и действовать.

---

> **«Если бы изотопы умели рассказывать свои истории — эта программа не понадобилась бы. Но раз не умеют, за них говорит Chicha-Isotope-Map.»**

