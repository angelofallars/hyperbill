package main

import (
	"fmt"
	"log/slog"

	"github.com/angelofallars/hyperbill/app"
)

func main() {
	app := app.New(slog.Default()).WithPort(3000)

	// Run the server
	err := app.Serve()
	if err != nil {
		fmt.Println(err)
	}
}
