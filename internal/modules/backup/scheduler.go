package backup

import (
	"context"
	"time"

	"go.uber.org/zap"
)

// schedulerAdvisoryLockKey is a fixed, service-unique key for the Postgres session advisory
// lock that guards the daily run so only ONE replica executes it. ("ordering-backup")
const schedulerAdvisoryLockKey int64 = 0x4F52_4442 // 'O','R','D','B'

// SchedulerConfig configures the auto-backup + retention churn loop.
type SchedulerConfig struct {
	Enabled       bool // BACKUP_SCHEDULE_ENABLED (default true)
	Hour          int  // BACKUP_SCHEDULE_HOUR (legacy default 2) — kept for config compat
	RetentionDays int  // BACKUP_RETENTION_DAYS (default 4)
}

// Scheduler wakes up at the top of every hour and, holding a Postgres advisory lock so only
// one replica runs, backs up ONLY the tenants that OPTED IN (BackupSetting.auto_enabled=true)
// at that hour, then runs a retention churn. Auto-backup is per-tenant opt-in — tenants that
// never activated it are never touched.
type Scheduler struct {
	svc *Service
	cfg SchedulerConfig
	log *zap.Logger
}

// NewScheduler builds the scheduler. Defaults are applied for zero-value config.
func NewScheduler(svc *Service, cfg SchedulerConfig, log *zap.Logger) *Scheduler {
	if cfg.Hour < 0 || cfg.Hour > 23 {
		cfg.Hour = 2
	}
	if cfg.RetentionDays <= 0 {
		cfg.RetentionDays = DefaultRetentionDays
	}
	return &Scheduler{svc: svc, cfg: cfg, log: log.Named("backup.Scheduler")}
}

// Start launches the scheduler goroutine: a churn-only pass on startup, then at the top of
// every hour it backs up the tenants that opted in for that hour, followed by a churn. Stops
// when ctx is cancelled.
func (sc *Scheduler) Start(ctx context.Context) {
	if !sc.cfg.Enabled {
		sc.log.Info("backup scheduler disabled (BACKUP_SCHEDULE_ENABLED=false)")
		return
	}
	sc.log.Info("backup scheduler started (per-tenant opt-in)",
		zap.Int("retention_days", sc.cfg.RetentionDays))

	go func() {
		// Startup pass: churn only (backupHour < 0 means "no backup this pass").
		sc.runGuarded(ctx, -1)
		for {
			next := nextTopOfHour(time.Now())
			timer := time.NewTimer(time.Until(next))
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
				sc.runGuarded(ctx, time.Now().Hour())
			}
		}
	}()
}

// runGuarded acquires the advisory lock and, if won, backs up the tenants that opted in for
// backupHour (when backupHour >= 0), then runs the safety churn. Only one replica wins.
func (sc *Scheduler) runGuarded(ctx context.Context, backupHour int) {
	conn, err := sc.svc.db.Conn(ctx)
	if err != nil {
		sc.log.Warn("scheduler: acquire conn failed", zap.Error(err))
		return
	}
	defer func() { _ = conn.Close() }()

	var got bool
	if err := conn.QueryRowContext(ctx, `SELECT pg_try_advisory_lock($1)`, schedulerAdvisoryLockKey).Scan(&got); err != nil {
		sc.log.Warn("scheduler: advisory lock failed", zap.Error(err))
		return
	}
	if !got {
		return
	}
	defer func() {
		_, _ = conn.ExecContext(context.Background(), `SELECT pg_advisory_unlock($1)`, schedulerAdvisoryLockKey)
	}()

	if backupHour >= 0 {
		sc.backupActivatedTenants(ctx, backupHour)
	}
	// Safety churn across all tenants at the configured retention window.
	if _, err := sc.svc.Churn(ctx, sc.cfg.RetentionDays); err != nil {
		sc.log.Warn("scheduler: churn failed", zap.Error(err))
	}
}

// backupActivatedTenants backs up ONLY the tenants that opted in for the given hour, then
// churns each to its own retention window.
func (sc *Scheduler) backupActivatedTenants(ctx context.Context, hour int) {
	tenants, err := sc.svc.ListActivatedTenants(ctx, hour)
	if err != nil {
		sc.log.Warn("scheduler: list activated tenants failed", zap.Error(err))
		return
	}
	if len(tenants) == 0 {
		return
	}
	ok := 0
	for _, t := range tenants {
		if _, err := sc.svc.Generate(ctx, t.TenantID); err != nil {
			sc.log.Warn("scheduler: tenant backup failed", zap.String("tenant", t.TenantID.String()), zap.Error(err))
			continue
		}
		if _, err := sc.svc.ChurnTenant(ctx, t.TenantID, t.RetentionDays); err != nil {
			sc.log.Warn("scheduler: tenant churn failed", zap.String("tenant", t.TenantID.String()), zap.Error(err))
		}
		ok++
	}
	sc.log.Info("scheduled auto-backup complete", zap.Int("hour", hour), zap.Int("activated", len(tenants)), zap.Int("succeeded", ok))
}

// nextTopOfHour returns the next top-of-hour strictly after now.
func nextTopOfHour(now time.Time) time.Time {
	return now.Truncate(time.Hour).Add(time.Hour)
}
