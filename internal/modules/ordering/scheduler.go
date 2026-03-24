package ordering

import (
	"context"
	"time"

	"go.uber.org/zap"
)

// OrderScheduler periodically checks for scheduled orders that are due to begin
// preparation and transitions them to the "preparing" status.
type OrderScheduler struct {
	logger  *zap.Logger
	service *OrderService
	ticker  *time.Ticker
}

// NewOrderScheduler creates a new order scheduler.
func NewOrderScheduler(logger *zap.Logger, service *OrderService) *OrderScheduler {
	return &OrderScheduler{
		logger:  logger.Named("order-scheduler"),
		service: service,
		ticker:  time.NewTicker(1 * time.Minute),
	}
}

// Start runs the scheduler loop. It blocks until the context is cancelled.
func (s *OrderScheduler) Start(ctx context.Context) {
	s.logger.Info("order scheduler started")
	for {
		select {
		case <-ctx.Done():
			s.ticker.Stop()
			s.logger.Info("order scheduler stopped")
			return
		case <-s.ticker.C:
			s.processScheduledOrders(ctx)
		}
	}
}

// processScheduledOrders queries confirmed scheduled orders whose scheduled_for
// time falls within the preparation buffer window, transitions each to "preparing",
// and publishes the order.ready event.
func (s *OrderScheduler) processScheduledOrders(ctx context.Context) {
	orders, err := s.service.repo.ListScheduledOrdersDue(ctx, ScheduledPrepTimeBuffer)
	if err != nil {
		s.logger.Error("failed to list scheduled orders due", zap.Error(err))
		return
	}

	if len(orders) == 0 {
		return
	}

	s.logger.Info("processing scheduled orders", zap.Int("count", len(orders)))

	for _, o := range orders {
		order := o // capture loop variable
		_, err := s.service.UpdateOrderStatus(
			ctx,
			order.TenantID,
			order.ID,
			OrderStatusPreparing,
			nil,
			"system",
			"",
		)
		if err != nil {
			s.logger.Error("failed to transition scheduled order to preparing",
				zap.String("order_id", order.ID.String()),
				zap.Error(err))
			continue
		}

		s.logger.Info("scheduled order moved to preparing",
			zap.String("order_id", order.ID.String()),
			zap.String("order_number", order.OrderNumber))
	}
}
