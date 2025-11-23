[![Latest stable release build](https://github.com/matveynator/chicha-isotope-map/actions/workflows/release.yml/badge.svg)](https://github.com/matveynator/chicha-isotope-map/actions/workflows/release.yml)

<img width="30%" align="left" alt="chicha-isotope-map" src="https://github.com/user-attachments/assets/39bfa7b1-03fb-43dd-89bd-8d6c516fd4db" />

- [🇬🇧 English](/README.md)
- [🇫🇷 Français](/doc/README_FR.md)
- [🇯🇵 日本語](/doc/README_JP.md)
- [🇷🇺 Русский](/doc/README_RU.md)
- [🇨🇭 Schwiizerdütsch](/doc/README_DE_CH.md)
- [🇮🇹 Italiano](/doc/README_IT.md)
- [🇨🇳 中文](/doc/README_ZH.md)
- [🇮🇳 हिन्दी](/doc/README_HI.md)
- [🇮🇷 فارسی](/doc/README_FA.md)
- [🇲🇳 Монгол](/doc/README_MN.md)
- [🇰🇿 Қазақша](/doc/README_KK.md)

# ☢️ 世界の放射線マップ
この地図は、誰でもすぐに「住んでいる場所や働く場所が安全かどうか」を理解できるように作りました。多くの人が畑を耕し、家畜を飼い、湧き水を飲みますが、環境が健全かどうかは分からないことがあります。

自然の放射線レベルは低く保たれます。数値が大きく上がるのは、人間活動や地質の特徴が理由のときだけです。そうした場所では、水・空気・土が時間とともに健康に影響し、肺や胃などの臓器を傷める可能性があります。

この地図が一人でも、一頭の命でも守れたなら、作った甲斐があります。より安全な道を選ぶための、シンプルで分かりやすい道しるべになれば幸いです。

ライブデモ: [https://pelora.org/](https://pelora.org/) — あなたのノードも同じように見えます。

👉 [ダウンロードページ](https://github.com/matveynator/chicha-isotope-map/releases)（全プラットフォーム、最新ビルド）

👉 [DeepWiki: Chicha Isotope Map](https://deepwiki.com/matveynator/chicha-isotope-map)

---

### 📸 例
<p>
  <a href="https://pelora.org" target="_blank"><img width="100%" alt="Fukushima view in chicha-isotope-map" src="https://github.com/user-attachments/assets/617a0ced-4280-41c2-9320-de1cfd33a61f" /></a><br />
  <a href="https://pelora.org" target="_blank"><img width="100%" alt="Safecast realtime radiation sensors in chicha-isotope-map" src="https://github.com/user-attachments/assets/13256b23-744d-4d02-a26c-ae9aef5b0d87" /></a><br />
  <a href="https://pelora.org" target="_blank"><img width="100%" alt="Air flights radiation in chicha-isotope-map" src="https://github.com/user-attachments/assets/cf0189c9-534f-4ff5-9d7a-ed5836e91ef5" /></a>
</p>

---

## 🧭 機能
- 多様な計測器のデータを集め、移動速度別にレイヤーを分けています（徒歩・車・航空）。
- 自分のトラックをアップロードすると、新しいポイントがすぐ地図に現れ、状況を把握しやすくなります。
- URL またはファイルでアーカイブを取り込み、自分のデータをアーカイブに保存できます（バックアップに便利）。
- 選んだ場所の放射線レベルが時間とともにどう変化したか、改善したか悪化したかを追えます。
- 地図の任意の場所への短縮リンクを作成できます。
- 印刷モードでは危険箇所に QR コードを付けられます。コードを読み取るだけでその地点の線量が分かるため、「水を飲まない・長時間滞在しない・農作を避ける」などの環境リスクを示せます。環境の専門家やモニタリング担当者、警告を出す部門に役立ちます。
- オープンな CC ライセンスのもとで外部サービスと連携できる API があります。

このプロジェクトは **Safecast** コミュニティの丁寧な支え、**Rob Oudendijk** の大きな働き、そして世界中でオープン線量測定に取り組む多くの人々によって成長しています。Safecast、AtomFast、Radiacode、DoseMap などのイニシアチブに感謝します。

---

## 🚀 すぐに使う（初心者向け）
最速の方法はバイナリをダウンロードすることです。Docker やデータベースなど追加ツールは不要です。ダウンロードして実行するだけ。

### オプション1. バイナリ（推奨）
1) [リリースページ](https://github.com/matveynator/chicha-isotope-map/releases)で自分の環境向けビルドをダウンロードします。
2) 実行権限を付けて起動します:
```bash
chmod +x ./chicha-isotope-map
./chicha-isotope-map
```
3) [http://localhost:8765](http://localhost:8765) を開けば、地図がすでに動いています。

必要に応じて調整できるもの:
- `-port 8765` — ローカルのポート。
- `-domain maps.example.org` — Let’s Encrypt で HTTPS（80/443 が必要）。
- `-default-lat` / `-default-lon` / `-default-zoom` / `-default-layer` — 起動時の地図ビュー。
- ストレージ: `-db-type sqlite|duckdb|chai|clickhouse|pgx`、ファイル型は `-db-path`、ネットワーク型は `-db-conn`。

### オプション2. ドメイン付き公開ノード
1) ドメイン指定でバイナリを起動します:
```bash
./chicha-isotope-map -domain example.org
```
2) Let’s Encrypt のために 80/443 を開けておきます。証明書が出れば [https://example.org](https://example.org) で公開されます。

### オプション3. Docker（すべて同梱）
1) Docker（Desktop でも CLI でも可）をインストールします。
2) Docker Hub で **matveynator/chicha-isotope-map** を探し、**Run** を押すか、次の一行を実行します:
```bash
docker run -d -p 8765:8765 --name chicha-isotope-map matveynator/chicha-isotope-map:latest
```
3) [http://localhost:8765](http://localhost:8765) を開けば完了です。

---

## 📥 データを入れる
- 地図ページで緑の **Upload** ボタンを押し、トラックをドロップします（`.kml`, `.kmz`, `.json`, `.rctrk`, `.csv`, `.gpx`, bGeigie Nano/Zen `$BNRDD`, AtomFast, RadiaCode, Safecast など）。
- pelora.org を丸ごとミラーするには一回だけ `chicha-isotope-map -import-tgz-url https://pelora.org/api/json/weekly.tgz` を実行します。毎週のアーカイブを取り込み、データベースを満たしてから終了するので、次の起動ですぐ地図が賑やかに見えます。
- 先にアーカイブをローカル保存したい場合: [https://pelora.org/api/json/weekly.tgz](https://pelora.org/api/json/weekly.tgz) をダウンロードし、`-import-tgz-path /path/to/weekly.tgz` を付けて自分のコピーから読み込みます。

### 🗺️ 初回から実データを入れて立ち上げる一行
まっさらな環境なら、この一行で既存の計測を取り込みつつそのまま地図を公開できます。
```bash
chicha-isotope-map -import-tgz-url https://pelora.org/api/json/weekly.tgz
```
取り込み後は通常どおり再起動するだけ（または同じコマンドを systemd などに登録）。[http://localhost:8765](http://localhost:8765) に開けば最初から実測値が見えます。

### 🛢️ インポートと通常運用で選ぶデータベース
- **PostgreSQL（`pgx`）** — 複数ユーザーでも最速で扱いやすい選択。例: `chicha-isotope-map -db-type pgx -db-conn postgres://USER:PASS@HOST:PORT/DATABASE?sslmode=allow -import-tgz-url https://pelora.org/api/json/weekly.tgz`
- **DuckDB / SQLite / Chai** — 単独利用向けのシンプルなファイル型。複数人で同時に書くと衝突するため、個人用マップに向きます。例: `chicha-isotope-map -db-type duckdb -import-tgz-url https://pelora.org/api/json/weekly.tgz`

## 📤 エクスポート
- 単一トラック: `/api/track/{trackID}.json`（古い `.cim` も動作）。
- 定期アーカイブ: `/api/json/weekly.tgz`（または `/daily.tgz`, `/monthly.tgz`, `/yearly.tgz`）。中身はトラックごとの JSON。

---

## 🧠 上級オプション
- データベース: 既定は内蔵 SQLite。DuckDB、Chai、ClickHouse、PostgreSQL（`pgx`）にも切り替え可能。
- インポート: URL またはファイルで、アーカイブを直接渡せます。
- エクスポート: JSON アーカイブ、単一トラック、旧 `.cim` に対応。
- 見た目: 起動時の座標とレイヤーを `-default-*` で指定。

---

## 🤝 自分のノードを持つ理由と少しの歴史
- 誰でも訓練なしで、住んでいる場所や畑、水源に放射線の危険があるか見えるようにしたかった。
- ノードが多いほど、汚染の見落としが起きにくくなります。

Chicha-Isotope-Map は **Dmitry Ignatenko** の現場での歩みに触発され、**Rob Oudendijk** と **Safecast** から強い影響を受けています。AtomFast と Radiacode コミュニティのオープンデータが日々の役立ちを支えています。もしこの地図が一人でも、一頭でも救えるなら、作った甲斐があります。
