# HD-Icons Go 版（对齐 Python 原版全部功能）

单二进制 Go 服务 + `go:embed` 前端，与 `hd-icons/app.py` API 行为 1:1 兼容。

## 运行

```bash
cd hd-icons-go
go build -o hd-icons .
ICONS_DIR=/app/icons PORT=50560 ./hd-icons
```

Docker（需 docker 环境，本机无 daemon 未实际构建，仅静态检查通过）：

```bash
docker build -t xushier/hd-icons:go .
docker compose up -d
```

## 环境变量

| 变量 | 默认 | 说明 |
|---|---|---|
| `PORT` | `50560` | 监听端口 |
| `ICONS_DIR` | `/app/icons` | 图标数据卷（与原容器一致） |
| `TITLE` | `小迪的图标库` | 网页标题（README 有、原 app.py 缺失，本次补齐） |
| `CUSTOM_URL` | 空（=同源） | 单击复制地址前缀（README 有、原 app.py 缺失，本次补齐，前端 `window.CUSTOM_URL`） |
| `ALL_PROXY`/`all_proxy` | 空 | 大小写均兼容，原版只认小写 |
| `UPDATE_INTERVAL` | `1h` | 上游定时检查间隔（Go duration，原后端每天/前端每小时已统一） |
| `GIT_DEPTH` | `1` | 首次浅克隆深度；设为空或 `0` 则完整克隆（占多 ~250MB） |
| `ICONS_REPO_URL` | `https://github.com/xushier/HD-Icons.git` | 图标库 git 源；可指向镜像/加速站，切换后重启即生效（自动 `set-url`，无需删卷） |
| `ICONS_REPO_FALLBACK` | 同官方源 | 镜像 clone/pull 失败时的回退地址；可指向第二镜像 |
| `FONT_DIR` | `/app/static/font` | 外部字体挂载优先于内置 `static/fonts` |

## 与原版差异（有意修复）

1. `GET /sw.js` 别名：原前端注册 `/sw.js` 但文件在 `/static/js/sw.js`，Flask 无此路由 → PWA 注册 404；Go 版补齐。
2. `sw.js` 缓存清单把不存在的 `styles.css/app.js` 修正为 `xdtx.css/xdtx.js`。
3. 上传文件名做 `Base` 清洗 + 后缀白名单服务端二次校验，`delete-image` 仅限 `upload/` 且防 `../`（原版可穿越）。
4. `git pull` 成功后增量补缩略图（原版新图无缩略图）。
5. 上传后只生成本次文件的缩略图（原版全量再生）。
6. 新增 `GET /healthz`（`{"status":"ok"}`），不影响旧 API。

## 已验证（2026-09-11，本地二进制实测）

- `go mod tidy / go vet / gofmt`干净，产物约16MB（`CGO_ENABLED=0`）。
- `/` 标题注入 + `window.CUSTOM_URL` 注入通过。
- `/images?type=all|circle|upload&search=` 过滤、白名单、空`[]`通过。
- `/icons/*` 带 `Cache-Control: public, max-age=15768000`，`/../` 穿越 404。
- `/static/*` 404 文案`文件未找到`，`/sw.js`、`/manifest.json` 200。
- 上传多文件成功；`.exe` 拒收；`../escape.png` 收敛为 `escape.png` 落盘 upload 内；删除正常/不存在/穿越语义与原版一致。
- 未验证：`docker build`（环境无 docker）、真实上游 `git clone/pull`（沙箱无大仓条件，仅逻辑平移 + 空仓库返回`无更新`通过）。
