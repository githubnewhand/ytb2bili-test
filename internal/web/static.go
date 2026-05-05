package web

import (
	"embed"
	"io/fs"
	"net/http"
	"path/filepath"
	"strings"
)

//go:embed bili-up-web/*
var staticFiles embed.FS

// GetStaticFS 返回嵌入的静态文件系统
func GetStaticFS() fs.FS {
	// 返回 bili-up-web 子目录
	sub, err := fs.Sub(staticFiles, "bili-up-web")
	if err != nil {
		panic("failed to create sub filesystem: " + err.Error())
	}
	return sub
}

// StaticFileHandler 创建一个处理静态文件的 HTTP 处理器
func StaticFileHandler() http.Handler {
	staticFS := GetStaticFS()
	return &staticHandler{fs: staticFS}
}

type staticHandler struct {
	fs fs.FS
}

func (h *staticHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/")
	
	// 如果路径为空，提供 index.html
	if path == "" {
		path = "index.html"
	}
	
	// 对于 SPA 路由，如果文件不存在且不是 API 请求，返回 index.html
	if !strings.HasPrefix(path, "api/") && !strings.HasPrefix(path, "_next/") {
		if _, err := fs.Stat(h.fs, path); err != nil {
			// 文件不存在，检查是否有相应的 HTML 文件
			if !strings.HasSuffix(path, ".html") && !strings.Contains(path, ".") {
				// 尝试添加 .html 后缀
				htmlPath := path + ".html"
				if _, err := fs.Stat(h.fs, htmlPath); err == nil {
					path = htmlPath
				} else {
					// 回退到 index.html (SPA 路由)
					path = "index.html"
				}
			} else {
				path = "index.html"
			}
		}
	}
	
	// 设置正确的 Content-Type
	ext := filepath.Ext(path)
	switch ext {
	case ".html":
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
	case ".js":
		w.Header().Set("Content-Type", "application/javascript")
	case ".css":
		w.Header().Set("Content-Type", "text/css")
	case ".json":
		w.Header().Set("Content-Type", "application/json")
	case ".png":
		w.Header().Set("Content-Type", "image/png")
	case ".jpg", ".jpeg":
		w.Header().Set("Content-Type", "image/jpeg")
	case ".gif":
		w.Header().Set("Content-Type", "image/gif")
	case ".svg":
		w.Header().Set("Content-Type", "image/svg+xml")
	case ".ico":
		w.Header().Set("Content-Type", "image/x-icon")
	}
	
	// 提供文件
	http.FileServer(http.FS(h.fs)).ServeHTTP(w, r)
}