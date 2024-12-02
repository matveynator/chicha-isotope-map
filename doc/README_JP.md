### 🌌 **chicha-isotope-map** — 放射線の見えない道を探る探検家

> **「見えないものを見られる人はいるでしょうか？いない？このプログラムはそれができます。放射性物質の痕跡を捉え、まるでシャーマンが灰を読み取るように、鮮やかで光り輝く形で画面に表示します。」**

---

## 📖 **プロジェクトについて**

**chicha-isotope-map**は、見えない放射性粒子の世界を明らかにします。足元の地面では、風や車、人によって運ばれた放射性同位体が痕跡を残しています。このプログラムはそれを地図上で可視化し、緑（安全）から赤（危険）まで、色でその痕跡を示します。

このプログラムは、`.kml`、`.kmz`、`.json`、および`.rctrk`（AtomFastやRadiaCodeのデータ形式）からデータを読み取り、データベースに保存します。数年後には、過去の放射線レベルの変化を振り返ることができます。

---

### 🌍 **自然に基づいた設計**

プログラムは、**自然放射線レベル**を基準としています。手つかずの地域では、通常、放射線レベルは**1～4 µR/h**程度です。これを超えるものは**放射性汚染**とみなされます。このプログラムは、これらの異常を追跡し、見えない痕跡を警告として可視化します。

---

### 📸 **ライブデモ**

<a href="https://jutsa.ru" target="_blank">こちらでプログラムの動作をご覧ください。</a>

---

### 📸 **ビジュアル例**

ソビエト時代、キスロヴォツク公園に野外プールが建設されました。このコンクリートは、かつてベシュタウ山の放射性鉱石を加工していたピャチゴルスクの工場から運ばれた可能性があります。材料を運んだトラックは、道路に見えない放射性の埃を残しました。数十年後、この痕跡は地図上で黄色い印として現れます—まるで秋の葉の斑点のように。一方、公園の他の部分は清潔で穏やかで緑豊かです。
<img src="https://repository-images.githubusercontent.com/870016860/11fd6abc-fe8b-4cd8-95c2-df1c631c8762">

---

## 📥 **ダウンロードと始め方** 📥

Linux 64-bit x86でのインストール:  
```bash
sudo curl https://files.zabiyaka.net/chicha-isotope-map/latest/no-gui/linux/amd64/chicha-isotope-map > /usr/local/bin/chicha-isotope-map; sudo chmod +x /usr/local/bin/chicha-isotope-map; chicha-isotope-map -v;
```

お使いのプラットフォームを選んで放射性同位体の痕跡を探索しましょう:

| プラットフォーム  | ダウンロードリンク                                                                                            |
|------------|--------------------------------------------------------------------------------------------------------|
| AIX        | [AIX用ダウンロード](http://files.zabiyaka.net/chicha-isotope-map/latest/no-gui/aix/)                      |
| Android    | [Android用ダウンロード](http://files.zabiyaka.net/chicha-isotope-map/latest/no-gui/android/)               |
| Dragonfly  | [Dragonfly用ダウンロード](http://files.zabiyaka.net/chicha-isotope-map/latest/no-gui/dragonfly/)           |
| FreeBSD    | [FreeBSD用ダウンロード](http://files.zabiyaka.net/chicha-isotope-map/latest/no-gui/freebsd/)               |
| Illumos    | [Illumos用ダウンロード](http://files.zabiyaka.net/chicha-isotope-map/latest/no-gui/illumos/)               |
| JavaScript | [JavaScript用ダウンロード](http://files.zabiyaka.net/chicha-isotope-map/latest/no-gui/js/)                 |
| Linux      | [Linux用ダウンロード](http://files.zabiyaka.net/chicha-isotope-map/latest/no-gui/linux/)                   |
| macOS      | [macOS用ダウンロード](http://files.zabiyaka.net/chicha-isotope-map/latest/no-gui/mac/)                     |
| NetBSD     | [NetBSD用ダウンロード](http://files.zabiyaka.net/chicha-isotope-map/latest/no-gui/netbsd/)                 |
| OpenBSD    | [OpenBSD用ダウンロード](http://files.zabiyaka.net/chicha-isotope-map/latest/no-gui/openbsd/)               |
| Plan9      | [Plan9用ダウンロード](http://files.zabiyaka.net/chicha-isotope-map/latest/no-gui/plan9/)                   |
| Solaris    | [Solaris用ダウンロード](http://files.zabiyaka.net/chicha-isotope-map/latest/no-gui/solaris/)               |
| Windows    | [Windows用ダウンロード](http://files.zabiyaka.net/chicha-isotope-map/latest/no-gui/windows/)               |

---

## 🛠 **使い方**

デフォルト設定でプログラムを実行する:
```bash
chicha-isotope-map
```

または、追加オプションを使用:
```bash
chicha-isotope-map --port=8765 --db-type=pgx --db-host=localhost --db-port=5432 --db-user=postgres --db-pass=yourpassword --db-name=isotope_db --pg-ssl-mode=prefer
```

---

### Webインターフェース:

1. ブラウザで [http://localhost:8765](http://localhost:8765) を開く。
2. **Upload**ボタンを使ってデータファイルをアップロードする。
3. 地図を探索: マーカーにカーソルを合わせると、放射線レベル、測定時間、位置が表示されます。

---

## ☢️ **なぜ重要なのか**

放射線は見えませんが、非常に危険です。それは地面に留まるだけでなく、土壌、水、植物に浸透し、時間とともに蓄積します。このプログラムは、汚染がどこに広がったのかを可視化し、見えないものを見える形にして理解を助けます。

---

> **「もし同位体が自分たちの話を語れるなら、このプログラムは不要でしょう。しかし、彼らは語れないので、chicha-isotope-mapが代わりにその物語を語ります。」**
