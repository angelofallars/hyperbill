package app

import (
	"net/http"

	"github.com/angelofallars/hyperbill/app/route/invoice"
	"github.com/go-chi/chi/v5/middleware"
)

func (a *API) RegisterRoutes() {
	a.router.Use(middleware.Logger)

	invoice.NewHandlerGroup().Mount(a.router)

	a.router.Handle("/static/*", http.StripPrefix("/static/", http.FileServer(http.Dir("app/static/"))))
}
