package main

import (
	"context"
	"log"
	"os/signal"
	"syscall"

	"github.com/bengobox/food-delivery-backend/internal/app"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	a, err := app.New(ctx)
	if err != nil {
		log.Fatalf("failed to initialise application: %v", err)
	}
	defer a.Close()

	if err := a.Run(ctx); err != nil {
		log.Fatalf("application runtime error: %v", err)
	}
}
