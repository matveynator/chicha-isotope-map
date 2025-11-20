[![Latest stable release build](https://github.com/matveynator/chicha-isotope-map/actions/workflows/release.yml/badge.svg)](https://github.com/matveynator/chicha-isotope-map/actions/workflows/release.yml)

<img width="30%" align="left" alt="chicha-isotope-map" src="https://github.com/user-attachments/assets/39bfa7b1-03fb-43dd-89bd-8d6c516fd4db" />

- [🇬🇧 English](/README.md)
- [🇫🇷 Français](/doc/README_FR.md)
- [🇯🇵 日本語](/doc/README_JP.md)
- [🇷🇺 Русский](/doc/README_RU.md)

# ☢️ 世界の放射線マップ
この地図はドミートリイ・リハチョフのように、謙虚で分かりやすく作っています。初心者でもすぐに「自分の暮らす場所、畑や果樹園、キノコや薬草を採る森、家畜を放牧する牧草地、水を汲む井戸や川」に放射線があるかどうかが分かります。自然の森や野原、川の多くは2〜3 µR/hほどで安定しています。それ以上は人間の活動が原因であることがほとんどです。チェコやロシア、カザフスタン、モンゴルのウラン鉱山が長い傷跡を残したこと、福島の後に暗い染みが広がったこと、チェルノブイリとブリャンスクが地図の「腫瘍」になったこと、フランスやチェコ、コーカサス・ミネラルウォーター一帯のラドンを多く含む地層が肺がんや胃がんのリスクを高めることが見て取れます。ウランやレアアースを浸出すると水溶性の塩が地下に残り、帯水層に入り、やがて私たちの水や食べ物に紛れ込みます。この地図が一人でも、一頭の動物でも守ることができるなら、作った意味があります。

ライブデモ: [https://pelora.org/](https://pelora.org/) — あなたのノードも同じように見えます。

👉 [ダウンロードページ](https://github.com/matveynator/chicha-isotope-map/releases)（全プラットフォーム、最新ビルド）

👉 [DeepWiki: Chicha Isotope Map](https://deepwiki.com/matveynator/chicha-isotope-map)

---

### 📸 例
<a href="https://pelora.org" target="_blank"><img width="800" alt="pelora.org chicha-isotope-map 例" src="https://github.com/user-attachments/assets/be706959-a2d5-4949-9378-811f4022aa98" /></a>

---

## 🧭 機能
- さまざまな測定器のデータを重ねて表示でき、地図レイヤーも選べます。
- 自分のトラックをアップロードすると、見ている場所の周りに新しいポイントがすぐに出ます。
- URL またはファイルでインポートし、アーカイブとしてエクスポートできます。
- 単独ノードでもネットワークでも動作します。ノードが増えるほど透明性が高まります。

このプロジェクトはコミュニティの助けで成長しています。特に **Rob Alden**、そして世界のオープン線量測定の仲間たち（Greenpeace などの環境チームにも感謝しています）から多くの提案をもらいました。

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
- すぐ使えるデータベース: [pelora.org](https://pelora.org/) に完全なアーカイブがあります。ローダーにその URL を渡すか、ダウンロードして **Upload** から追加します。
- Web インポート: **Upload** → ファイルを選択（`.kml`, `.kmz`, `.json`, `.rctrk`, `.csv`, `.gpx`, bGeigie Nano/Zen `$BNRDD`, AtomFast, RadiaCode, Safecast など）。
- API インポート: `curl -F 'files[]=@/path/to/file.log' http://localhost:8765/upload` （診断: `/upload_diag`）。

## 📤 エクスポート
- 単一トラック: `/api/track/{trackID}.json`（古い `.cim` も動作）。
- 定期アーカイブ: `/api/json/weekly.tgz`（または `/daily.tgz`, `/monthly.tgz`, `/yearly.tgz`）。中身はトラックごとの JSON。

---

## 🧠 上級オプション
- データベース: 既定は内蔵 SQLite。DuckDB、Chai、ClickHouse、PostgreSQL（`pgx`）にも切り替え可能。
- インポート: URL またはファイル、アーカイブも受け付けます。
- エクスポート: JSON アーカイブ、単一トラック、旧 `.cim` も対応。
- 見た目: 起動時の座標とレイヤーを `-default-*` で指定。

---

## 🤝 自分のノードを持つ理由と少しの歴史
- 誰でも訓練なしで、住んでいる場所や畑、水源に放射線の危険があるか見えるようにしたかった。
- 自分のノードがベースラインと履歴を与えます（通常 0.8–4 µR/h）ので、異常が目立ちます。
- ノードが多いほど、汚染の見落としが起きにくくなります。

Chicha-Isotope-Map は **Dmitry Ignatenko 研究室**のために作られ、**Safecast** に触発され、AtomFast と Radiacode コミュニティのオープンデータに支えられています。もしこの地図が一人でも、一頭でも救えるなら、作った甲斐があります。
