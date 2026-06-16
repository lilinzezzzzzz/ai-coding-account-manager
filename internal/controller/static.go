package controller

import (
	"bytes"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

// StaticController 返回前端静态资源。
type StaticController struct {
	staticFS fs.FS
}

// NewStaticController 创建静态资源 controller。
func NewStaticController(staticFS fs.FS) StaticController {
	return StaticController{staticFS: staticFS}
}

// ServeHTTP 从嵌入的前端资源中返回匹配文件。
func (controller StaticController) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
	if name == "." || name == "" {
		name = "index.html"
	}
	if !fs.ValidPath(name) {
		writeNotFound(w)
		return
	}

	file, err := controller.staticFS.Open(name)
	if errors.Is(err, fs.ErrNotExist) {
		writeNotFound(w)
		return
	}
	if err != nil {
		writeStaticUnavailable(w)
		return
	}
	defer file.Close()

	stat, err := file.Stat()
	if err != nil {
		writeStaticUnavailable(w)
		return
	}
	if stat.IsDir() {
		writeNotFound(w)
		return
	}

	content, err := io.ReadAll(file)
	if err != nil {
		writeStaticUnavailable(w)
		return
	}
	http.ServeContent(w, r, stat.Name(), stat.ModTime(), bytes.NewReader(content))
}

func writeNotFound(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusNotFound)
	_, _ = w.Write([]byte("404 page not found\n"))
}

func writeStaticUnavailable(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusInternalServerError)
	_, _ = w.Write([]byte("static file unavailable\n"))
}
