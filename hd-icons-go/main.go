package main

import (
	"bytes"
	"embed"
	"encoding/json"
	"html/template"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"io/fs"
	"log"
	"mime"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/disintegration/imaging"
	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/tiff"
	_ "golang.org/x/image/webp"
)

//go:embed templates/index.html
var templatesFS embed.FS

//go:embed static
var staticFS embed.FS

var (
	iconsDir      string
	port          string
	title         string
	customURL     string
	fontDirExt    string // 外部字体挂载目录，默认 /app/static/font
	repoURL       string // 图标库 git 地址，默认官方 GitHub，可经 ICONS_REPO_URL 指向镜像
	fallbackURL   string // 镜像失败时的回退地址，默认官方 GitHub（ICONS_REPO_FALLBACK 可改备选镜像）
	usingFallback bool   // 本次进程内已回退，ensureRemote 不再切回故障镜像
	updateMu      sync.Mutex
	indexTmpl     *template.Template
)

const defaultRepoURL = "https://github.com/xushier/HD-Icons.git"

var (
	allTypes = []string{"border-radius", "circle", "svg", "upload"}
	// /images 白名单（与 app.py 一致）
	listExts = map[string]bool{
		".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".webp": true,
		".svg": true, ".bmp": true, ".tiff": true, ".apng": true, ".ico": true, ".tif": true,
	}
	// 缩略图白名单（与 app.py 一致：跳过 svg/apng）
	thumbExts = map[string]bool{
		".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".webp": true,
		".bmp": true, ".tiff": true, ".ico": true, ".tif": true,
	}
)

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func imageDirFor(t string) string {
	if t == "upload" {
		return filepath.Join(iconsDir, "upload")
	}
	return filepath.Join(iconsDir, "HD-Icons", t)
}

// ---------- 缩略图 ----------

func copyFile(src, dst string) bool {
	in, err := os.Open(src)
	if err != nil {
		return false
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return false
	}
	out, err := os.Create(dst)
	if err != nil {
		return false
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return false
	}
	return true
}

func generateThumbnail(imagePath, thumbnailPath string) bool {
	src, err := imaging.Open(imagePath, imaging.AutoOrientation(true))
	if err != nil {
		// ico/webp 编码不支持等情况：退化为直接复制原图，保证缩略图可用
		log.Printf("缩略图 decode 失败，回退复制: %s: %v", imagePath, err)
		return copyFile(imagePath, thumbnailPath)
	}
	dst := imaging.Fit(src, 128, 128, imaging.Lanczos)
	if err := os.MkdirAll(filepath.Dir(thumbnailPath), 0o755); err != nil {
		return false
	}
	if err := imaging.Save(dst, thumbnailPath); err != nil {
		log.Printf("缩略图 save 失败，回退复制: %s: %v", thumbnailPath, err)
		return copyFile(imagePath, thumbnailPath)
	}
	return true
}

func generateThumbnailsForAllImages() {
	for _, t := range allTypes {
		srcDir := imageDirFor(t)
		dstDir := filepath.Join(iconsDir, "thumbnails", t)
		if _, err := os.Stat(srcDir); err != nil {
			log.Printf("文件夹 %s 不存在，跳过生成缩略图。", srcDir)
			continue
		}
		if err := os.MkdirAll(dstDir, 0o755); err != nil {
			log.Printf("创建缩略图目录失败 %s: %v", dstDir, err)
			continue
		}
		entries, err := os.ReadDir(srcDir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			if !thumbExts[strings.ToLower(filepath.Ext(e.Name()))] {
				continue
			}
			dst := filepath.Join(dstDir, e.Name())
			if _, err := os.Stat(dst); err == nil {
				continue
			}
			log.Printf("生成缩略图: %s", dst)
			generateThumbnail(filepath.Join(srcDir, e.Name()), dst)
		}
	}
}

// ---------- git 同步 ----------

