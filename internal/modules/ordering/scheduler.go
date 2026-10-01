package ordering

import (
	"context"
	sharedcache "github.com/Bengo-Hub/cache"
	"time"

	"go.uber.org/zap"
)

// OrderScheduler periodically checks for scheduled orders whose prep window has opened and hands
// them to the outlet (see processScheduledOrders).
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

// processScheduledOrders hands confirmed scheduled orders to the outlet (POS/KDS/kitchen chit or
// appointment) once their prep window opens. The kitchen then works the order like any other and
// its own Start/Ready actions drive the customer-facing status, so nothing is forced to
// "preparing" here. Orders already handed over are skipped; pos-api is idempotent on the online
// order id if two replicas race.
func (s *OrderScheduler) processScheduledOrders(ctx context.Context) {
	// Runs on every replica's ticker; only the first replica in each period does the work.
	if !sharedcache.ClaimPeriod(ctx, "ordering:scheduled-handoff", time.Minute) {
		return
	}
	orders, err := s.service.repo.ListScheduledOrdersDue(ctx, ScheduledPrepTimeBuffer)
	if err != nil {
		s.logger.Error("failed to list scheduled orders due", zap.Error(err))
		return
	}

	for _, o := range orders {
		order := o // capture loop variable
		if alreadyHandedOff(&order) {
			continue
		}
		s.service.handOffToOutlet(ctx, &order)
		s.logger.Info("scheduled order handed to the outlet",
			zap.String("order_id", order.ID.String()),
			zap.String("order_number", order.OrderNumber))
	}
}
