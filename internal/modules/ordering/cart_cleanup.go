package ordering

import (
	"context"
	"time"

	"go.uber.org/zap"
)

// CartCleanupJob periodically expires stale active carts across all tenants.
type CartCleanupJob struct {
	repo   Repository
	logger *zap.Logger
}

// NewCartCleanupJob creates a new cart cleanup job.
func NewCartCleanupJob(repo Repository, logger *zap.Logger) *CartCleanupJob {
	return &CartCleanupJob{
		repo:   repo,
		logger: logger.Named("cart-cleanup"),
	}
}

// Start runs the cleanup loop. It blocks until the context is cancelled.
func (j *CartCleanupJob) Start(ctx context.Context) {
	ticker := time.NewTicker(15 * time.Minute)
	defer ticker.Stop()

	j.logger.Info("cart cleanup job started", zap.Duration("interval", 15*time.Minute))

	// Run once immediately on start.
	j.run(ctx)

	for {
		select {
		case <-ctx.Done():
			j.logger.Info("cart cleanup job stopped")
			return
		case <-ticker.C:
			j.run(ctx)
		}
	}
}

// run performs a single cleanup pass across all tenants.
func (j *CartCleanupJob) run(ctx context.Context) {
	tenantIDs, err := j.repo.ListDistinctCartTenantIDs(ctx)
	if err != nil {
		j.logger.Error("failed to list distinct cart tenant IDs", zap.Error(err))
		return
	}

	if len(tenantIDs) == 0 {
		return
	}

	var totalExpired int
	for _, tid := range tenantIDs {
		count, err := j.repo.ExpireOldCarts(ctx, tid)
		if err != nil {
			j.logger.Error("failed to expire old carts for tenant",
				zap.String("tenant_id", tid.String()),
				zap.Error(err))
			continue
		}
		totalExpired += count
	}

	if totalExpired > 0 {
		j.logger.Info("cart cleanup pass completed",
			zap.Int("tenants_checked", len(tenantIDs)),
			zap.Int("carts_expired", totalExpired))
	}
}
