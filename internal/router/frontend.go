package router

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/httptransport"
)

func registerFrontendRoutes(router chi.Router) {
	staticDir := frontendStaticDir()
	fileServer := http.FileServer(http.Dir(staticDir))
	router.Get("/", fileServer.ServeHTTP)
	router.Get("/*", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			httptransport.WriteError(r.Context(), w, writeAPINotFound(w, r))
			return
		}
		if !staticFileExists(staticDir, r.URL.Path) {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte("404 page not found\n"))
			return
		}
		fileServer.ServeHTTP(w, r)
	})
}

func staticFileExists(staticDir string, requestPath string) bool {
	cleaned := filepath.Clean("/" + requestPath)
	relativePath := strings.TrimPrefix(cleaned, "/")
	if relativePath == "" {
		relativePath = "index.html"
	}
	fullPath := filepath.Join(staticDir, relativePath)
	relativeToStatic, err := filepath.Rel(staticDir, fullPath)
	if err != nil || strings.HasPrefix(relativeToStatic, "..") {
		return false
	}
	info, err := os.Stat(fullPath)
	return err == nil && !info.IsDir()
}

func frontendStaticDir() string {
	workingDir, err := os.Getwd()
	if err != nil {
		return "frontend/static"
	}
	for dir := workingDir; ; dir = filepath.Dir(dir) {
		candidate := filepath.Join(dir, "frontend", "static")
		if _, err := os.Stat(filepath.Join(candidate, "index.html")); err == nil {
			return candidate
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "frontend/static"
		}
	}
}
