package router

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/entity"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/httptransport"
)

func registerErrorHandlers(router chi.Router) {
	router.NotFound(httptransport.Handle(writeAPINotFound))
	router.MethodNotAllowed(httptransport.Handle(writeAPIMethodNotAllowed))
}

func writeAPINotFound(_ http.ResponseWriter, _ *http.Request) error {
	return entity.NewAppErrorWithMessage(entity.ErrorCodeNotFound, "接口不存在")
}

func writeAPIMethodNotAllowed(_ http.ResponseWriter, _ *http.Request) error {
	return entity.NewAppError(entity.ErrorCodeMethodNotAllowed)
}
