[![最新の安定版リリースビルド](https://github.com/matveynator/chicha-isotope-map/actions/workflows/release.yml/badge.svg)](https://github.com/matveynator/chicha-isotope-map/actions/workflows/release.yml)

* [🇬🇧 English](/README.md)
* [🇫🇷 Français](/doc/README_FR.md)
* [🇯🇵 日本語](/doc/README_JP.md)
* [🇷🇺 Русский](/doc/README_RU.md)

# chicha-isotope-map ― 世界の放射線マップ ☢️

ライブデモ <a href="https://pelora.org" target="_blank">https://pelora.org</a>

---

### 📸 **ライブデモ**

<a href="https://pelora.org" target="_blank"><img width="800"  alt="pelora.org chicha-isotope-map example demo" src="https://github.com/user-attachments/assets/be706959-a2d5-4949-9378-811f4022aa98" /></a>

---

**Chicha Isotope Map** の **DeepWiki** ページは、**Safecast** プロジェクトの [Rob Ouden](https://github.com/robouden) 氏により作成されました。心から感謝いたします。
このページではソフトウェアの内部構造が丁寧に解説されており、開発者が基盤を理解し、論理を追い、改善や拡張を続けられるようになっています。
DeepWiki のおかげで、コードは単なる道具ではなく、多くの人の手で成長し進化していく「生きたプロジェクト」となりました。

👉 [DeepWiki: Chicha Isotope Map](https://deepwiki.com/matveynator/chicha-isotope-map)

---

## 🚀 自分のノードを 2 コマンドで立ち上げる

余計な作業は不要です。イメージにはデータベース（PostgreSQL）が同梱されています。コマンドをコピーして実行すれば完了です。

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

#### 🔥 独自ドメインで HTTPS 公開ノード

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

証明書が発行されたらアクセス: [https://example.org](https://example.org)

---

## DuckDB

DuckDB は組み込みデータベースです。1 ファイル、サーバ不要、C++ で実装されています。
Go ドライバは **CGO** と **共有ライブラリ** に依存しているため、次のようにビルドします：

```bash
CGO_ENABLED=1 go build -tags duckdb
```

実行例:

```bash
./chicha-isotope-map -db-type duckdb
```

※ macOS では単に [ビルド済みリリース](https://github.com/matveynator/chicha-isotope-map/releases/tag/latest) をダウンロードして使うこともできます。

---

Chicha-Isotope-Map は単なるソフトウェアではなく、目には見えない微粒子の世界を映し出す窓です。
かつては推測にすぎなかったものが、今は地図上の点として描かれます。落ち着いた緑から警告の赤へ ― 背景放射の物語が色彩となって現れるのです。

このプロジェクトは **Dmitry Ignatenko 放射線研究所** によって作られ、そして **Safecast** ― 福島の悲劇を科学的知識の贈り物に変えた日本の市民科学者コミュニティ ― から大きな着想を得ています。

見えないものを測り、記録し、分かち合うことで、チェルノブイリや福島の苦い教訓を繰り返さない。
その行為自体が「科学の光」「安全の光」「希望の光」となります。

放射線を「恐れるもの」から「理解できるもの」へと変えてくれて、本当にありがとうございます。

