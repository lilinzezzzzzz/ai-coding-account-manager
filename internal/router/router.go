package router

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/controller"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/entity"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/httptransport"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/middleware"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/security"
)

// Config 保存 router 构造参数。
type Config struct {
	SecurityManager *security.Manager
}

// NewHandler 创建应用 HTTP 路由。
func NewHandler(cfg Config) http.Handler {
	router := chi.NewRouter()

	registerMiddlewares(router, cfg.SecurityManager)
	registerErrorHandlers(router)
	registerAPIRoutes(router, cfg.SecurityManager)

	return router
}

func registerMiddlewares(router chi.Router, securityManager *security.Manager) {
	router.Use(middleware.SecurityHeaders)
	router.Use(middleware.RequireHost(securityManager))
}

func registerErrorHandlers(router chi.Router) {
	router.NotFound(httptransport.Handle(writeAPINotFound))
	router.MethodNotAllowed(httptransport.Handle(writeAPIMethodNotAllowed))
}

func registerAPIRoutes(router chi.Router, securityManager *security.Manager) {
	healthController := controller.NewHealthController()
	sessionController := controller.NewSessionController(securityManager)

	router.Route("/api", func(api chi.Router) {
		registerHealthRoutes(api, healthController)
		registerSessionRoutes(api, sessionController, securityManager)
	})
}

func registerHealthRoutes(router chi.Router, healthController controller.HealthController) {
	router.Get("/health", httptransport.Handle(healthController.GetHealth))
}

func registerSessionRoutes(router chi.Router, sessionController controller.SessionController, securityManager *security.Manager) {
	router.With(middleware.RequireSession(securityManager)).
		Get("/session", httptransport.Handle(sessionController.GetSession))

	router.With(middleware.RequireJSONContentType, middleware.LimitBodySize).
		Post("/session/bootstrap", httptransport.Handle(sessionController.ExchangeBootstrap))
}

func writeAPINotFound(_ http.ResponseWriter, _ *http.Request) error {
	return entity.NewAppErrorWithMessage(entity.ErrorCodeNotFound, "接口不存在")
}

func writeAPIMethodNotAllowed(_ http.ResponseWriter, _ *http.Request) error {
	return entity.NewAppError(entity.ErrorCodeMethodNotAllowed)
}
