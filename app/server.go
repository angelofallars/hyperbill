package app

import (
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/angelofallars/hyperbill/pkg/trello"
	"github.com/go-chi/chi/v5"
)

type App struct {
	host string
	port int

	slog   *slog.Logger
	router chi.Router

	trelloClient *trello.Client
}

func New(slog *slog.Logger, trelloClient *trello.Client) *App {
	app := &App{
		host: "localhost",
		port: 3000,

		router: chi.NewRouter(),
		slog:   slog,

		trelloClient: trelloClient,
	}

	app.RegisterRoutes()

	return app
}

func (a *App) WithHost(host string) *App {
	a.host = host
	return a
}

func (a *App) WithPort(port uint) *App {
	a.port = int(port)
	return a
}

func (a *App) Serve() error {
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
