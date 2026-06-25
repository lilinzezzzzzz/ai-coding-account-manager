package router

import (
	"github.com/go-chi/chi/v5"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/controller"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/httptransport"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/middleware"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/security"
)

func registerAPIRoutes(router chi.Router, cfg Config) {
	router.Route("/api", func(r chi.Router) {
		registerAPIGroupRoutes(r, cfg)
	})
}

func registerAPIGroupRoutes(router chi.Router, cfg Config) {
	healthController := controller.NewHealthController()
	registerHealthRoutes(router, healthController)

	if cfg.AccountService == nil {
		return
	}

	providerController := controller.NewProviderController(cfg.ProviderService)
	registerProviderRoutes(router, providerController, cfg.SecurityManager)

	accountController := controller.NewAccountController(cfg.AccountService)
	registerAccountRoutes(router, accountController, cfg.SecurityManager)

	if cfg.LoginTaskService == nil {
		return
	}

	loginTaskController := controller.NewLoginTaskController(cfg.LoginTaskService)
	registerLoginTaskRoutes(router, loginTaskController, cfg.SecurityManager)
}

func registerHealthRoutes(router chi.Router, healthController controller.HealthController) {
	router.Get("/health", httptransport.Handle(healthController.GetHealth))
}

func registerProviderRoutes(router chi.Router, providerController controller.ProviderController, securityManager *security.Manager) {
	router.Get("/providers", httptransport.Handle(providerController.ListProviders))
}

func registerAccountRoutes(router chi.Router, accountController controller.AccountController, securityManager *security.Manager) {
	mutation := middleware.Mutation(securityManager)
	jsonMutation := middleware.JSONMutation(securityManager)
	authJSONMutation := middleware.JSONMutationWithLimit(securityManager, 2*1024*1024)

	router.Get("/accounts", httptransport.Handle(accountController.ListAccounts))
	router.With(jsonMutation).Post("/providers/{providerId}/accounts/create", httptransport.Handle(accountController.CreateAccount))
	router.With(jsonMutation).Post("/providers/{providerId}/accounts/import-current", httptransport.Handle(accountController.ImportCurrentAccount))
	router.With(authJSONMutation).Post("/providers/{providerId}/accounts/auth-json/import", httptransport.Handle(accountController.ImportAccountAuthJSONAndRefresh))
	router.With(jsonMutation).Post("/providers/{providerId}/accounts/{accountId}/activate", httptransport.Handle(accountController.ActivateAccount))
	router.With(jsonMutation).Post("/providers/{providerId}/accounts/{accountId}/plan-expiration/update", httptransport.Handle(accountController.UpdatePlanExpiration))
	router.With(jsonMutation).Post("/providers/{providerId}/accounts/{accountId}/relogin", httptransport.Handle(accountController.ReloginAccount))
	router.With(jsonMutation).Post("/providers/{providerId}/accounts/{accountId}/refresh", httptransport.Handle(accountController.RefreshAccount))
	router.With(mutation).Delete("/providers/{providerId}/accounts/{accountId}", httptransport.Handle(accountController.DeleteAccount))
}

func registerLoginTaskRoutes(router chi.Router, loginTaskController controller.LoginTaskController, securityManager *security.Manager) {
	jsonMutation := middleware.JSONMutation(securityManager)

	router.With(jsonMutation).Post("/providers/{providerId}/login-tasks/create", httptransport.Handle(loginTaskController.CreateLoginTask))
	router.Get("/providers/{providerId}/login-tasks/{taskId}", httptransport.Handle(loginTaskController.GetLoginTask))
	router.With(jsonMutation).Post("/providers/{providerId}/login-tasks/{taskId}/cancel", httptransport.Handle(loginTaskController.CancelLoginTask))
}
