package audit

import (
	"context"
	"time"

	"github.com/bengobox/ordering-backend/internal/ent"
	"github.com/bengobox/ordering-backend/internal/ent/auditlog"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// Entry represents a structured audit event.
type Entry struct {
	TenantID    *uuid.UUID
	UserID      *uuid.UUID
	Action      string
	Resource    string
	ResourceID  string
	HTTPMethod  string
	Path        string
	StatusCode  int
	IPAddress   string
	UserAgent   string
	RequestBody map[string]any
	Context     map[string]any
	DurationMS  int64
	OccurredAt  time.Time
}

// Logger writes audit entries into the database.
type Logger struct {
	client *ent.Client
	logger *zap.Logger
}

// New constructs a Logger.
func New(client *ent.Client, logger *zap.Logger) *Logger {
	return &Logger{
		client: client,
		logger: logger.Named("audit"),
	}
}

// Record persists an audit entry, logging failures but not interrupting flows.
func (l *Logger) Record(ctx context.Context, entry Entry) {
	if entry.Action == "" {
		return
	}

	builder := l.client.AuditLog.Create().
		SetAction(entry.Action).
		SetOccurredAt(timeOrDefault(entry.OccurredAt))

	if entry.Resource != "" {
		builder.SetResourceType(entry.Resource)
	}
	if entry.ResourceID != "" {
		builder.SetResourceID(entry.ResourceID)
	}
	if entry.HTTPMethod != "" {
		builder.SetHTTPMethod(entry.HTTPMethod)
	}
	if entry.Path != "" {
		builder.SetPath(entry.Path)
	}
	if entry.StatusCode > 0 {
		builder.SetStatusCode(entry.StatusCode)
	}
	if entry.IPAddress != "" {
		builder.SetIPAddress(entry.IPAddress)
	}
	if entry.UserAgent != "" {
		builder.SetUserAgent(truncate(entry.UserAgent, 512))
	}
	if entry.RequestBody != nil {
		builder.SetRequestBody(entry.RequestBody)
	}
	if entry.Context != nil {
		builder.SetContext(entry.Context)
	}
	if entry.DurationMS > 0 {
		builder.SetDurationMs(entry.DurationMS)
	}
	if entry.TenantID != nil {
		builder.SetTenantID(*entry.TenantID)
	}
	if entry.UserID != nil {
		builder.SetUserID(*entry.UserID)
	}

	if err := builder.Exec(ctx); err != nil {
		l.logger.Warn("failed to persist audit log",
			zap.Error(err),
			zap.String("action", entry.Action),
			zap.String("resource", entry.Resource),
		)
	}
}

// RecordAsync persists an audit entry asynchronously to avoid blocking the request.
func (l *Logger) RecordAsync(entry Entry) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		l.Record(ctx, entry)
	}()
}

// ListRecent retrieves most recent entries for debugging/ops.
func (l *Logger) ListRecent(ctx context.Context, tenantID *uuid.UUID, limit int) ([]*ent.AuditLog, error) {
	if limit <= 0 {
		limit = 50
	}

	query := l.client.AuditLog.Query().
		Order(ent.Desc(auditlog.FieldOccurredAt)).
		Limit(limit)

	if tenantID != nil {
		query = query.Where(auditlog.TenantIDEQ(*tenantID))
	}

	return query.All(ctx)
}

// ListByResource retrieves audit logs for a specific resource.
func (l *Logger) ListByResource(ctx context.Context, resourceType, resourceID string, limit int) ([]*ent.AuditLog, error) {
	if limit <= 0 {
		limit = 50
	}

	return l.client.AuditLog.Query().
		Where(
			auditlog.ResourceTypeEQ(resourceType),
			auditlog.ResourceIDEQ(resourceID),
		).
		Order(ent.Desc(auditlog.FieldOccurredAt)).
		Limit(limit).
		All(ctx)
}

// ListByUser retrieves audit logs for a specific user.
func (l *Logger) ListByUser(ctx context.Context, userID uuid.UUID, limit int) ([]*ent.AuditLog, error) {
	if limit <= 0 {
		limit = 50
	}

	return l.client.AuditLog.Query().
		Where(auditlog.UserIDEQ(userID)).
		Order(ent.Desc(auditlog.FieldOccurredAt)).
		Limit(limit).
		All(ctx)
}

func timeOrDefault(t time.Time) time.Time {
	if t.IsZero() {
		return time.Now().UTC()
	}
	return t
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen]
}
