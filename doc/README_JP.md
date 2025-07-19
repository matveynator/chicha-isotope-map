> *Chicha Isotope Map* は **ドミトリー・イグナチェンコ放射線研究所** のために開発され、日本発の市民科学コミュニティ [Safecast](https://map.safecast.org) に深くインスパイアされています。放射線を「探し・測り・共有」するあなた方の行動は、見えないものを可視化し、**チェルノブイリ** や **福島** のような悲劇を二度と繰り返させない未来を切り拓きます。あなた方の挑戦は、科学と安全、そして希望への道を照らします。  
> 見えないものを可視化し、バックグラウンド放射線を恐れではなく知識の源へと変えてくれてありがとう。そして、探し、測り、共有し、勇敢に先頭に立ってくれる皆さんに感謝します。  

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

#### チェルノブイリ（1986年）— 蒸気爆発と黒鉛火災；ヨーロッパ全域に大規模な放射性降下物

```
./chicha-isotope-map -default-lat=51.389 -default-lon=30.099 -default-zoom=11 -default-layer="Google Satellite"
```

#### 福島第一（2011年）— 津波で冷却機能が喪失；炉心溶融し海と大気へ放出

```
./chicha-isotope-map -default-lat=37.421 -default-lon=141.033 -default-zoom=12 -default-layer="Google Satellite"
```

#### キシュティム／マヤーク（1957年）— 廃液タンク爆発；ウラル山脈上空に放射性プルーム

```
./chicha-isotope-map -default-lat=55.700 -default-lon=60.800 -default-zoom=9 -default-layer="Google Satellite"
```

#### スリーマイル島（1979年）— 炉心部分溶融；場外への放出は限定的

```
./chicha-isotope-map -default-lat=40.153 -default-lon=-76.723 -default-zoom=12 -default-layer="Google Satellite"
```

#### ウィンドスケール（1957年）— 黒鉛炉火災；イギリス上空にヨウ素131放出

```
./chicha-isotope-map -default-lat=54.432 -default-lon=-3.553 -default-zoom=12 -default-layer="Google Satellite"
```

#### ゴイアニア（1987年）— 孤立したCs‑137線源が開封；都市全域が汚染

```
./chicha-isotope-map -default-lat=-16.686 -default-lon=-49.264 -default-zoom=13 -default-layer="Google Satellite"
```

#### ピャチゴルスク、ベシュタウ山（1940〜50年代）— ソ連初の原爆用ウラン鉱山；地区全域に汚染

```
./chicha-isotope-map -default-lat=44.089 -default-lon=42.976 -default-zoom=11 -default-layer="Google Satellite"
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
