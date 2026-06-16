package router

import (
	"fmt"
	"io/fs"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/controller"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/middleware"
)

// NewHandler 创建应用 HTTP 路由。
func NewHandler(staticFS fs.FS) (http.Handler, error) {
	if staticFS == nil {
		return nil, fmt.Errorf("static filesystem is required")
	}

	router := chi.NewRouter()
	router.Use(middleware.SecurityHeaders)

	healthController := controller.NewHealthController()
	staticController := controller.NewStaticController(staticFS)

	router.Get("/api/health", healthController.Show)
	router.Get("/*", staticController.ServeHTTP)

	return router, nil
}