func proxyEnv() string {
	for _, k := range []string{"ALL_PROXY", "all_proxy", "HTTPS_PROXY", "https_proxy", "HTTP_PROXY", "http_proxy"} {
		if v := os.Getenv(k); v != "" {
			return v
		}
	}
	return ""
}

func applyProxy() {
	p := proxyEnv()
	if p == "" {
		return
	}
	exec.Command("git", "config", "--global", "http.proxy", p).Run()
	exec.Command("git", "config", "--global", "https.proxy", p).Run()
}

func hdIconsDir() string { return filepath.Join(iconsDir, "HD-Icons") }

// gitOrigin 返回本地仓库当前 origin，无 origin 返回 ""
func gitOrigin() string {
	out, err := exec.Command("git", "-C", hdIconsDir(), "remote", "get-url", "origin").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// setOrigin 切换本地仓库 origin，失败返回 err
func setOrigin(url string) error {
	if gitOrigin() == "" {
		return exec.Command("git", "-C", hdIconsDir(), "remote", "add", "origin", url).Run()
	}
	return exec.Command("git", "-C", hdIconsDir(), "remote", "set-url", "origin", url).Run()
}

// ensureRemote 使本地仓库 origin 与配置的 repoURL 一致：
// 用户切换镜像后无需删卷重建，重启即生效，后续 pull 走新源。
// 若本进程内已因镜像故障回退，则保持回退源不再切回。
func ensureRemote() {
	updateMu.Lock()
	defer updateMu.Unlock()
	if usingFallback {
		return
	}
	if cur := gitOrigin(); cur == "" {
		if err := setOrigin(repoURL); err != nil {
			log.Printf("设置 git origin 失败: %v", err)
		}
		return
	} else if cur != repoURL {
		log.Printf("图标库源变更: %s -> %s", cur, repoURL)
		if err := setOrigin(repoURL); err != nil {
			log.Printf("切换 git origin 失败: %v", err)
		}
	}
}

// gitClone 按 GIT_DEPTH 从 url 克隆
func gitClone(url string) error {
	args := []string{"clone"}
	if depth := getenv("GIT_DEPTH", "1"); depth != "" && depth != "0" {
		args = append(args, "--depth", depth)
	}
	args = append(args, url, hdIconsDir())
	cmd := exec.Command("git", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func initIconsRepo() {
	if err := os.MkdirAll(iconsDir, 0o755); err != nil {
		log.Fatalf("创建 icons 目录失败: %v", err)
	}
	if _, err := os.Stat(hdIconsDir()); err != nil {
		log.Println("icons 文件夹为空，正在克隆 HD-Icons 仓库...")
		applyProxy()
		if err := gitClone(repoURL); err != nil {
			// 镜像源失败则回退官方源重试，仅配置了非默认源时触发
			if repoURL != fallbackURL {
				log.Printf("图标库源 %s 克隆失败，回退 %s: %v", repoURL, fallbackURL, err)
				updateMu.Lock()
				usingFallback = true
				updateMu.Unlock()
				if err2 := gitClone(fallbackURL); err2 != nil {
					log.Printf("git clone 失败: %v（服务继续启动，可稍后手动检查更新）", err2)
				}
			} else {
				log.Printf("git clone 失败: %v（服务继续启动，可稍后手动检查更新）", err)
			}
		}
	} else {
		ensureRemote()
	}
	if err := os.MkdirAll(filepath.Join(iconsDir, "upload"), 0o755); err != nil {
		log.Fatalf("创建 upload 目录失败: %v", err)
	}
	generateThumbnailsForAllImages()
}

// 返回 true=有更新
func checkForUpdates() bool {
	updateMu.Lock()
	defer updateMu.Unlock()
	if _, err := os.Stat(hdIconsDir()); err != nil {
		log.Printf("检查更新时 HD-Icons 目录不存在: %v", err)
		return false
	}
	applyProxy()
	out, err := exec.Command("git", "-C", hdIconsDir(), "pull").CombinedOutput()
	if err != nil {
		// 镜像源 pull 失败则切回退源重试一次，仅当前源非回退源时触发
		if !usingFallback && gitOrigin() != "" && gitOrigin() != fallbackURL {
			log.Printf("图标库源 pull 失败，回退 %s: %s", fallbackURL, strings.TrimSpace(string(out)))
			if err2 := setOrigin(fallbackURL); err2 != nil {
				log.Printf("回退 git origin 失败: %v", err2)
				return false
			}
			usingFallback = true
			out, err = exec.Command("git", "-C", hdIconsDir(), "pull").CombinedOutput()
			if err != nil {
				log.Printf("回退源 pull 依然失败: %v, 输出: %s", err, string(out))
				return false
			}
			log.Printf("已回退到 %s，后续更新走回退源（重启后恢复配置源）。", fallbackURL)
		} else {
			log.Printf("检查更新时出错: %v, 输出: %s", err, string(out))
			return false
		}
	}
	if strings.Contains(string(out), "Already up to date.") {
		return false
	}
	log.Println("检测到更新，图标库已更新。")
	generateThumbnailsForAllImages()
	return true
}

func parseInterval(s string) time.Duration {
	if s == "" {
		return time.Hour
	}
	if d, err := time.ParseDuration(s); err == nil && d > 0 {
		return d
	}
	return time.Hour
}

func autoCheckUpdates(interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for range t.C {
		if checkForUpdates() {
			log.Println("定时更新完成，已补缩略图。")
		}
	}
}

// ---------- handlers ----------

type indexData struct {
	Title     string
	CustomURL string
}

func handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	if _, err := os.Stat(filepath.Join(iconsDir, "thumbnails")); err != nil {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("正在生成缩略图，请稍后刷新页面..."))
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = indexTmpl.Execute(w, indexData{Title: title, CustomURL: customURL})
}

func handleHealthz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}

func serveEmbedFile(w http.ResponseWriter, r *http.Request, fsPath string) {
	data, err := staticFS.ReadFile(fsPath)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("文件未找到"))
		return
	}
	ctype := mime.TypeByExtension(filepath.Ext(fsPath))
	if ctype == "" {
		ctype = http.DetectContentType(data)
	}
	if strings.HasPrefix(ctype, "text/") && !strings.Contains(ctype, "charset") {
		ctype += "; charset=utf-8"
	}
	w.Header().Set("Content-Type", ctype)
	http.ServeContent(w, r, path.Base(fsPath), time.Now(), bytes.NewReader(data))
}

