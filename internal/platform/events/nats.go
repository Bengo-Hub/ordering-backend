package events

import (
	"context"
	"fmt"
	"time"

	"github.com/nats-io/nats.go"

	"github.com/bengobox/ordering-backend/internal/config"
)

// Connect establishes a NATS connection with sane defaults.
func Connect(cfg config.EventsConfig) (*nats.Conn, error) {
	opts := []nats.Option{
		nats.Name("ordering-backend"),
		nats.Timeout(5 * time.Second),
		nats.ReconnectWait(2 * time.Second),
		nats.MaxReconnects(-1),
	}

	return nats.Connect(cfg.NATSURL, opts...)
}

// EnsureStream creates the JetStream stream for ordering events if it doesn't exist.
// This must be called on startup so downstream consumers (inventory-api, logistics-api,
// notifications-api) can reliably subscribe to ordering.* subjects with durability.
func EnsureStream(ctx context.Context, nc *nats.Conn, cfg config.EventsConfig) error {
	if nc == nil {
		return fmt.Errorf("nats connection is nil")
	}

	js, err := nc.JetStream()
	if err != nil {
		return fmt.Errorf("jetstream init: %w", err)
	}

	// Check if stream already exists
	_, err = js.StreamInfo(cfg.StreamName)
	if err == nil {
		return nil // stream exists
	}

	// Create stream with subjects matching all ordering event types
	_, err = js.AddStream(&nats.StreamConfig{
		Name:     cfg.StreamName, // "ordering"
		Subjects: []string{"ordering.>", "cafe.>"},
		Replicas: 1,
	})
	if err != nil {
		return fmt.Errorf("create stream %s: %w", cfg.StreamName, err)
	}

	return nil
}
