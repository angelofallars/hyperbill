package main

import (
	"fmt"
	"log/slog"

	"github.com/angelofallars/hyperbill/app"
	"github.com/angelofallars/hyperbill/internal/service"
)

func main() {
	svcInvoice := service.NewInvoice()

	app := app.New(slog.Default(), svcInvoice).WithPort(3000)

	// Run the server
	err := app.Serve()
	if err != nil {
		fmt.Println(err)
	}
}
