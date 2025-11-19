package main

import (
	"context"
	"log"
	"os/signal"
	"syscall"

	_ "github.com/bengobox/cafe-backend/internal/http/docs"

	"github.com/bengobox/cafe-backend/internal/app"
)

// @title Cafe Backend API
// @version 0.1.0
// @description HTTP API for the BengoBox food delivery backend service.
// @BasePath /api/v1
// @Schemes http
// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
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
