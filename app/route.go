package app

import (
	"net/http"

	"github.com/angelofallars/hyperbill/app/route/invoice"
	"github.com/go-chi/chi/v5/middleware"
)

func (a *App) RegisterRoutes() {
	a.router.Use(middleware.Logger)
	a.router.Use(middleware.Recoverer)

	invoice.NewHandlerGroup(a.svcInvoice).Mount(a.router)

	a.router.Handle("/static/*", http.StripPrefix("/static/", http.FileServer(http.Dir("app/static/"))))
}
