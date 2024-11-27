# 🌌 **Isotope Pathways** — 見えない道の追跡者

> **「この世界に、見えないものを見ることができる人はいるだろうか？ いない？ でもこのプログラムはできるんだ。古いシャーマンが灰を読んで未来を見通すように、放射性の痕跡を見つけ出し、それを画面に浮かび上がらせる。色鮮やかで、きらめいて、生き生きとしている。」**

---

## 📖 **プロジェクトについて**

**Isotope Pathways** は、ただのプログラムではありません。見えない粒子たちの世界を、あなたの目の前に映し出します。想像してみてください。道を歩くあなたの足元には、放射性同位体が舞い踊っている。そしてこのプログラムが、その姿を見せてくれます。緑から赤まで、静かなものから不安なものまで、同位体たちの痕跡を地図上に描き出します。

AtomFast や RadiaCode の形式（`.kml`, `.kmz`, `.json`, `.rctrk`）のデータを読み込み、データベースに保存し、何年も後に「2024年にはここで4.1 µR/h だった」と言えるようにします。

### 🌍 **自然に基づく**

私たちは **自然の放射線レベル** を基準にしました。手つかずの自然の場所に行けば、おそらく **毎時3〜4マイクロレントゲン** 程度を見ることになるでしょう。これが通常の放射線レベルです。場所によって放射線量は異なり、地球はそれを受け入れています。

このレベルを超えるものは **異物** です。私たちはそれを **放射性汚染** と呼びます。道端に散らばる同位体、風や人、交通によって運ばれたもの。それらは、地図上で見えない痕跡として残されます。まるで新雪に残るブーツの足跡のように。

---

### 📸 **デモ**

<a href="https://jutsa.ru" target="_blank">こちらからリアルタイムでプログラムの動作を見ることができます。</a>

### 📸 **スクリーンショット**

... ソビエト時代、キスロボーツク公園では露天プールが建設されていました。おそらくピャチゴルスクの工場からコンクリートを調達していたのでしょう。その工場はかつてベシュタウ山の放射性鉱石を加工していました。トラックはその道を行き来し、そのタイヤからの粉塵がアスファルトに落ち、目には見えない印を残しました。年月が経ちましたが、その痕跡は今でも残っており、過去の記憶のように輝いています。建設現場の周囲に広がった粉塵は、公園に定着し、地図上では黄色で表されます。まるで秋の葉の斑点のように。それ以外の公園は、清潔で穏やかで、緑に満ちています。
<img src="https://repository-images.githubusercontent.com/870016860/11fd6abc-fe8b-4cd8-95c2-df1c631c8762">

---

## 📥 **ダウンロードして始める** 📥

お使いのプラットフォームのバージョンを選んで、同位体の痕跡を追い始めましょう：

