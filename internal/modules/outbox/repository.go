// Package outbox provides the ordering-backend-specific outbox publisher.
// The repository is provided by the shared-events library
// (eventslib.NewSQLOutboxRepository). Only the Publisher polling loop is
// service-specific because ordering uses plain NATS (not JetStream) for
// outbox publishing via the OutboxPublisher adapter.
package outbox
