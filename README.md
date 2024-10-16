
# 🌌 **Isotope Pathways** — следопыт невидимых дорог

> **"Есть ли на свете кто-то, кто умеет видеть невидимое? Нет? А программа умеет. Она берёт радиоактивные следы, как старый шаман читает по пеплу, и выводит их на экран — цветные, мерцающие, живые."**

---

## 📖 **О проекте**

**Isotope Pathways** — это не просто программа, это целый мир невидимых частиц, которые теперь становятся видимыми. Представьте себе: идёте по дороге, а под вашими ногами танцуют радиоактивные изотопы. И эта программа умеет их показывать. Она рисует карту, где каждый изотоп оставляет свой след, от зелёного до красного, от спокойствия до тревоги.

Она может читать данные из файлов форматов AtomFast и RadiaCode `.kml`, `.kmz` и `.rctrk`, хранить их в своей базе данных, чтобы потом, через много лет, вы могли бы сказать: «А вот тогда, в 2024 году, здесь было 4.1 µR/h».


### 🌍 **Исходя из природы**

Мы взяли за основу **природный фон радиации**. Если вы отправитесь в чистое, нетронутое место, там вы, скорее всего, увидите **3-4 микрорентгена в час**. Это — норма. На любой высоте свой уровень радиации, и планета этому радуется.

Все, что выше этого фона — **чужеродное**. Это мы называем **радиоактивным загрязнением**. Здесь видно, как изотопы разбросаны по дорогам, унесены ветром, людьми и транспортом. И вот они, маленькие невидимые следы на карте, как следы сапог по свежевыпавшему снегу.


---

### 📸 **Скриншоты**

... В советское время в Кисловодском парке строили открытый бассейн. Возможно, взяли бетон с завода в Пятигорске, где когда-то перерабатывали радиоактивную руду с горы Бештау. И вот, по дороге ездили грузовики, пыль с их колес оседала на асфальте, оставляя свои невидимые метки. Прошли годы, а эти следы всё ещё светятся, словно воспоминания о былом. Пыль, разлетевшаяся вокруг стройки, осела в парке — на карте она жёлтым цветом, словно пятна осенних листьев. Всё остальное в парке — чистое, спокойное, зелёное.
<img src="https://repository-images.githubusercontent.com/870016860/11fd6abc-fe8b-4cd8-95c2-df1c631c8762">

---

## 📥 **Скачать и начать** 📥

Выберите версию для своей платформы и начните следить за следами изотопов:

| Платформа  | Ссылка для скачивания                                                                                 |
|------------|-------------------------------------------------------------------------------------------------------|
| AIX        | [Скачать для AIX](http://files.zabiyaka.net/isotope-pathways/latest/no-gui/aix/)                      |
| Android    | [Скачать для Android](http://files.zabiyaka.net/isotope-pathways/latest/no-gui/android/)               |
| Dragonfly  | [Скачать для Dragonfly](http://files.zabiyaka.net/isotope-pathways/latest/no-gui/dragonfly/)           |
| FreeBSD    | [Скачать для FreeBSD](http://files.zabiyaka.net/isotope-pathways/latest/no-gui/freebsd/)               |
| Illumos    | [Скачать для Illumos](http://files.zabiyaka.net/isotope-pathways/latest/no-gui/illumos/)               |
| JavaScript | [Скачать для JavaScript](http://files.zabiyaka.net/isotope-pathways/latest/no-gui/js/)                 |
| Linux      | [Скачать для Linux](http://files.zabiyaka.net/isotope-pathways/latest/no-gui/linux/)                   |
| macOS      | [Скачать для macOS](http://files.zabiyaka.net/isotope-pathways/latest/no-gui/mac/)                     |
| NetBSD     | [Скачать для NetBSD](http://files.zabiyaka.net/isotope-pathways/latest/no-gui/netbsd/)                 |
| OpenBSD    | [Скачать для OpenBSD](http://files.zabiyaka.net/isotope-pathways/latest/no-gui/openbsd/)               |
| Plan9      | [Скачать для Plan9](http://files.zabiyaka.net/isotope-pathways/latest/no-gui/plan9/)                   |
| Solaris    | [Скачать для Solaris](http://files.zabiyaka.net/isotope-pathways/latest/no-gui/solaris/)               |
| Windows    | [Скачать для Windows](http://files.zabiyaka.net/isotope-pathways/latest/no-gui/windows/)               |

Или создайте её своими руками:

```bash
git clone https://github.com/matveynator/isotope-pathways.git
cd isotope-pathways
go build -ldflags "-X isotope-pathways/pkg/config.CompileVersion=1.0.0" -o isotope-pathways.go main.go
./isotope-pathways.go
```

---

## 🛠 **Как пользоваться?**

### Запуск программы:

```bash
go run isotope-pathways.go --port=8765 --db-type=genji --db-path=./path-to-database-file.8765.genji
```

- `--port`: Порт, на котором будет запущен сервер. По умолчанию `8765`.  
  _"Хотите говорить с программой на другом языке? Измените порт, и она заговорит с вами по-новому."_

- `--db-type`: Тип базы данных: `genji` или `sqlite`. По умолчанию `genji`.  
  _"Предпочитаете старую школу с SQLite или новаторский дух с Genji? Выбирайте."_

- `--db-path`: Путь к файлу базы данных. Если не указан, используется файл в текущей директории.  
  _"Где прятать следы? В каком углу диска? Укажите путь, и он станет их домом."_

- `--version`: Показать версию программы и завершить выполнение.  
  _"Как старый фокусник, программа расскажет вам, из какой эпохи она пришла."_

### Веб-интерфейс:

1. Откройте `<a href="http://localhost:8765" target="new">http://localhost:8765</a>` в вашем браузере.
2. Загрузите данные, используя кнопку `Upload`.
3. Наведите курсор на маркер — и перед вами откроется мир невидимого. Узнайте дозу радиации, время измерения и место, где изотопы решили оставить свои метки.

---

## ☢️ **Радиация и её следы**

Что такое радиация? Это как шёпот ветра в горах, который никто не слышит, но он там. А наша программа — это человек с необычным слухом. Она видит то, что вы не видите. Она расскажет вам, где и когда был тот самый лишний микрорентген. Она покажет, как изотопы разлетелись по городу, как они упали в тихий пруд или затерялись в старом лесу. Опасность их в том, что они не просто лежат на земле, как забытая монетка. Нет, они проникают в почву, воду, растения. Живешь себе спокойно, ешь яблочки, пьешь воду из колодца, а изотопы незаметно пробираются внутрь. И дозы растут, как маленькие зайчики, накапливаясь в вашем теле. Незаметно, тихо. Но все равно опасно. 

---

> **"Если бы изотопы могли говорить, они бы рассказывали вам свои истории. Но пока они молчат, наша программа за них всё расскажет."**

