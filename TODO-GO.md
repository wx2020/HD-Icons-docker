# HD-Icons-docker Go 化 ToDo（对齐原 Python 版全部功能）

> 范围：`hd-icons/app.py(226行) + templates/index.html + static(js/css/fonts/assets/manifest) + Dockerfile/docker-compose.yml + README` 的全量行为。
> 目标：单二进制 Go 服务 + 多阶段构建小镜像，API 与前端行为 1:1 兼容。

## P0 — 项目骨架与构建
- [ ] 新建 `hd-icons-go/`（或原地替换）：`main.go`、`go.mod`、`Dockerfile`、`docker-compose.yml`
- [ ] 多阶段构建：`golang:1.22-bookworm AS builder` → `debian:bookworm-slim / alpine / scratch` 运行态
- [ ] 运行态包含 `git`（或改用 `go-git` 纯 Go 实现则可去 git，具体见 P2）
- [ ] `WORKDIR /app`，`VOLUME /app/icons`，`EXPOSE 50560`，`CMD ["/app/hd-icons"]` 保持与原容器挂载/端口兼容
- [ ] `static/`、`templates/` 用 `go:embed` 打入二进制，同时兼容 `/app/static/font` 外部字体挂载覆盖
- [ ] 环境变量统一：`PORT`(默认50560)、`ICONS_DIR`(默认`/app/icons`)、`ALL_PROXY/all_proxy` 兼容大小写、`CUSTOM_URL`、`TITLE`（README 有、现 app.py 缺失，本次补齐）
- [ ] 健康检查：`GET /healthz`（新增，不影响旧 API）

## P1 — 路由与静态服务（对齐 Flask）
- [ ] `GET /` → 渲染 `templates/index.html`；若 `icons/thumbnails` 未就绪返回 `正在生成缩略图，请稍后刷新页面...`（与原 `index()` 一致）
- [ ] `GET /static/<path>` → 静态文件服务（对齐 `serve_static`，含 404 文案 `文件未找到`）
- [ ] `GET /sw.js` → 兼容现前端 `navigator.serviceWorker.register('/sw.js')`（原文件在 `/static/js/sw.js`，Flask 并无 `/sw.js` 路由，Go 版需补 301/别名，否则 PWA 注册 404）
- [ ] `GET /icons/<path>` → `http.ServeContent` + 响应头 `Cache-Control: public, max-age=15768000`（半年缓存，与 `serve_icons` 一致）
- [ ] `GET /manifest.json` / `GET /static/manifest.json` 双路径兼容（`index.html` 引的是后者，PWA 规范常用前者）
- [ ] `TITLE` 标题注入：模板 `小迪的图标库` 改为 `{{.Title}}`，默认 `小迪的图标库`
- [ ] `CUSTOM_URL` 复制前缀：与前端约定（默认空=同源 `window.location.origin`，云端模式用该前缀；需同时改 `xdtx.js` 取后端注入的 `window.CUSTOM_URL`）
- [ ] 请求日志 + `panic` 恢复中间件（替代 gunicorn 的 worker 日志能力）

## P1 — 图片列表 API `GET /images?type=&search=`
- [ ] 参数：`type=all|border-radius|circle|svg|upload`（大小写不敏感，默认 `all`），`search` 子串匹配（大小写不敏感）
- [ ] `all` 合并 4 子目录并返回 `{"name","type"}`；单类只读对应目录；目录不存在返回 `[]`（不对 500）
- [ ] 后缀白名单与原版完全一致：`png,jpg,jpeg,gif,webp,svg,bmp,tiff,apng,ico,tif`
- [ ] `Content-Type: application/json`，空结果 `[]`；排序与 `os.listdir` 一致（建议显式 `sort.Strings` 保证确定性）
- [ ] 前端 URL 规则不变：SVG 走原图 `/icons/HD-Icons/<type>/`，非 SVG 走 `/icons/thumbnails/<type>/`，`upload` 类走 `/icons/upload` 与 `/icons/thumbnails/upload`

## P1 — 上传 `POST /upload-image`
- [ ] `multipart field=file` 多文件（`r.MultipartReader`，与 `request.files.getlist('file')` 一致）
- [ ] 服务端二次校验后缀白名单（前端 `allowedFormats` 可绕过，不可只靠前端）
- [ ] 文件名安全：`filepath.Base + Clean`，拒绝 `../` 路径穿越；重名策略与原版一致（直接覆盖）或改为记录日志
- [ ] 落盘到 `<ICONS_DIR>/upload/`，不存在则 `MkdirAll`
- [ ] 增量生成缩略图（原版全量再生太慢，Go 版改为仅处理本次上传文件；行为兼容、性能优化）
- [ ] 返回 `{"status":"success","message":"文件上传成功"}`；无文件时 `{"status":"error","message":"没有文件上传/没有选择文件"}`，状态码与原版对齐
- [ ] 限制：`MaxBytesReader`（如 50MB）、超时、并发上传互斥

## P1 — 删除 `POST /delete-image`
- [ ] `JSON {"image_name"}`，仅允许 `upload` 分类（拒绝 `../`、`/`、`HD-Icons` 越权删除）
- [ ] 同时删原图 `<ICONS_DIR>/upload/<name>` + 缩略图 `<ICONS_DIR>/thumbnails/upload/<name>`
- [ ] 返回语义与原版一致：`{status:success}` / `{status:error,message:文件不存在|删除原始文件失败|删除缩略图文件失败}`
- [ ] 删除后前端 `loadImages()` 刷新行为不变

