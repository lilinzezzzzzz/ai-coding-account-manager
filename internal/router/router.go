package router

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/controller"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/entity"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/httptransport"
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

func registerErrorHandlers(router chi.Router) {
	router.NotFound(httptransport.Handle(writeAPINotFound))
	router.MethodNotAllowed(httptransport.Handle(writeAPIMethodNotAllowed))
}

func registerAPIRoutes(router chi.Router, cfg Config) {
	healthController := controller.NewHealthController()

	router.Route("/api", func(api chi.Router) {
		registerHealthRoutes(api, healthController)
		if cfg.AccountService != nil {
			registerProviderRoutes(api, controller.NewProviderController(cfg.ProviderService), cfg.SecurityManager)
			registerAccountRoutes(api, controller.NewAccountController(cfg.AccountService), cfg.SecurityManager)
			if cfg.LoginTaskService != nil {
				registerLoginTaskRoutes(api, controller.NewLoginTaskController(cfg.LoginTaskService), cfg.SecurityManager)
			}
		}
	})
}

func registerHealthRoutes(router chi.Router, healthController controller.HealthController) {
	router.Get("/health", httptransport.Handle(healthController.GetHealth))
}

func registerProviderRoutes(router chi.Router, providerController controller.ProviderController, securityManager *security.Manager) {
	router.Get("/providers", httptransport.Handle(providerController.ListProviders))
}

func registerAccountRoutes(router chi.Router, accountController controller.AccountController, securityManager *security.Manager) {
	mutation := []func(http.Handler) http.Handler{
		middleware.RequireOrigin(securityManager),
	}
	jsonMutation := append([]func(http.Handler) http.Handler{}, mutation...)
	jsonMutation = append(jsonMutation, middleware.RequireJSONContentType, middleware.LimitBodySize)
	authJSONMutation := append([]func(http.Handler) http.Handler{}, mutation...)
	authJSONMutation = append(authJSONMutation, middleware.RequireJSONContentType, middleware.LimitBodyBytes(2*1024*1024))

	router.Get("/accounts", httptransport.Handle(accountController.ListAccounts))
	router.With(jsonMutation...).Post("/providers/{providerId}/accounts/create", httptransport.Handle(accountController.CreateAccount))
	router.With(jsonMutation...).Post("/providers/{providerId}/accounts/import-current", httptransport.Handle(accountController.ImportCurrentAccount))
	router.With(authJSONMutation...).Post("/providers/{providerId}/accounts/{accountId}/auth-json/import", httptransport.Handle(accountController.ImportAccountAuthJSON))
	router.With(jsonMutation...).Post("/providers/{providerId}/accounts/{accountId}/activate", httptransport.Handle(accountController.ActivateAccount))
	router.With(jsonMutation...).Post("/providers/{providerId}/accounts/{accountId}/rename", httptransport.Handle(accountController.RenameAccount))
	router.With(jsonMutation...).Post("/providers/{providerId}/accounts/{accountId}/relogin", httptransport.Handle(accountController.ReloginAccount))
	router.With(jsonMutation...).Post("/providers/{providerId}/accounts/{accountId}/refresh", httptransport.Handle(accountController.RefreshAccount))
	router.With(mutation...).Delete("/providers/{providerId}/accounts/{accountId}", httptransport.Handle(accountController.DeleteAccount))
}

func registerLoginTaskRoutes(router chi.Router, loginTaskController controller.LoginTaskController, securityManager *security.Manager) {
	mutation := []func(http.Handler) http.Handler{
		middleware.RequireOrigin(securityManager),
	}
	jsonMutation := append([]func(http.Handler) http.Handler{}, mutation...)
	jsonMutation = append(jsonMutation, middleware.RequireJSONContentType, middleware.LimitBodySize)

	router.With(jsonMutation...).Post("/providers/{providerId}/login-tasks/create", httptransport.Handle(loginTaskController.CreateLoginTask))
	router.Get("/providers/{providerId}/login-tasks/{taskId}", httptransport.Handle(loginTaskController.GetLoginTask))
	router.With(jsonMutation...).Post("/providers/{providerId}/login-tasks/{taskId}/cancel", httptransport.Handle(loginTaskController.CancelLoginTask))
}

func registerFrontendRoutes(router chi.Router) {
	staticDir := frontendStaticDir()
	fileServer := http.FileServer(http.Dir(staticDir))
	router.Get("/", fileServer.ServeHTTP)
	router.Get("/*", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			httptransport.WriteError(w, writeAPINotFound(w, r))
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

func writeAPINotFound(_ http.ResponseWriter, _ *http.Request) error {
	return entity.NewAppErrorWithMessage(entity.ErrorCodeNotFound, "接口不存在")
}

func writeAPIMethodNotAllowed(_ http.ResponseWriter, _ *http.Request) error {
	return entity.NewAppError(entity.ErrorCodeMethodNotAllowed)
}
