package main

import (
	"context"
	"log"
	"os/signal"
	"syscall"

	_ "github.com/bengobox/ordering-backend/internal/http/docs"

	"github.com/bengobox/ordering-backend/internal/app"
)

// @title Ordering Service API
// @version 0.1.0
// @description HTTP API for the Codevertex ordering service - online delivery/shipping orders only.
// @BasePath /api/v1
// @schemes http https
// @securityDefinitions.apikey bearerAuth
// @in header
// @name Authorization
// @description JWT token from auth-service. Format: Bearer {token}
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
