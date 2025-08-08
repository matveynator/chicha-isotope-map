- [🇬🇧 English](/README.md)
- [🇫🇷 Français](/doc/README_FR.md)
- [🇯🇵 日本語](/doc/README_JP.md)
- [🇷🇺 Русский](/doc/README_RU.md)


# ☢️ Chicha‑Isotope‑Map — 個人向けの放射線マップ

---

## 🚀 インストールと自前ノード：コマンド2つで完了

余計なものは一切なし。イメージにデータベース（PostgreSQL）同梱。コマンドをコピーして実行すれば、すぐ使えます。

#### 🔥 ローカル（ポート 8765）

```bash
docker run -d \
  --name chicha-isotope-map \
  -p 8765:8765 \
  -v chicha-data:/var/lib/postgresql/data \
  -e DEFAULT_LAT=44.08832 \
  -e DEFAULT_LON=42.97577 \
  -e DEFAULT_ZOOM=11 \
  -e DEFAULT_LAYER="OpenStreetMap" \
  --restart unless-stopped \
  matveynator/chicha-isotope-map:latest
```

開く: [http://localhost:8765](http://localhost:8765)

#### 🔥 自分のドメインで HTTPS の公開ノード

```bash
docker run -d \
  --name chicha-isotope-map \
  -p 80:80 -p 443:443 \
  -v chicha-data:/var/lib/postgresql/data \
  -e DOMAIN=example.org \
  -e DEFAULT_LAT=44.08832 \
  -e DEFAULT_LON=42.97577 \
  -e DEFAULT_ZOOM=11 \
  -e DEFAULT_LAYER="OpenStreetMap" \
  --restart unless-stopped \
  matveynator/chicha-isotope-map:latest
```

証明書発行後、こちらへアクセスしてください: [https://example.org](https://example.org)

---

### ⚙️ 環境変数での設定（必要最小限）

* `DOMAIN` — 公開ノード用。80/443 で HTTPS が有効になり、Let’s Encrypt による自動証明書発行を行います。
* `DEFAULT_LAT`, `DEFAULT_LON` — マップの初期座標。
* `DEFAULT_ZOOM` — 初期ズーム（11 は都市レベルで見やすい設定）。
* `DEFAULT_LAYER` — `OpenStreetMap` または `Google Satellite`。
* `PORT` — アプリ内部のポート（既定は 8765。通常は変更不要）。

> ヒント：データは `-v chicha-data:/var/lib/postgresql/data` のボリュームに置いておくと、コンテナ更新後も保持されます。

---

### 💾 バックアップと復元（かんたん）

**毎日のバックアップ（03:00）:**

```bash
0 3 * * * docker exec chicha-isotope-map pg_dump -U chicha_isotope_map chicha_isotope_map | gzip > /backup/chicha_isotope_map_$(date +\%F).sql.gz
```

**アーカイブからの復元:**

```bash
docker exec -it chicha-isotope-map psql -U postgres -c "DROP DATABASE IF EXISTS chicha_isotope_map; CREATE DATABASE chicha_isotope_map OWNER chicha_isotope_map;"

zcat /backup/chicha_isotope_map_2025-07-24.sql.gz | docker exec -i chicha-isotope-map psql -U chicha_isotope_map chicha_isotope_map
```

---

## ⬇️ 実行ファイルのダウンロード（Docker なし）

ご利用の環境向けバイナリをダウンロードし、実行権限を付与して起動します。

**Linux x64**

```bash
sudo curl -L https://github.com/matveynator/chicha-isotope-map/releases/download/latest/chicha-isotope-map_linux_amd64 \
  -o /usr/local/bin/chicha-isotope-map \
  && sudo chmod +x /usr/local/bin/chicha-isotope-map \
  && chicha-isotope-map
```

**macOS Intel (x86\_64)**

```bash
sudo curl -L https://github.com/matveynator/chicha-isotope-map/releases/download/latest/chicha-isotope-map_darwin_amd64 \
  -o /usr/local/bin/chicha-isotope-map \
  && sudo chmod +x /usr/local/bin/chicha-isotope-map \
  && chicha-isotope-map
```

**macOS Apple Silicon (arm64)**

```bash
sudo curl -L https://github.com/matveynator/chicha-isotope-map/releases/download/latest/chicha-isotope-map_darwin_arm64 \
  -o /usr/local/bin/chicha-isotope-map \
  && sudo chmod +x /usr/local/bin/chicha-isotope-map \
  && chicha-isotope-map
```

その他のプラットフォーム（Windows / ARM / BSD）はリリースページをご覧ください：
[https://github.com/matveynator/chicha-isotope-map/releases/tag/latest](https://github.com/matveynator/chicha-isotope-map/releases/tag/latest)

---

## 🖥 通常起動（Docker なし）：フラグと使用例

バイナリ `chicha-isotope-map` を直接起動する場合の要点です。まず重要項目、続いて追加パラメータを示します。

### 重要

* `-domain string` — HTTPS を有効化し、Let’s Encrypt の自動証明書で 80/443 を使用します。ドメインがサーバーに向いており、80/443 が空いている必要があります。

  * 例: `sudo chicha-isotope-map -domain maps.example.org -default-lat 44.08832 -default-lon 42.97577 -default-zoom 11 -default-layer "OpenStreetMap"`

* `-port int` — HTTP サーバーのポート（既定 8765）。ドメインなしのローカル実行に便利です。

  * 例: `chicha-isotope-map -port 8765`

* `-default-lat float` と `-default-lon float` — マップの初期緯度・経度。

  * 例: `-default-lat 44.08832 -default-lon 42.97577`

* `-default-zoom int` — 初期ズーム（都市部は通常 11–13）。

  * 例: `-default-zoom 11`

* `-default-layer string` — ベースレイヤー：`OpenStreetMap` または `Google Satellite`。

  * 例: `-default-layer "Google Satellite"`

### ストレージ（必要な場合）

* `-db-type string` — DB ドライバ：`genji`、`sqlite`、`pgx`（PostgreSQL）。既定は `genji`。
* `-db-path string` — `genji`/`sqlite` 用の DB ファイルパス（未指定時はカレントディレクトリ）。

  * 例: `-db-type sqlite -db-path /var/lib/chicha/chicha.sqlite`
* `-db-host string`, `-db-port int`（既定 5432）, `-db-name string`, `-db-user string`, `-db-pass string` — `pgx` 用の PostgreSQL 接続設定。

  * 例: `-db-type pgx -db-host 127.0.0.1 -db-port 5432 -db-name chicha_isotope_map -db-user postgres -db-pass secret`
* `-pg-ssl-mode string` — PostgreSQL の SSL モード：`disable`、`allow`、`prefer`（既定）、`require`、`verify-ca`、`verify-full`。

  * 例: `-pg-ssl-mode require`

### サービス系

* `-version` — バージョンを表示して終了。

### クイック例

* **ローカル（HTTPS なし）:**

  ```bash
  chicha-isotope-map \
    -port 8765 \
    -default-lat 44.08832 -default-lon 42.97577 \
    -default-zoom 11 \
    -default-layer "OpenStreetMap"
  ```

* **80/443 で HTTPS の公開サーバー:**

  ```bash
  sudo chicha-isotope-map \
    -domain maps.example.org \
    -default-lat 44.08832 -default-lon 42.97577 \
    -default-zoom 11 \
    -default-layer "OpenStreetMap"
  ```

* **1 ファイルに保存（SQLite）:**

  ```bash
  chicha-isotope-map \
    -db-type sqlite -db-path /var/lib/chicha-isotope-map.sqlite \
    -port 8765 \
    -default-lat 44.08832 -default-lon 42.97577 -default-zoom 11
  ```

* **PostgreSQL に接続:**

  ```bash
  chicha-isotope-map \
    -db-type pgx \
    -db-host 127.0.0.1 -db-port 5432 \
    -db-name chicha_isotope_map -db-user postgres -db-pass secret \
    -pg-ssl-mode require \
    -default-lat 44.08832 -default-lon 42.97577 -default-zoom 11
  ```

---

## 🤝 自前ノードを立てる理由

* **シンプル：** 自分のコミュニティには、自分たちのマップを。
* **有益：** 街／地域／事業所のバックグラウンド履歴を、手元に長期保存。
* **ネットワークのため：** ノードが増えるほど、透明性とレジリエンスが高まります。

---

Chicha‑Isotope‑Map は単なるプログラムではありません。肉眼では見えないが計測器には映る微粒子の世界を覗く小さな窓です。かつては「あるかもしれない」と想像するしかなかったものが、今は地図上に色の点で描かれます。穏やかな緑から、警戒の赤まで。

* **何をどこから読み込む？**

  * `.kml`, `.kmz`, `.json`, `.rctrk`, `.csv`, `.gpx` (AtomFast, RadiaCode, Safecast など）のファイルから。
  * すべてを独自 DB に保存し、何年先でも正確に振り返れます。「2008年3月12日、この場所は 4.1 µR/h だった？」に答えられるように。

* **基準は？**

  * 自然放射線レベル。クリーンな場所ではおよそ 0.8–4 µR/h。
  * それを超えるものは人工的な汚染。新雪に残る足跡のように、風や車、人の動きで同位体がどのように広がったかが見えてきます。

---

### 📸 **スクリーンショット**

…ソ連時代、キスロヴォツク公園に屋外プールを建設していました。もしかすると、ベシュタウ山の鉱石をかつて処理していたピャチゴルスクの工場からコンクリートを運んだのかもしれません。道を走るトラックの車輪から舞い上がった粉じんがアスファルトに落ち、目には見えない印を残す――そんな情景です。年月が経っても、その痕跡はいまだにほのかに光り、過去の記憶のように残っています。工事から広がった粉じんは公園の周りに降り積もり、地図では黄色の斑点として現れます。まるで落ち葉のように。一方で、公園のその他の場所は穏やかでクリーンな緑です。<img src="https://repository-images.githubusercontent.com/870016860/11fd6abc-fe8b-4cd8-95c2-df1c631c8762">

---

### 📸 **デモ**

<a href="https://jutsa.ru" target="_blank"><img width="1156" height="844" alt="Chicha Isotope Map" src="https://github.com/user-attachments/assets/8d806377-671f-47a0-b918-f2a9afd4123e" /></a>

<a href="https://jutsa.ru" target="_blank">こちらで実際の動作をリアルタイムで確認できます。</a>

---

### 📣 自分のノードをコミュニティに告知するには

1. ドメインで HTTPS のノードを立てます（`-e DOMAIN=...` と 80/443 の公開）。
2. スクリーンショットと簡単な説明（都市名・エリア・計測範囲など）を用意します。
3. プロジェクトのリポジトリの Issue に投稿して知らせてください。

---

*Chicha Isotope Map*は、**ドミトリー・イグナテンコ放射線研究所**のために制作されたものであり、その発想の源は、日本の市民科学コミュニティである**Safecast**にあります。彼らは福島の悲劇を、科学的知識へと昇華させた先駆者です。

放射線の真実を探し、測定し、そして分かち合うことで、あなた方は「見えないもの」を「見えるもの」とし、世界がチェルノブイリや福島の過ちを繰り返さないよう導いてくれています。

その努力は、科学の光であり、安全への道しるべであり、希望の証です。

自然放射線を恐れの対象から理解の対象へと変えてくださったことに、深く感謝申し上げます。

測り、伝え、そして何より最初の一歩を踏み出す勇気を持ってくださった皆さまに、心より敬意を表します。

また、**AtomFast**および**Radiacode**のコミュニティの皆さまの測定とデータの共有に、特別な感謝を申し上げます。


