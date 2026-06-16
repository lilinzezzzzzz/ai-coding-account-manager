package router

import (
	"fmt"
	"io/fs"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/controller"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/entity"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/middleware"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/transport/httpapi"
)

// NewHandler 创建应用 HTTP 路由。
func NewHandler(staticFS fs.FS) (http.Handler, error) {
	if staticFS == nil {
		return nil, fmt.Errorf("static filesystem is required")
	}

	router := chi.NewRouter()
	router.Use(middleware.SecurityHeaders)
	router.NotFound(func(w http.ResponseWriter, r *http.Request) {
		if isAPIPath(r.URL.Path) {
			httpapi.WriteError(w, entity.NewAppError(entity.ErrorCodeNotFound, "接口不存在"))
			return
		}
		http.NotFound(w, r)
	})
	router.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
		if isAPIPath(r.URL.Path) {
			httpapi.WriteError(w, entity.NewAppError(entity.ErrorCodeMethodNotAllowed, "请求方法不支持"))
			return
		}
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
	})

	healthController := controller.NewHealthController()
	staticController := controller.NewStaticController(staticFS)

	router.Get("/api/health", healthController.Show)
	router.Get("/api", writeAPINotFound)
	router.Get("/api/*", writeAPINotFound)
	router.Get("/*", staticController.ServeHTTP)

	return router, nil
}

func isAPIPath(path string) bool {
	return path == "/api" || len(path) > len("/api/") && path[:len("/api/")] == "/api/"
}

func writeAPINotFound(w http.ResponseWriter, _ *http.Request) {
	httpapi.WriteError(w, entity.NewAppError(entity.ErrorCodeNotFound, "接口不存在"))
}