func handleStatic(w http.ResponseWriter, r *http.Request) {
	rel := strings.TrimPrefix(r.URL.Path, "/static/")
	rel = path.Clean("/" + rel)[1:]
	if strings.Contains(rel, "..") {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("文件未找到"))
		return
	}
	// 外部字体挂载优先：/static/font/* -> fontDirExt
	if strings.HasPrefix(rel, "font/") && fontDirExt != "" {
		extPath := filepath.Join(fontDirExt, strings.TrimPrefix(rel, "font/"))
		if st, err := os.Stat(extPath); err == nil && !st.IsDir() {
			http.ServeFile(w, r, extPath)
			return
		}
	}
	serveEmbedFile(w, r, "static/"+rel)
}

func handleSW(w http.ResponseWriter, r *http.Request) {
	serveEmbedFile(w, r, "static/js/sw.js")
}

func handleManifest(w http.ResponseWriter, r *http.Request) {
	serveEmbedFile(w, r, "static/manifest.json")
}

func handleIcons(w http.ResponseWriter, r *http.Request) {
	rel := strings.TrimPrefix(r.URL.Path, "/icons/")
	rel = path.Clean("/" + rel)[1:]
	full := filepath.Join(iconsDir, filepath.FromSlash(rel))
	// 防穿越：必须落在 iconsDir 内
	absBase, _ := filepath.Abs(iconsDir)
	absFull, _ := filepath.Abs(full)
	if absFull != absBase && !strings.HasPrefix(absFull, absBase+string(os.PathSeparator)) {
		http.NotFound(w, r)
		return
	}
	st, err := os.Stat(absFull)
	if err != nil || st.IsDir() {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Cache-Control", "public, max-age=15768000")
	http.ServeFile(w, r, absFull)
}

type imageItem struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

func handleImages(w http.ResponseWriter, r *http.Request) {
	imageType := strings.ToLower(r.URL.Query().Get("type"))
	if imageType == "" {
		imageType = "all"
	}
	search := strings.ToLower(r.URL.Query().Get("search"))
	var images []imageItem
	collect := func(t string) {
		dir := imageDirFor(t)
		entries, err := os.ReadDir(dir)
		if err != nil {
			return
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			name := e.Name()
			if !listExts[strings.ToLower(filepath.Ext(name))] {
				continue
			}
			if search != "" && !strings.Contains(strings.ToLower(name), search) {
				continue
			}
			images = append(images, imageItem{Name: name, Type: t})
		}
	}
	if imageType == "all" {
		for _, t := range allTypes {
			collect(t)
		}
	} else {
		valid := false
		for _, t := range allTypes {
			if t == imageType {
				valid = true
				break
			}
		}
		if valid {
			collect(imageType)
		}
	}
	if images == nil {
		images = []imageItem{}
	}
	sort.Slice(images, func(i, j int) bool {
		if images[i].Type == images[j].Type {
			return images[i].Name < images[j].Name
		}
		return images[i].Type < images[j].Type
	})
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(images)
}

func handleCheckUpdate(w http.ResponseWriter, r *http.Request) {
	updated := checkForUpdates()
	w.Header().Set("Content-Type", "application/json")
	if updated {
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "有更新"})
	} else {
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "无更新"})
	}
}

