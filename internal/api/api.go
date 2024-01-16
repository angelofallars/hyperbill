package api

import (
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/angelofallars/gotemplate/pkg/trello"
	"github.com/go-chi/chi/v5"
)

type API struct {
	host string
	port int

	slog   *slog.Logger
	router chi.Router

	trelloClient *trello.Client
}

func New(slog *slog.Logger, trelloClient *trello.Client) *API {
	api := &API{
		host: "localhost",
		port: 3000,

		router: chi.NewRouter(),
		slog:   slog,

		trelloClient: trelloClient,
	}

	api.RegisterRoutes()

	return api
}

func (a *API) WithHost(host string) *API {
	a.host = host
	return a
}

func (a *API) WithPort(port uint) *API {
	a.port = int(port)
	return a
}

func (a *API) Serve() error {
	addr := fmt.Sprintf("%s:%d", a.host, a.port)
	server := http.Server{
		Addr:    addr,
		Handler: a.router,

		IdleTimeout:  time.Minute,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	a.slog.Info("server started listening", "addr", addr)

	return server.ListenAndServe()
}
