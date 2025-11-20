[![最新の安定版](https://github.com/matveynator/chicha-isotope-map/actions/workflows/release.yml/badge.svg)](https://github.com/matveynator/chicha-isotope-map/actions/workflows/release.yml)

<img width="30%" align="left" alt="chicha-isotope-map" src="https://github.com/user-attachments/assets/39bfa7b1-03fb-43dd-89bd-8d6c516fd4db" />

* [🇬🇧 English](/README.md)
* [🇫🇷 Français](/doc/README_FR.md)
* [🇯🇵 日本語](/doc/README_JP.md)
* [🇷🇺 Русский](/doc/README_RU.md)

# ☢️ 放射線マップ
デモ: [https://pelora.org/](https://pelora.org/) — 自分のノードも同じ見た目です。

👉 [DeepWiki: Chicha Isotope Map](https://deepwiki.com/matveynator/chicha-isotope-map)

---

### 📸 サンプル
<a href="https://pelora.org" target="_blank"><img width="800" alt="pelora.org chicha-isotope-map" src="https://github.com/user-attachments/assets/be706959-a2d5-4949-9378-811f4022aa98" /></a>

---

## 🚀 Docker で即起動
PostgreSQL 同梱。コピーして実行するだけ。

#### 🔥 ローカル (ポート 8765)
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

#### 🔥 独自ドメインで HTTPS
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
Let’s Encrypt 完了後: [https://example.org](https://example.org)

**環境変数:** `DOMAIN`(HTTPS), `DEFAULT_LAT` / `DEFAULT_LON` / `DEFAULT_ZOOM` / `DEFAULT_LAYER`(初期表示), `PORT`(内部ポート)。データは `-v chicha-data:/var/lib/postgresql/data` に置き、アップデートで消えないようにします。

---

## ⬇️ バイナリを直接入手
ダウンロード→実行権限付与→起動。

**Linux x64**
```bash
sudo curl -L https://github.com/matveynator/chicha-isotope-map/releases/download/latest/chicha-isotope-map_linux_amd64 \
  -o /usr/local/bin/chicha-isotope-map \
  && sudo chmod +x /usr/local/bin/chicha-isotope-map \
  && chicha-isotope-map
```

**macOS Intel (x86_64)**
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

その他 (Windows / ARM / BSD): [最新リリース](https://github.com/matveynator/chicha-isotope-map/releases/tag/latest)。

---

## 🖥 バイナリの主なフラグ
- `-domain maps.example.org` — 80/443 で HTTPS (Let’s Encrypt)。
- `-port 8765` — ローカル実行用ポート。
- `-default-lat` / `-default-lon` / `-default-zoom` / `-default-layer` — 初期表示設定。
- ストレージ: `-db-type sqlite|duckdb|pgx|chai|clickhouse`, `-db-path`(ファイルDB), `-db-conn`(ネットワークDB)。
- ユーティリティ: `-version` でバージョン表示。

DuckDB: `CGO_ENABLED=1 go build -tags duckdb` の後 `./chicha-isotope-map -db-type duckdb`。

---

## 📥 インポート
- 対応: `.kml`, `.kmz`, `.json`, `.rctrk`, `.csv`, `.gpx`, bGeigie Nano/Zen `$BNRDD` ログ (`.log` / `.txt`), AtomFast / RadiaCode / Safecast 等。
- Web: ノードを開く → **Upload** → ファイル選択 → 直近の取り込みトラックが自動表示。
- API: `curl -F 'files[]=@/path/to/file.log' http://localhost:8765/upload`（診断 `/upload_diag`）。
- 近傍の最新測定: `/api/latest?lat=...&lon=...&radius_m=1500&limit=20`。

---

## 📤 エクスポート
- **トラック単位:** `/api/track/{trackID}.json`（古い `.cim` も可）。`from`/`to` で ID 範囲を絞れます。
- **まとめアーカイブ:** `/api/json/weekly.tgz`（設定により `/daily.tgz` `/monthly.tgz` `/yearly.tgz` も）。各トラックが1つの JSON に入ります。
- **JSON スキーマ:**
  - ルート: `trackID`, `trackIndex`(1始まり), `apiURL`, `firstID`, `lastID`, `markerCount`, `disclaimers`, `markers`。
  - マーカー: `id`, `timeUnix`, `timeUTC`(RFC3339), `lat`, `lon`, 任意 `altitudeM`, `temperatureC`, `humidityPercent`, 速度 (`speedMS`, `speedKMH`), 線量 (`doseRateMicroSvH`, `doseRateMicroRh`), `countRateCPS`, 必要に応じ `detectorType`, `detectorName`, `radiationTypes`。
  - `disclaimers` には多言語の注意書きを同梱。
- **今後:** 各ポイントのスペクトルデータも同じ JSON に追加する予定です。

---

## 💾 バックアップ / 復元
- **毎日 03:00:** `0 3 * * * docker exec chicha-isotope-map pg_dump -U chicha_isotope_map chicha_isotope_map | gzip > /backup/chicha_isotope_map_$(date +\%F).sql.gz`
- **復元:**
  ```bash
  docker exec -it chicha-isotope-map psql -U postgres -c "DROP DATABASE IF EXISTS chicha_isotope_map; CREATE DATABASE chicha_isotope_map OWNER chicha_isotope_map;"
  zcat /backup/chicha_isotope_map_2025-07-24.sql.gz | docker exec -i chicha-isotope-map psql -U chicha_isotope_map chicha_isotope_map
  ```

---

## 🤝 自前ノードを立てる理由
- コミュニティの測定と履歴を自分で管理。
- 自然バックグラウンド（おおむね 0.8–4 µR/h）の変化を把握。
- ノードが増えるほど透明性とレジリエンスが高まります。

Chicha‑Isotope‑Map は **Dmitry Ignatenko Radiation Research Lab** のために作られ、**Safecast** に着想を得ています。AtomFast と Radiacode のコミュニティにも感謝します。
