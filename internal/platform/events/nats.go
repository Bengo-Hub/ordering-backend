package events

import (
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
