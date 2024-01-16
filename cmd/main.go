package main

import (
	"fmt"
	"log"
	"log/slog"

	"github.com/joho/godotenv"

	"github.com/angelofallars/hyperbill/internal/api"
	"github.com/angelofallars/hyperbill/pkg/trello"
)

func main() {
	// Setup
	envMap, err := godotenv.Read(".env")
	if err != nil {
		log.Fatalf("error loading .env file: %s", err)
	}

	trelloAPIKey, ok := envMap["TRELLO_API_KEY"]
	if !ok {
		log.Fatalf("cannot read TRELLO_API_KEY from .env")
	}

	trelloToken, ok := envMap["TRELLO_TOKEN"]
	if !ok {
		log.Fatalf("cannot read TRELLO_TOKEN from .env")
	}

	trelloClient := trello.New(trelloAPIKey, trelloToken)

	api := api.New(slog.Default(), trelloClient).WithPort(3000)

	// Run the server
	err = api.Serve()
	if err != nil {
		fmt.Println(err)
	}
}