| プラットフォーム  | ダウンロードリンク                                                                                  |
|------------|--------------------------------------------------------------------------------------------------------|
| AIX        | [AIX 用ダウンロード](http://files.zabiyaka.net/chicha-isotope-map/latest/no-gui/aix/)                     |
| Android    | [Android 用ダウンロード](http://files.zabiyaka.net/chicha-isotope-map/latest/no-gui/android/)              |
| Dragonfly  | [Dragonfly 用ダウンロード](http://files.zabiyaka.net/chicha-isotope-map/latest/no-gui/dragonfly/)          |
| FreeBSD    | [FreeBSD 用ダウンロード](http://files.zabiyaka.net/chicha-isotope-map/latest/no-gui/freebsd/)              |
| Illumos    | [Illumos 用ダウンロード](http://files.zabiyaka.net/chicha-isotope-map/latest/no-gui/illumos/)              |
| JavaScript | [JavaScript 用ダウンロード](http://files.zabiyaka.net/chicha-isotope-map/latest/no-gui/js/)                |
| Linux      | [Linux 用ダウンロード](http://files.zabiyaka.net/chicha-isotope-map/latest/no-gui/linux/)                  |
| macOS      | [macOS 用ダウンロード](http://files.zabiyaka.net/chicha-isotope-map/latest/no-gui/mac/)                    |
| NetBSD     | [NetBSD 用ダウンロード](http://files.zabiyaka.net/chicha-isotope-map/latest/no-gui/netbsd/)                |
| OpenBSD    | [OpenBSD 用ダウンロード](http://files.zabiyaka.net/chicha-isotope-map/latest/no-gui/openbsd/)              |
| Plan9      | [Plan9 用ダウンロード](http://files.zabiyaka.net/chicha-isotope-map/latest/no-gui/plan9/)                  |
| Solaris    | [Solaris 用ダウンロード](http://files.zabiyaka.net/chicha-isotope-map/latest/no-gui/solaris/)              |
| Windows    | [Windows 用ダウンロード](http://files.zabiyaka.net/chicha-isotope-map/latest/no-gui/windows/)              |

もしくは、自分でビルドしてみてください：

```bash
git clone https://github.com/matveynator/chicha-isotope-map.git
cd chicha-isotope-map
go build chicha-isotope-map.go
chmod +x ./chicha-isotope-map
./chicha-isotope-map
```

---

## 🛠 **使い方**

### プログラムを実行する:

```bash
./chicha-isotope-map
```

または追加設定付きで実行する:

```bash
./chicha-isotope-map --port=8765 --db-type=genji --db-path=./path-to-database-file.8765.genji
```

#### サポートされているデータベースタイプ:
- `genji`: 軽量で高速な組み込みデータベース。外部依存はありません。
- `sqlite`: ローカルストレージに人気のファイルベースデータベース。
- `pgx` (PostgreSQL): `pgx`ドライバを使用してPostgreSQLサーバに接続。

#### PostgreSQLの例:

```bash
./chicha-isotope-map --port=8765 --db-type=pgx --db-host=localhost --db-port=5432 --db-user=postgres --db-pass=あなたのパスワード --db-name=isotope_db --pg-ssl-mode=prefer
```

- `--db-type`: データベースのタイプ (`genji`, `sqlite` または `pgx`)。デフォルトは`genji`です。
  - **pgx**: `pgx`ドライバを使用してPostgreSQLを利用します。
- `--db-host`: PostgreSQLデータベースのホスト。デフォルトは`127.0.0.1`です。
- `--db-port`: PostgreSQLのポート。デフォルトは`5432`です。
- `--db-user`: PostgreSQLのユーザー名。デフォルトは`postgres`です。
- `--db-pass`: PostgreSQLのパスワード。
- `--db-name`: PostgreSQLデータベースの名前。デフォルトは`isotope_db`です。
- `--pg-ssl-mode`: PostgreSQL用のSSLモード。デフォルトは`prefer`です。

_デフォルトの設定を使用してカスタムデータベースでPostgreSQLを実行する例:_

```bash
./chicha-isotope-map --db-type=pgx --db-name=私のデータベース
```

これにより、ドライバ`pgx`を使用して、`localhost:5432`でユーザー`postgres`のパスワードなしで、`私のデータベース`という名前のPostgreSQLデータベースに接続します。


### ウェブインターフェース:

1. <a href="http://localhost:8765" target="new">http://localhost:8765</a> をブラウザで開きます。
2. 「アップロード」ボタンを使ってデータを読み込みます。
3. マーカーにカーソルを合わせると、見えない世界が目の前に広がります。放射線量、測定時間、同位体が痕跡を残した場所を確認してください。

---

## ☢️ **放射線とその痕跡**

放射線とは何か？ それは山々を通り抜ける風の囁きのようなものです。誰も聞こえませんが、確かにそこに存在します。このプログラムは、普通の人には聞こえないその囁きを聞き取ることができる存在です。それは、どこで、いつ、余分なマイクロレントゲンがあったのかを教えてくれます。同位体がどのように街に飛び散り、静かな池に落ちたり、古い森に迷い込んだりしたかを見せてくれます。その危険性は、ただ地面に横たわる忘れられたコインのようなものではありません。それは土壌、水、

植物に浸透します。静かに暮らしているうちに、あなたはリンゴを食べ、井戸水を飲み、同位体は知らず知らずのうちに体内に侵入していきます。放射線量は、まるで小さなウサギのように、体の中で静かに積み重なっていくのです。しかし、それでもなお危険です。

---

> **「もし同位体たちが話すことができたら、彼らは自分たちの物語を語ったでしょう。でも今のところ彼らは黙っています。その代わりに、このプログラムがその物語を語ってくれます。」**
