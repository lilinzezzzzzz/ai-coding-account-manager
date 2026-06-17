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

	registerMiddlewares(router)
	registerErrorHandlers(router)
	registerAPIRoutes(router)

	return router
}

func registerMiddlewares(router chi.Router) {
	router.Use(middleware.SecurityHeaders)
}

func registerErrorHandlers(router chi.Router) {
	router.NotFound(httpapi.Handle(writeAPINotFound))
	router.MethodNotAllowed(httpapi.Handle(writeAPIMethodNotAllowed))
}

func registerAPIRoutes(router chi.Router) {
	healthController := controller.NewHealthController()

	router.Route("/api", func(api chi.Router) {
		registerHealthRoutes(api, healthController)
	})
}

func registerHealthRoutes(router chi.Router, healthController controller.HealthController) {
	router.Get("/health", httpapi.Handle(healthController.GetHealth))
}

func writeAPINotFound(_ http.ResponseWriter, _ *http.Request) error {
	return entity.NewAppErrorWithMessage(entity.ErrorCodeNotFound, "接口不存在")
}

func writeAPIMethodNotAllowed(_ http.ResponseWriter, _ *http.Request) error {
	return entity.NewAppError(entity.ErrorCodeMethodNotAllowed)
}
