package router

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/middleware"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/security"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/service"
)

// Config 保存 router 构造参数。
type Config struct {
	SecurityManager  *security.Manager
	ProviderService  service.ProviderService
	AccountService   *service.AccountService
	LoginTaskService *service.LoginTaskService
}

// NewHandler 创建应用 HTTP 路由。
func NewHandler(cfg Config) http.Handler {
	router := chi.NewRouter()

	registerMiddlewares(router, cfg.SecurityManager)
	registerErrorHandlers(router)
	registerAPIRoutes(router, cfg)
	registerFrontendRoutes(router)

	return router
}

func registerMiddlewares(router chi.Router, securityManager *security.Manager) {
	router.Use(middleware.SecurityHeaders)
	router.Use(middleware.RequireHost(securityManager))
}