## P1 — 缩略图管线（对齐 Pillow）
- [ ] 启动时 `generate_thumbnails_for_all_images`：遍历 `border-radius/circle/svg/upload`，缺目录则跳过+日志
- [ ] 仅对 `png,jpg,jpeg,gif,webp,bmp,tiff,ico,tif` 生成（跳过 `svg/apng` 与原版一致），已存在则跳过
- [ ] 等比缩放 `max 128x128`（对齐 `PIL.Image.thumbnail((128,128))`），保留宽高比与格式
- [ ] Go 库选型：`disintegration/imaging + golang.org/x/image/{webp,tiff,bmp}`，`ico` 需第三方解码；全量格式矩阵测试通过
- [ ] 上传后增量缩略图；`git pull` 更新后增量补缩略图（原版缺失，本次补上否则新图无缩略图）

## P2 — 图标库同步（对齐 git 逻辑 + 修 README 不一致）
- [ ] 启动 `init_icons_repo`：`ICONS_DIR` 不存在则创建；`HD-Icons` 不存在则 `git clone https://github.com/xushier/HD-Icons.git`；`upload/` 不存在则创建
- [ ] `GET /check-update` → `git -C <ICONS_DIR>/HD-Icons pull`，含 `Already up to date.` 判断，返回 `{"status":"有更新"/"无更新"}`（与原 `check_update` 一致）
- [ ] 定时任务：原代码 `86400s(每天)`、README `每小时`、前端 `setInterval 3600000(每小时)` 三处打架 → 统一为 env `UPDATE_INTERVAL`(默认 `1h`)，`time.Ticker + goroutine` 实现
- [ ] 代理：同时读取 `ALL_PROXY`/`all_proxy`/`HTTP_PROXY`，`git config --global http(s).proxy` 或注入 `HTTPS_PROXY` 环境；文档与实现统一
- [ ] `pull` 后自动补缩略图 + 并发锁（避免与上传/列表竞态）
- [ ] 手动 `check-update` 与定时共用同一把锁 + 同一函数

## P3 — 前端原样移植（templates + static）
- [ ] `index.html`：社交链接(GitHub/B站/微信/YouTube/抖音/什么值得买)、标题、搜索框、分类按钮(所有/圆角/圆形/矢量/自定)、`#status`、`#image-container(grid)`、上传弹窗(`drop-zone/file-list/progress-bar`)、页脚、PWA `manifest` 全部保留
- [ ] `xdtx.js` 全部函数保留：`setImageType/updateButtonState/loadImages/initLazyLoad/updateStatus/handleKeyPress/toggleMode/autoToggleMode/checkForUpdates/autoCheckUpdates/showUploadPopup/closeUploadPopup/handleFileSelect/updateFileList/uploadFiles/showPreviewPopup/showDeleteConfirmation/deleteImage/closeDeletePopup/DOMContentLoaded`
- [ ] 日/夜间模式 + `prefers-color-scheme` 自动切换保留；`xdtx.css` 原样搬运
- [ ] 单击复制(`ClipboardJS`)、放大预览、自定义类删除二次确认、拖拽上传+进度条保留
- [ ] CDN(`clipboard/tailwind/font-awesome`)本地化备选（README v4.4 要求无魔法环境可用，原 `index.html` 仍走 CDN，需收敛）
- [ ] `sw.js` 缓存 `icon-library-v1`、图片正则 `png|jpg|svg|jpeg|ico|gif|webp|apng|tif|tiff|bmp` 保留；修正其 `ASSETS_TO_CACHE` 中不存在的 `/static/css/styles.css`、`/static/js/app.js` 引用
- [ ] `manifest.json(name/short_name/icons 192/512)`、`assets(favicon/hd-icons/icon-*)`、`fonts(font.ttf/ZTQXinYiJiXiangSong.TTF)` 原样搬运；自定义字体挂载 `/app/static/font/font_zh.ttf + font_en.ttf` 生效逻辑补齐

## P4 — 容器与文档
- [ ] 重写根 `README` 的 `docker run / docker-compose / Unraid` 示例，镜像名、端口 `50560`、挂载 `/app/icons`、可选 `/app/static/font` 不变
- [ ] `docker-compose.yml` 补 `ALL_PROXY/CUSTOM_URL/TITLE/UPDATE_INTERVAL` 注释示例（与 Go 版 env 对齐）
- [ ] 镜像体积/启动耗时对比记录（`python:3.9-slim` vs Go 多阶段产物）
- [ ] `preview/` 截图回归：日间/夜间/移动端/复制/上传 5 张逐一核对

## P5 — 验证（DoD）
- [ ] `go vet + gofmt + golangci-lint` 通过
- [ ] 接口测试：`/images` 各 type+search、`/upload-image` 多文件+非法后缀、`delete-image` 正常/不存在/穿越、`/check-update` 有/无更新、`/icons/*` 缓存头、`/` 未就绪文案
- [ ] 前端回归：分类切换、搜索、复制、放大、上传、删除、日夜切换、PWA 离线
- [ ] 容器回归：首次 `clone` 等待、断网+代理、重启后缩略图复用、`docker-compose` 一键起
- [ ] 体积目标：最终镜像 ≤50MB（含 git）或 ≤25MB（`go-git` 方案）