func sanitizeFileName(raw string) (string, bool) {
	base := path.Base(strings.ReplaceAll(raw, "\\", "/"))
	base = strings.TrimSpace(base)
	if base == "" || base == "." || base == "/" {
		return "", false
	}
	if strings.Contains(base, "..") || strings.Contains(base, "/") {
		return "", false
	}
	ext := strings.ToLower(filepath.Ext(base))
	if !listExts[ext] {
		return "", false
	}
	if len(base) > 255 {
		return "", false
	}
	return base, true
}

var _ = image.Rect // 占位，保证 image 包注册意图明确

func handleUpload(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "error", "message": "仅支持 POST"})
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 50<<20)
	if err := r.ParseMultipartForm(50 << 20); err != nil {
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "error", "message": "没有文件上传"})
		return
	}
	files := r.MultipartForm.File["file"]
	if len(files) == 0 {
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "error", "message": "没有选择文件"})
		return
	}
	uploadDir := filepath.Join(iconsDir, "upload")
	if err := os.MkdirAll(uploadDir, 0o755); err != nil {
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "error", "message": "创建上传目录失败"})
		return
	}
	updateMu.Lock()
	defer updateMu.Unlock()
	saved := 0
	for _, fh := range files {
		if fh.Filename == "" {
			continue
		}
		name, ok := sanitizeFileName(fh.Filename)
		if !ok {
			log.Printf("跳过不支持的文件名: %s", fh.Filename)
			continue
		}
		src, err := fh.Open()
		if err != nil {
			continue
		}
		dstPath := filepath.Join(uploadDir, name)
		dst, err := os.Create(dstPath)
		if err != nil {
			src.Close()
			continue
		}
		_, err = io.Copy(dst, src)
		dst.Close()
		src.Close()
		if err != nil {
			continue
		}
		saved++
		// 增量缩略图
		if thumbExts[strings.ToLower(filepath.Ext(name))] {
			thumbPath := filepath.Join(iconsDir, "thumbnails", "upload", name)
			if _, err := os.Stat(thumbPath); err != nil {
				generateThumbnail(dstPath, thumbPath)
			}
		}
	}
	if saved == 0 {
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "error", "message": "文件格式不支持"})
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "success", "message": "文件上传成功"})
}

