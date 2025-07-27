- [🇬🇧 English](/README.md)
- [🇫🇷 Français](/doc/README_FR.md)
- [🇯🇵 日本語](/doc/README_JP.md)
- [🇷🇺 Русский](/doc/README_RU.md)

# 🌌 Chicha‑Isotope‑Map ― 放射線の隠れた小径へのガイド

Chicha‑Isotope‑Map は単なるプログラムではありません。肉眼では見えないが計測器には確かに感じられる微粒子の世界を覗くための「窓」です。これまでは「きっとあるはず」と推測するしかなかったものが、今は地図上に鮮やかな点として描かれます――穏やかな緑から、不安を呼ぶ赤まで。

* **何を、どこから読み込むの？**

  * `.kml`, `.kmz`, `.json`, `.rctrk`（AtomFast, RadiaCode）形式のファイルから読み込みます。
  * すべてを独自のデータベースに保存するので、数年後でも正確に振り返れます。
    例：「2024年3月12日、ここは 4.1 µR/h だった」と。

* **基準は何？**

  * 自然放射線のバックグラウンド： “きれい” な場所ではおおよそ 0.8〜4 µR/h。
  * それ以上は外来の汚染です。風や車、人によってアイソトープがどのように拡散したか、雪原についた足跡のように地図上で確認できます。

---

### 📸 **スクリーンショット**

…ソ連時代、キスロヴォツク公園では屋外プールが建設されていました。もしかすると、ベシュタウ山の放射性鉱石をかつて処理していたピャチゴルスクの工場からコンクリートを持ってきたのかもしれません。道を走ったトラックの車輪から舞い上がった粉塵はアスファルトに降り積もり、目には見えない印を残しました。年月が経った今でも、その痕跡は微かに光り続け、まるで昔の記憶のようです。工事現場から舞い散った塵は公園に降り積もり、地図上では秋の落ち葉のような黄色の斑点に見えます。一方、公園の他の部分は清浄で穏やかな緑です。 <img src="https://repository-images.githubusercontent.com/870016860/11fd6abc-fe8b-4cd8-95c2-df1c631c8762">

---

### 📸 **デモ**

<a href="https://jutsa.ru" target="_blank"><img width="1156" height="844" alt="Chicha Isotope Map" src="https://github.com/user-attachments/assets/8d806377-671f-47a0-b918-f2a9afd4123e" /></a>

<a href="https://jutsa.ru" target="_blank">こちらでリアルタイム動作を見ることができます。</a>

---

## 🚀 インストールと自前ノードの立ち上げ（5分で完了！）

### 1. Docker でクイックスタート

**なぜ Docker？**
Docker はプログラムとその実行環境を「コンテナ」にまとめます。データベースや依存関係を手作業で整える必要はありません。用意されたイメージを実行するだけです。

#### ローカル実行（ポート 5000）

```bash
docker run -d \
  --name chicha-isotope-map \
  -e PORT=5000 \
  -p 5000:5000 \
  -v isotope-data:/var/lib/postgresql/data \
  matveynator/chicha-isotope-map:latest
```

ブラウザで [http://localhost:5000](http://localhost:5000) を開くと地図が表示されます。

#### 自分のドメイン＋HTTPSで運用する

1. `domain.com` があなたのサーバーの IP を指していることを確認。
2. ポート 80 と 443 が空いていることを確認。
3. **root** 権限で以下を実行：

```bash
docker run -d \
  --name chicha-isotope-map \
  -e DOMAIN=domain.com \
  -p 80:80 -p 443:443 \
  -v isotope-data:/var/lib/postgresql/data \
  matveynator/chicha-isotope-map:latest
```

プログラムが自動的に SSL 証明書を取得・更新します。

#### 地図の追加設定

利用可能なオプションは `--help` で確認できます。
必要に応じて開始位置やスタイルを指定：

```text
  -e DEFAULT_LAT=51.389      # 緯度
  -e DEFAULT_LON=30.099      # 経度
  -e DEFAULT_ZOOM=11         # ズームレベル
  -e DEFAULT_LAYER="OpenStreetMap" または "Google Satellite"
```

#### 定期バックアップ（1日1回）

`crontab -e` に追加：

```bash
0 3 * * * docker exec chicha-isotope-map pg_dump -U chicha_isotope_map chicha_isotope_map | gzip > /backup/chicha_isotope_map_$(date +\%F).sql.gz
```

#### アーカイブからの復元

```bash
docker exec -it chicha-isotope-map psql -U postgres -c "DROP DATABASE IF EXISTS chicha_isotope_map; CREATE DATABASE chicha_isotope_map OWNER chicha_isotope_map;"

zcat /backup/chicha_isotope_map_2025-07-24.sql.gz | docker exec -i chicha-isotope-map psql -U chicha_isotope_map chicha_isotope_map
```

---

### 2. Docker を使わないインストール

コンテナが好きでない方は、ビルド済みバイナリをダウンロードして一瞬で起動できます。さらに簡単です！

> コマンドは **root** 権限で実行してください（`sudo -i` または `sudo ...`）。

* **Linux x64**:

```
curl -L https://github.com/matveynator/chicha-isotope-map/releases/download/latest/chicha-isotope-map_linux_amd64 > /usr/local/bin/chicha-isotope-map && chmod +x /usr/local/bin/chicha-isotope-map && chicha-isotope-map
```

* **macOS Intel**:

```
curl -L https://github.com/matveynator/chicha-isotope-map/releases/download/latest/chicha-isotope-map_darwin_amd64 > /usr/local/bin/chicha-isotope-map && chmod +x /usr/local/bin/chicha-isotope-map && chicha-isotope-map
```

* **macOS Apple Silicon**:

```
curl -L https://github.com/matveynator/chicha-isotope-map/releases/download/latest/chicha-isotope-map_darwin_arm64 > /usr/local/bin/chicha-isotope-map && chmod +x /usr/local/bin/chicha-isotope-map && chicha-isotope-map
```

その他のプラットフォーム（Windows / ARM / BSDなど）はリリースページから取得できます：
[https://github.com/matveynator/chicha-isotope-map/releases/tag/latest](https://github.com/matveynator/chicha-isotope-map/releases/tag/latest)

起動後、デフォルトではポート 8765 を待ち受けています。
[http://localhost:8765](http://localhost:8765) を開いてください。

---

## 🤝 なぜ自分のノードを？

* **独立性:** データはあなたの手元にあり、他人のネットワークに依存しません。
* **ネットワークの強靭性:** ノードが多ければ多いほど、妨害や改ざんは難しくなります。
* **地域のバックグラウンド履歴:** あなたの地域の放射線地図を長期保存できます。

あなたのサーバー一台一台が、もう一つの情報の灯台です。世界をより透明にしてくれてありがとう！

Chicha アイソトープマップは、ドミトリー・イグナテンコ放射線研究ラボのために作られ、日本のコミュニティ Safecast に触発されました。彼らは悲劇を知識へと変えた市民科学者の集団です。放射線の真実を探し、測り、伝えることで、あなたは見えないものを見えるようにし、チェルノブイリやフクシマを繰り返さないための力になります。あなたの活動は科学・安全・希望の光です。バックグラウンド放射線を恐怖の種ではなく理解の源へと変え、探し、測り、共有し、勇気を持って最初の一歩を踏み出してくれてありがとう。
