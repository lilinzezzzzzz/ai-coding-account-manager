package router

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/controller"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/entity"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/middleware"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/transport/httpapi"
)

// NewHandler 创建应用 HTTP 路由。
func NewHandler() http.Handler {
	router := chi.NewRouter()
	router.Use(middleware.SecurityHeaders)
	router.NotFound(httpapi.Handle(writeAPINotFound))
	router.MethodNotAllowed(httpapi.Handle(writeAPIMethodNotAllowed))

	healthController := controller.NewHealthController()

	router.Get("/api/health", httpapi.Handle(healthController.GetHealth))

	return router
}

func writeAPINotFound(_ http.ResponseWriter, _ *http.Request) error {
	return entity.NewAppErrorWithMessage(entity.ErrorCodeNotFound, "接口不存在")
}

func writeAPIMethodNotAllowed(_ http.ResponseWriter, _ *http.Request) error {
	return entity.NewAppError(entity.ErrorCodeMethodNotAllowed)
}
