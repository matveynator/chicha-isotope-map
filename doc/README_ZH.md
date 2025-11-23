[![最新稳定版构建](https://github.com/matveynator/chicha-isotope-map/actions/workflows/release.yml/badge.svg)](https://github.com/matveynator/chicha-isotope-map/actions/workflows/release.yml)

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

# ☢️ 世界辐射地图
这张地图的目标，是让没有专业背景的人也能一眼看出周围的房屋、农田、森林或水源是否受到辐射威胁。干净的地方通常保持在 2–3 µR/h；更暗的斑点几乎都来自人为活动。地图展示了捷克、俄罗斯、哈萨克斯坦、蒙古的铀矿如何留下长长的痕迹；展示了福岛像黑红色的“肿瘤”一样立在日本海岸；展示了切尔诺贝利和布良斯克地区如何在土地上留下印记；展示了法国、捷克和高加索矿泉区的氡矿脉怎样提高风险。开采铀和稀土后留下的可溶性盐会渗入含水层，最终进入我们的饮水和食物。如果这张地图能保护到哪怕一个人或动物，制作它就是值得的。

在线演示：[https://pelora.org/](https://pelora.org/) —— 你的节点看起来也是这样。

👉 [统一下载页](https://github.com/matveynator/chicha-isotope-map/releases)（所有平台，最新构建）

👉 [DeepWiki：Chicha Isotope Map](https://deepwiki.com/matveynator/chicha-isotope-map)

---

### 📸 示例
<a href="https://pelora.org" target="_blank"><img width="800" alt="pelora.org chicha-isotope-map 示例" src="https://github.com/user-attachments/assets/be706959-a2d5-4949-9378-811f4022aa98" /></a>

---

## 🧭 地图包含什么
- 实时地图，汇集多种探测器的测量；可自由切换图层。
- 上传自己的轨迹；新点会立刻出现在你查看的区域。
- 通过 URL 或文件导入，导出为归档。
- 可单节点运行，也能组成网络：节点越多，透明度越高。

项目在 **Safecast** 和社区的帮助下成长：许多好点子来自 **Rob Oudendijk** 和全球开放剂量学的朋友们（感谢 Greenpeace 及其他环保团队）。

---

## 🚀 快速上手（面向新手）
最快的方式：下载二进制文件。无需 Docker、数据库或额外工具——下载、运行、完成。

### 方案 1. 二进制（推荐）
1) 打开 [发布页](https://github.com/matveynator/chicha-isotope-map/releases) 并下载适合你的系统的构建。
2) 赋予可执行权限并运行：
```bash
chmod +x ./chicha-isotope-map
./chicha-isotope-map
```
3) 打开 [http://localhost:8765](http://localhost:8765) —— 地图已经运行。

可选参数：
- `-port 8765` —— 本地端口。
- `-domain maps.example.org` —— 使用 Let’s Encrypt 获取 HTTPS（需要 80/443）。
- `-default-lat` / `-default-lon` / `-default-zoom` / `-default-layer` —— 地图初始视图。
- 存储：`-db-type sqlite|duckdb|chai|clickhouse|pgx`，文件库用 `-db-path`，网络库用 `-db-conn`。

### 方案 2. 有域名的公共节点
1) 使用你的域名启动：
```bash
./chicha-isotope-map -domain example.org
```
2) 确保 80/443 端口开放以获取 Let’s Encrypt 证书。完成后站点位于 [https://example.org](https://example.org)。

### 方案 3. Docker（一切打包好）
1) 安装 Docker（Desktop 或 CLI）。
2) 在 Docker Hub 搜索 **matveynator/chicha-isotope-map** 并点击 **Run**（或执行命令）：
```bash
docker run -d -p 8765:8765 --name chicha-isotope-map matveynator/chicha-isotope-map:latest
```
3) 打开 [http://localhost:8765](http://localhost:8765) —— 完成。

---

## 📥 导入数据
- 在地图页面点击绿色的 **Upload** 按钮，上传你的轨迹（`.kml`、`.kmz`、`.json`、`.rctrk`、`.csv`、`.gpx`、bGeigie Nano/Zen `$BNRDD`、AtomFast 导出、RadiaCode、Safecast 等）。
- 想要 pelora.org 的镜像？运行一次 `chicha-isotope-map -import-tgz-url https://pelora.org/api/json/weekly.tgz` —— 它会下载每周归档、填充数据库后退出，这样下一次启动就自带数据。
- 想先下载归档？获取 [https://pelora.org/api/json/weekly.tgz](https://pelora.org/api/json/weekly.tgz)，用 `-import-tgz-path /path/to/weekly.tgz` 启动，使用本地副本。

### 🗺️ 一条命令完成首启动并加载真实数据
在全新系统上，运行：
```bash
chicha-isotope-map -import-tgz-url https://pelora.org/api/json/weekly.tgz
```
导入完成后按常规方式重启（或把同一命令放入 systemd 服务）—— 打开 [http://localhost:8765](http://localhost:8765) 就能看到真实测量点。

### 🛢️ 导入和日常使用的数据库选择
- **PostgreSQL (`pgx`)** —— 速度最快，适合多人写入。示例：`chicha-isotope-map -db-type pgx -db-conn postgres://USER:PASS@HOST:PORT/DATABASE?sslmode=allow -import-tgz-url https://pelora.org/api/json/weekly.tgz`
- **DuckDB / SQLite / Chai** —— 适合单用户的文件型方案。并发写入会冲突，适合个人地图。示例：`chicha-isotope-map -db-type duckdb -import-tgz-url https://pelora.org/api/json/weekly.tgz`

## 📤 导出
- 单个轨迹：`/api/track/{trackID}.json`（旧的 `.cim` 也支持）。
- 定期归档：`/api/json/weekly.tgz`（或 `/daily.tgz`、`/monthly.tgz`、`/yearly.tgz`）。内容：每条轨迹一个 JSON。

---

## 🧠 高级选项
- 数据库：默认内置 SQLite；可切换到 DuckDB、Chai、ClickHouse 或 PostgreSQL（`pgx`）。
- 导入：通过 URL 或文件，也可直接提供归档。
- 导出：JSON 归档、单条轨迹、兼容 `.cim`。
- 外观：初始坐标和图层（`-default-*`）。

---

## 🤝 为什么要有自己的节点，以及一点历史
- 我们希望每个人无需培训就能知道：辐射是否威胁他们生活、耕作或取水的地方。
- 节点越多，整体图景越可靠，越不容易错过污染。

Chicha‑Isotope‑Map 受 **Dmitry Ignatenko** 的野外研究启发，并深受 **Rob Oudendijk** 和 **Safecast** 影响。来自 AtomFast 和 Radiacode 社区的开放数据让地图保持实用。如果地图能拯救哪怕一条生命，它的存在就有意义。
