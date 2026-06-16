package backup

import (
	"context"
	"os"
	"time"

	"github.com/google/uuid"

	entbackup "github.com/bengobox/ordering-backend/internal/ent/backup"
	entbackupsetting "github.com/bengobox/ordering-backend/internal/ent/backupsetting"
)

// Settings is the per-tenant auto-backup configuration exposed to the UI. Auto-backup is
// OPT-IN: it runs only when AutoEnabled is true (default false).
type Settings struct {
	AutoEnabled   bool `json:"auto_enabled"`
	ScheduleHour  int  `json:"schedule_hour"`
	RetentionDays int  `json:"retention_days"`
}

// DefaultSettings is the off-by-default configuration returned when a tenant has never
// activated auto-backup.
func DefaultSettings() Settings {
	return Settings{
		AutoEnabled:   false,
		ScheduleHour:  2,
		RetentionDays: DefaultRetentionDays,
	}
}

// GetSettings returns the tenant's auto-backup settings, or DefaultSettings() (off) when no
// row exists yet.
func (s *Service) GetSettings(ctx context.Context, tenantID uuid.UUID) (Settings, error) {
	row, err := s.orm.BackupSetting.Query().
		Where(entbackupsetting.TenantID(tenantID)).
		Only(ctx)
	if err != nil {
		// No row yet → auto-backup has never been activated; return defaults (off).
		return DefaultSettings(), nil
	}
	return Settings{
		AutoEnabled:   row.AutoEnabled,
		ScheduleHour:  row.ScheduleHour,
		RetentionDays: row.RetentionDays,
	}, nil
}

// UpsertSettings clamps and persists the tenant's auto-backup settings (update-if-exists
// else create), returning the stored values.
func (s *Service) UpsertSettings(ctx context.Context, tenantID uuid.UUID, in Settings) (Settings, error) {
	if in.ScheduleHour < 0 || in.ScheduleHour > 23 {
		in.ScheduleHour = 2
	}
	if in.RetentionDays <= 0 {
		in.RetentionDays = DefaultRetentionDays
	}

	existing, err := s.orm.BackupSetting.Query().
		Where(entbackupsetting.TenantID(tenantID)).
		Only(ctx)
	if err == nil {
		updated, uerr := existing.Update().
			SetAutoEnabled(in.AutoEnabled).
			SetScheduleHour(in.ScheduleHour).
			SetRetentionDays(in.RetentionDays).
			Save(ctx)
		if uerr != nil {
			return Settings{}, uerr
		}
		return Settings{
			AutoEnabled:   updated.AutoEnabled,
			ScheduleHour:  updated.ScheduleHour,
			RetentionDays: updated.RetentionDays,
		}, nil
	}

	created, cerr := s.orm.BackupSetting.Create().
		SetTenantID(tenantID).
		SetAutoEnabled(in.AutoEnabled).
		SetScheduleHour(in.ScheduleHour).
		SetRetentionDays(in.RetentionDays).
		Save(ctx)
	if cerr != nil {
		return Settings{}, cerr
	}
	return Settings{
		AutoEnabled:   created.AutoEnabled,
		ScheduleHour:  created.ScheduleHour,
		RetentionDays: created.RetentionDays,
	}, nil
}

// ActivatedTenant is one tenant that has opted in to auto-backup at a given hour.
type ActivatedTenant struct {
	TenantID      uuid.UUID
	RetentionDays int
}

// ListActivatedTenants returns the tenants that have OPTED IN to auto-backup (auto_enabled
// true) for the given service-local hour. This is what gates the scheduler — tenants without
// a row, or with auto_enabled=false, are never backed up automatically.
func (s *Service) ListActivatedTenants(ctx context.Context, hour int) ([]ActivatedTenant, error) {
	rows, err := s.orm.BackupSetting.Query().
		Where(
			entbackupsetting.AutoEnabled(true),
			entbackupsetting.ScheduleHour(hour),
		).
		All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]ActivatedTenant, 0, len(rows))
	for _, r := range rows {
		rd := r.RetentionDays
		if rd <= 0 {
			rd = DefaultRetentionDays
		}
		out = append(out, ActivatedTenant{TenantID: r.TenantID, RetentionDays: rd})
	}
	return out, nil
}

// ChurnTenant deletes the given tenant's backups older than retentionDays — both the artifact
// file and its tracking row. Scoped to one tenant (used by the per-tenant auto-backup path).
func (s *Service) ChurnTenant(ctx context.Context, tenantID uuid.UUID, retentionDays int) (int, error) {
	if retentionDays <= 0 {
		retentionDays = DefaultRetentionDays
	}
	cutoff := time.Now().Add(-time.Duration(retentionDays) * 24 * time.Hour)

	rows, err := s.orm.Backup.Query().
		Where(
			entbackup.TenantID(tenantID),
			entbackup.CreatedAtLT(cutoff),
		).
		All(ctx)
	if err != nil {
		return 0, err
	}
	removed := 0
	for _, r := range rows {
		if err := os.Remove(r.Path); err != nil && !os.IsNotExist(err) {
			continue
		}
		if err := s.orm.Backup.DeleteOne(r).Exec(ctx); err != nil {
			continue
		}
		removed++
	}
	return removed, nil
}
