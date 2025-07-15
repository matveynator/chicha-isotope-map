「Chicha Isotope Map」は、ドミトリー・イグナテンコ氏の放射線研究所のために特別に開発されました。背景放射線を恐れではなく知識の源として見つめ、見えないものを見えるようにしてくれてありがとうございます。そして、探し、測り、共有し、最初に歩むことを恐れないあなた方に感謝します。

### 🌌 **chicha-isotope-map** — 放射線の隠れた経路をたどる探査ツール

> **「見えないものを見る」このプログラムは放射性の痕跡を視覚化し、見えない軌跡を鮮やかな地図に変えます。**

---

## 📖 **プロジェクトについて**

**Chicha-Isotope-Map** は、放射性粒子が作る見えない世界を可視化するツールです。地面の下を通る放射性同位体の痕跡は、風、車、人によって運ばれ、地図上に表示されます。緑（安全）から赤（危険）まで、痕跡の強さによって色分けされます。

`.kml`, `.kmz`, `.json`, `.rctrk`（AtomFast や RadiaCode）形式のデータを読み込み、データベースに保存します。何年も後に、放射線レベルの変化を振り返ることができます。

---

### 🌍 **自然に着想を得て**

プログラムは**自然放射線レベル**を基準にしています。手つかずの自然では、通常の放射線量は **1〜4 µR/h** です。これを超える値は**放射性汚染**とみなされます。Chicha-Isotope-Map はこのような異常を検出し、見えない足跡を可視化します。

---

### 📸 **ライブデモ**

<a href="https://jutsa.ru" target="_blank">こちらから実際の動作を確認できます。</a>

---

### 📸 **ビジュアル例**

ソ連時代、キスロヴォツク公園には屋外プールが建設されました。コンクリートは、ベシュタウ山の放射性鉱石を加工していたピャチゴルスクの工場から運ばれた可能性があります。トラックが材料を運ぶ過程で、道路に放射性の粉塵が目に見えない形で残りました。数十年後も、それらの痕跡は地図上に黄色の点として現れます——まるで秋の落ち葉のように。公園のその他の部分は、今も清潔で静か、そして緑に包まれています。 <img src="https://repository-images.githubusercontent.com/870016860/11fd6abc-fe8b-4cd8-95c2-df1c631c8762">

---

## 📥 **ダウンロードと開始方法** 📥

### Linux 64-bit amd64：

※ROOTユーザーとしてインストールしてください。

```bash
curl -L https://github.com/matveynator/chicha-isotope-map/releases/download/latest/chicha-isotope-map_linux_amd64 > /usr/local/bin/chicha-isotope-map; chmod +x /usr/local/bin/chicha-isotope-map; chicha-isotope-map --version;
```

### Mac OS X Intel：

※ROOTユーザーとしてインストールしてください。

```bash
curl -L https://github.com/matveynator/chicha-isotope-map/releases/download/latest/chicha-isotope-map_darwin_amd64 > /usr/local/bin/chicha-isotope-map; chmod +x /usr/local/bin/chicha-isotope-map; chicha-isotope-map --version;
```

### Mac OS X Apple Silicon：

※ROOTユーザーとしてインストールしてください。

```bash
curl -L https://github.com/matveynator/chicha-isotope-map/releases/download/latest/chicha-isotope-map_darwin_amd64 > /usr/local/bin/chicha-isotope-map; chmod +x /usr/local/bin/chicha-isotope-map; chicha-isotope-map --version;
```

[他のプラットフォーム向け（Linux, macOS, Windows, FreeBSD, OpenBSD, NetBSD）はこちら](https://github.com/matveynator/chicha-isotope-map/releases/tag/latest)

---

## 🛠 **使い方**

デフォルト設定で起動するには：

```bash
chicha-isotope-map
```

オプション付きで起動するには：

```bash
chicha-isotope-map --port=8765 --db-type=pgx --db-host=localhost --db-port=5432 --db-user=postgres --db-pass=yourpassword --db-name=isotope_db --pg-ssl-mode=prefer
```

### PostgreSQL 接続例（`pgx`ドライバ）：

```bash
chicha-isotope-map --port=8765 --db-type=pgx --db-host=localhost --db-port=5432 --db-user=postgres --db-pass=my_secure_password --db-name=radiation_data --pg-ssl-mode=require
```

この構成では、ローカルマシン上の `radiation_data` という PostgreSQL データベースに接続します。パスワードやホスト名は適宜変更してください。

---

### Webインターフェース

1. ブラウザで [http://localhost:8765](http://localhost:8765) を開きます。
2. **Upload** ボタンをクリックして、データファイルをアップロードします。
3. 地図上のマーカーにマウスを乗せると、放射線量、タイムスタンプ、位置情報が表示されます。

---

## ☢️ **なぜ重要なのか**

放射線は見えませんが、危険です。土壌、水、植物に入り込み、蓄積していきます。このプログラムは、汚染がどこまで広がっているかを可視化し、理解と行動につなげます。

---

> **「もしも同位体が自分の物語を語れたなら、このプログラムは要らなかったでしょう。でも語れないからこそ、Chicha-Isotope-Map がその声になるのです。」**