func handleDelete(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "error", "message": "仅支持 POST"})
		return
	}
	var data struct {
		ImageName string `json:"image_name"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&data); err != nil {
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "error", "message": "文件不存在"})
		return
	}
	name, ok := sanitizeFileName(data.ImageName)
	if !ok {
		// 兼容原版无后缀校验但防穿越：至少做 Base 清洗
		base := path.Base(strings.ReplaceAll(data.ImageName, "\\", "/"))
		if base == "" || base == "." || strings.Contains(base, "..") || strings.Contains(base, "/") {
			_ = json.NewEncoder(w).Encode(map[string]string{"status": "error", "message": "文件不存在"})
			return
		}
		name = base
	}
	updateMu.Lock()
	defer updateMu.Unlock()
	uploadDir := filepath.Join(iconsDir, "upload")
	origPath := filepath.Join(uploadDir, name)
	thumbPath := filepath.Join(iconsDir, "thumbnails", "upload", name)
	success := false
	if _, err := os.Stat(origPath); err == nil {
		if err := os.Remove(origPath); err != nil {
			_ = json.NewEncoder(w).Encode(map[string]string{"status": "error", "message": "删除原始文件失败"})
			return
		}
		success = true
	}
	if _, err := os.Stat(thumbPath); err == nil {
		if err := os.Remove(thumbPath); err != nil {
			_ = json.NewEncoder(w).Encode(map[string]string{"status": "error", "message": "删除缩略图文件失败"})
			return
		}
		success = true
	}
	if !success {
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "error", "message": "文件不存在"})
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "success"})
}

// ---------- 中间件 ----------

func withRecovery(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				log.Printf("panic: %v", rec)
				http.Error(w, "内部错误", http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func withLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(start))
	})
}

func main() {
	iconsDir = getenv("ICONS_DIR", "/app/icons")
	port = getenv("PORT", "50560")
	title = getenv("TITLE", "小迪的图标库")
	customURL = strings.TrimSuffix(getenv("CUSTOM_URL", ""), "/")
	fontDirExt = getenv("FONT_DIR", "/app/static/font")
	repoURL = strings.TrimSpace(getenv("ICONS_REPO_URL", defaultRepoURL))
	if repoURL == "" {
		repoURL = defaultRepoURL
	}
	fallbackURL = strings.TrimSpace(getenv("ICONS_REPO_FALLBACK", defaultRepoURL))
	if fallbackURL == "" {
		fallbackURL = defaultRepoURL
	}

	// 模板解析
	tplBytes, err := templatesFS.ReadFile("templates/index.html")
	if err != nil {
		log.Fatalf("读取模板失败: %v", err)
	}
	indexTmpl, err = template.New("index").Parse(string(tplBytes))
	if err != nil {
		log.Fatalf("解析模板失败: %v", err)
	}
	// 确保 embed 完整性
	if _, err := fs.Sub(staticFS, "static"); err != nil {
		log.Fatalf("静态资源缺失: %v", err)
	}

	initIconsRepo()

	interval := parseInterval(getenv("UPDATE_INTERVAL", "1h"))
	go autoCheckUpdates(interval)

	mux := http.NewServeMux()
	mux.HandleFunc("/", handleIndex)
	mux.HandleFunc("/healthz", handleHealthz)
	mux.HandleFunc("/static/", handleStatic)
	mux.HandleFunc("/sw.js", handleSW)
	mux.HandleFunc("/manifest.json", handleManifest)
	mux.HandleFunc("/icons/", handleIcons)
	mux.HandleFunc("/images", handleImages)
	mux.HandleFunc("/check-update", handleCheckUpdate)
	mux.HandleFunc("/upload-image", handleUpload)
	mux.HandleFunc("/delete-image", handleDelete)

	addr := "0.0.0.0:" + port
	log.Printf("HD-Icons Go 版启动: %s, icons=%s, title=%s", addr, iconsDir, title)
	srv := &http.Server{
		Addr:         addr,
		Handler:      withRecovery(withLogging(mux)),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
	}
	log.Fatal(srv.ListenAndServe())
}
