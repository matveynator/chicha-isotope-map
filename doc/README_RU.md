# 🌌 Chicha Isotope Map — собственный узел за 5 минут

> *Делаем невидимое видимым — каждый на своём сервере или ноуте.*
> **Спасибо всем, кто запустит свой узел — вместе мы усиливаем «сетевой дозиметр»!**

---

## 1 · Быстрый старт в Docker


### Локальный порт (5000)      
```
docker run -d --name chicha-isotope-map -e PORT=5000 -p 5000:5000 -v isotope-data:/var/lib/postgresql/data matveynator/chicha-isotope-map:latest
```
 
### Домен + HTTPS (`domain.com`) 
```
docker run -d --name chicha-isotope-map -e DOMAIN=domain.com  -p 80:80 -p 443:443 -v isotope-data:/var/lib/postgresql/data matveynator/chicha-isotope-map:latest
``` 

### Опции карты

```text
-e DEFAULT_LAT=51.389   # широта центра
-e DEFAULT_LON=30.099   # долгота
-e DEFAULT_ZOOM=11      # стартовый zoom
-e DEFAULT_LAYER="OpenStreetMap" | "Google Satellite"
```

### Бэкап / крон (раз в сутки)

```bash
# crontab -e
0 3 * * * docker exec isotope-map pg_dump -U chicha_isotope_map chicha_isotope_map | gzip > /backup/chicha_isotope_map_$(date +\%F).sql.gz 
```

### Восстановление из бэкапа

```bash
docker exec -it chicha-isotope-map psql -U postgres -c "DROP DATABASE IF EXISTS chicha_isotope_map; CREATE DATABASE chicha_isotope_map OWNER chicha_isotope_map;"

zcat /backup/chicha_isotope_map_2025-07-24.sql.gz | docker exec -i chicha-isotope-map psql -U chicha_isotope_map chicha_isotope_map
```

---

## 2 📦 Установка без Docker (нужно скачать всего один бинарь)

> **Скачайте → сделайте исполняемым → запустите**.
> Выполняйте команды **от root** (`sudo -i` либо добавьте `sudo` перед каждой).


### Linux 64-bit amd6  
```
curl -L https://github.com/matveynator/chicha-isotope-map/releases/download/latest/chicha-isotope-map_linux_amd64  > /usr/local/bin/chicha-isotope-map && chmod +x /usr/local/bin/chicha-isotope-map && chicha-isotope-map
```

### macOS Intel      
```
curl -L https://github.com/matveynator/chicha-isotope-map/releases/download/latest/chicha-isotope-map_darwin_amd64 > /usr/local/bin/chicha-isotope-map && chmod +x /usr/local/bin/chicha-isotope-map && chicha-isotope-map
```

### macOS Apple Silicon
```
curl -L https://github.com/matveynator/chicha-isotope-map/releases/download/latest/chicha-isotope-map_darwin_arm64 > /usr/local/bin/chicha-isotope-map && chmod +x /usr/local/bin/chicha-isotope-map && chicha-isotope-map
```

### Другие платформы

См. список файлов на странице релиза:
[https://github.com/matveynator/chicha-isotope-map/releases/tag/latest](https://github.com/matveynator/chicha-isotope-map/releases/tag/latest) 
(Linux ARM, Windows \*.exe, FreeBSD, OpenBSD, NetBSD и др.) 

После установки запустите, например:

```bash
# локально на 5000-м порту
chicha-isotope-map -port=5000
# либо с кастомной стартовой точкой
chicha-isotope-map -default-lat=51.389 -default-lon=30.099 -default-zoom=11
```

Откройте браузер: [http://localhost:5000](http://localhost:5000) (или указанный порт).
База Genji лежит рядом с бинарём (`database-*.genji`) — копия файла = бэкап. 
Можно подключить любую базу по желанию - подробнее --help.

---

## 3 · Зачем нужен собственный узел?

* **Независимость** — ваши данные у вас: интернет-штормы не страшны.
* **Коллективная устойчивость** — сеть узлов труднее «выключить» или «подменить».
* **История фона в вашем регионе** — сегодня, завтра, через 10 лет.

**Каждый запущенный сервер = ещё один «датчик правды». Спасибо, что делаете мир чуть прозрачнее!**
