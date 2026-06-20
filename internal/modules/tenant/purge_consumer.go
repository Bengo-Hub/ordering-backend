package tenant

import (
	"context"
	"errors"
	"fmt"
	"time"

	sharedevents "github.com/Bengo-Hub/shared-events"
	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"go.uber.org/zap"

	"github.com/bengobox/ordering-backend/internal/ent"
	entauditlog "github.com/bengobox/ordering-backend/internal/ent/auditlog"
	entbackup "github.com/bengobox/ordering-backend/internal/ent/backup"
	entbackupsetting "github.com/bengobox/ordering-backend/internal/ent/backupsetting"
	entcart "github.com/bengobox/ordering-backend/internal/ent/cart"
	entcartitem "github.com/bengobox/ordering-backend/internal/ent/cartitem"
	entcatalogoverride "github.com/bengobox/ordering-backend/internal/ent/catalogoverride"
	entcustomeraddress "github.com/bengobox/ordering-backend/internal/ent/customeraddress"
	entdatadeletionjob "github.com/bengobox/ordering-backend/internal/ent/datadeletionjob"
	entdataexportjob "github.com/bengobox/ordering-backend/internal/ent/dataexportjob"
	entdatasubjectrequest "github.com/bengobox/ordering-backend/internal/ent/datasubjectrequest"
	entdeliverywindow "github.com/bengobox/ordering-backend/internal/ent/deliverywindow"
	entdeliveryzone "github.com/bengobox/ordering-backend/internal/ent/deliveryzone"
	entgooglebusinessconnection "github.com/bengobox/ordering-backend/internal/ent/googlebusinessconnection"
	entgrouporder "github.com/bengobox/ordering-backend/internal/ent/grouporder"
	entgroupparticipant "github.com/bengobox/ordering-backend/internal/ent/groupparticipant"
	entloyaltyaccount "github.com/bengobox/ordering-backend/internal/ent/loyaltyaccount"
	entloyaltytransaction "github.com/bengobox/ordering-backend/internal/ent/loyaltytransaction"
	entorder "github.com/bengobox/ordering-backend/internal/ent/order"
	entorderassignment "github.com/bengobox/ordering-backend/internal/ent/orderassignment"
	entorderevent "github.com/bengobox/ordering-backend/internal/ent/orderevent"
	entorderitem "github.com/bengobox/ordering-backend/internal/ent/orderitem"
	entorderingrole "github.com/bengobox/ordering-backend/internal/ent/orderingrole"
	entoutboxevent "github.com/bengobox/ordering-backend/internal/ent/outboxevent"
	entpurgeoutlet "github.com/bengobox/ordering-backend/internal/ent/outlet"
	entoutletrating "github.com/bengobox/ordering-backend/internal/ent/outletrating"
	entpromocode "github.com/bengobox/ordering-backend/internal/ent/promocode"
	entpromoredemption "github.com/bengobox/ordering-backend/internal/ent/promoredemption"
	entserviceconfig "github.com/bengobox/ordering-backend/internal/ent/serviceconfig"
	entslametric "github.com/bengobox/ordering-backend/internal/ent/slametric"
	enttenant "github.com/bengobox/ordering-backend/internal/ent/tenant"
	enttenantsetting "github.com/bengobox/ordering-backend/internal/ent/tenantsetting"
	enttenantsyncevent "github.com/bengobox/ordering-backend/internal/ent/tenantsyncevent"
	entuser "github.com/bengobox/ordering-backend/internal/ent/user"
	entuserfavorite "github.com/bengobox/ordering-backend/internal/ent/userfavorite"
	entuserpreference "github.com/bengobox/ordering-backend/internal/ent/userpreference"
	entuserprofile "github.com/bengobox/ordering-backend/internal/ent/userprofile"
	entuserroleassignment "github.com/bengobox/ordering-backend/internal/ent/userroleassignment"
)

const (
	// purgeStreamName is the JetStream stream that retains "tenant.*" events. subscriptions-api
	// publishes tenant.purge on the "tenant" aggregate via its outbox; the subject is therefore
	// "tenant.purge" and lands on a stream whose subjects include "tenant.>".
	purgeStreamName     = "tenant"
	purgeStreamSubjects = "tenant.>"
	purgeStreamMaxAge   = 72 * time.Hour

	// purgeSubject is "{aggregate_type}.{event_type}" = "tenant.purge" (confirmed against
	// subscriptions-api internal/http/handlers/platform.go ConfirmDormancyPurge, which calls
	// WriteOutboxEventPublic(..., "tenant", tenantID, "purge", ...)).
	purgeSubject = "tenant.purge"

	// A PULL durable so BOTH ordering replicas share one work-queue consumer (restart safe),
	// mirroring TreasuryPaymentConsumer.
	purgeDurable    = "ordering-tenant-purge"
	purgeAckWait    = 60 * time.Second
	purgeMaxDeliver = 5 // bounded redelivery: a persistently failing DB shouldn't loop forever
	purgeFetchBatch = 5
	purgeFetchWait  = 5 * time.Second
)

// PurgeConsumer DELETES all of a tenant's ordering-backend data when subscriptions-api confirms a
// platform-owner-approved dormancy purge (tenant.purge). This is IRREVERSIBLE, so the handler is
// defensive: it acts only on a well-formed event whose payload is explicitly confirmed == true AND
// reason == "dormancy", and it scopes every delete strictly by tenant_id (or by a tenant-owned
// parent row). All deletes run in a single transaction and are idempotent (re-delivery is a no-op).
type PurgeConsumer struct {
	orm *ent.Client
	log *zap.Logger
}

// NewPurgeConsumer constructs the tenant.purge consumer.
func NewPurgeConsumer(orm *ent.Client, log *zap.Logger) *PurgeConsumer {
	return &PurgeConsumer{
		orm: orm,
		log: log.Named("tenant.purge_consumer"),
	}
}

// Start ensures the "tenant" stream exists then pull-subscribes to tenant.purge until ctx is done.
func (c *PurgeConsumer) Start(ctx context.Context, js nats.JetStreamContext) error {
	if js == nil {
		c.log.Warn("JetStream not available, skipping tenant.purge consumer")
		return nil
	}

	// Ensure the stream that retains tenant events exists (idempotent; safe if another service
	// already created it with overlapping subjects).
	if _, err := js.StreamInfo(purgeStreamName); err != nil {
		if _, aerr := js.AddStream(&nats.StreamConfig{
			Name:      purgeStreamName,
			Subjects:  []string{purgeStreamSubjects},
			Retention: nats.LimitsPolicy,
			MaxAge:    purgeStreamMaxAge,
			Storage:   nats.FileStorage,
		}); aerr != nil && aerr != nats.ErrStreamNameAlreadyInUse {
			return fmt.Errorf("tenant purge consumer: ensure stream: %w", aerr)
		}
	}

	sub, err := js.PullSubscribe(
		purgeSubject,
		purgeDurable,
		nats.AckExplicit(),
		nats.AckWait(purgeAckWait),
		nats.MaxDeliver(purgeMaxDeliver),
		nats.BindStream(purgeStreamName),
	)
	if err != nil {
		return fmt.Errorf("tenant purge consumer: pull subscribe: %w", err)
	}
	c.log.Info("tenant purge consumer started (pull)",
		zap.String("subject", purgeSubject), zap.String("durable", purgeDurable))

	for {
		if ctx.Err() != nil {
			_ = sub.Unsubscribe()
			return nil
		}
		msgs, ferr := sub.Fetch(purgeFetchBatch, nats.MaxWait(purgeFetchWait))
		if ferr != nil {
			if errors.Is(ferr, nats.ErrTimeout) {
				continue // no messages this window — keep polling
			}
			if ctx.Err() != nil {
				_ = sub.Unsubscribe()
				return nil
			}
			c.log.Warn("tenant purge: fetch error", zap.Error(ferr))
			continue
		}
		for _, m := range msgs {
			c.handleMessage(ctx, m)
		}
	}
}

// handleMessage validates the envelope, applies the safety guards, and on a confirmed dormancy
// purge deletes all of the tenant's data. Ack on success or unrecoverable input; Nak only on a
// transient DB error (bounded by purgeMaxDeliver).
func (c *PurgeConsumer) handleMessage(ctx context.Context, msg *nats.Msg) {
	evt, err := sharedevents.FromJSON(msg.Data)
	if err != nil {
		// Malformed envelope — redelivery cannot fix it. Ack to drop.
		c.log.Warn("tenant purge: unmarshal failed (acking, won't redeliver)", zap.Error(err))
		_ = msg.Ack()
		return
	}

	// GUARD 1: empty / invalid tenant_id → log + Ack, do nothing. Never run an unscoped delete.
	if evt.TenantID == uuid.Nil {
		c.log.Warn("tenant purge: empty/invalid tenant_id — skipping (acking)")
		_ = msg.Ack()
		return
	}

	// GUARD 2: only act on an explicitly confirmed dormancy purge. Anything else is acked & skipped
	// so a stray or future tenant.* event can never trigger irreversible deletion.
	confirmed, _ := evt.Payload["confirmed"].(bool)
	reason, _ := evt.Payload["reason"].(string)
	if evt.EventType != "purge" || !confirmed || reason != "dormancy" {
		c.log.Info("tenant purge: event not a confirmed dormancy purge — skipping (acking)",
			zap.String("tenant_id", evt.TenantID.String()),
			zap.String("event_type", evt.EventType),
			zap.Bool("confirmed", confirmed),
			zap.String("reason", reason))
		_ = msg.Ack()
		return
	}

	tenantID := evt.TenantID
	c.log.Warn("tenant purge: CONFIRMED dormancy purge received — DELETING all tenant data",
		zap.String("tenant_id", tenantID.String()))

	if derr := c.purgeTenant(ctx, tenantID); derr != nil {
		// Transient DB error — Nak for bounded redelivery so a temporary outage doesn't drop the
		// purge. Idempotent: a partial-then-retried purge simply deletes whatever remains.
		c.log.Error("tenant purge: delete failed (will retry, bounded)",
			zap.String("tenant_id", tenantID.String()), zap.Error(derr))
		_ = msg.Nak()
		return
	}

	c.log.Warn("tenant purge: COMPLETED — all tenant data deleted",
		zap.String("tenant_id", tenantID.String()))
	_ = msg.Ack()
}

// purgeTenant deletes every ordering-backend row owned by tenantID inside one transaction. Tables
// are deleted child-before-parent so foreign keys never block. Edge-FK children (no own tenant_id)
// are scoped through their tenant-owned parent. Logs a per-table count. Idempotent: zero rows is a
// success, so re-delivery / a retried partial purge is safe.
func (c *PurgeConsumer) purgeTenant(ctx context.Context, tenantID uuid.UUID) error {
	tx, err := c.orm.Tx(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	counts := map[string]int{}

	// del runs a tenant-scoped delete and records the count, aborting the whole purge on any error.
	type delFn func() (int, error)
	run := func(table string, fn delFn) error {
		n, derr := fn()
		if derr != nil {
			return fmt.Errorf("delete %s: %w", table, derr)
		}
		counts[table] = n
		return nil
	}

	// --- Resolve tenant-owned parent IDs for edge-FK children (no own tenant_id column). ---
	orderIDs, err := tx.Order.Query().Where(entorder.TenantID(tenantID)).IDs(ctx)
	if err != nil {
		return fmt.Errorf("collect order ids: %w", err)
	}
	cartIDs, err := tx.Cart.Query().Where(entcart.TenantID(tenantID)).IDs(ctx)
	if err != nil {
		return fmt.Errorf("collect cart ids: %w", err)
	}
	groupOrderIDs, err := tx.GroupOrder.Query().Where(entgrouporder.TenantID(tenantID)).IDs(ctx)
	if err != nil {
		return fmt.Errorf("collect group_order ids: %w", err)
	}
	promoCodeIDs, err := tx.PromoCode.Query().Where(entpromocode.TenantID(tenantID)).IDs(ctx)
	if err != nil {
		return fmt.Errorf("collect promo_code ids: %w", err)
	}
	loyaltyAccountIDs, err := tx.LoyaltyAccount.Query().Where(entloyaltyaccount.TenantID(tenantID)).IDs(ctx)
	if err != nil {
		return fmt.Errorf("collect loyalty_account ids: %w", err)
	}

	// --- 1) Leaf children scoped via a tenant-owned parent (only if the parent set is non-empty). ---
	if len(orderIDs) > 0 {
		if err := run("order_items", func() (int, error) {
			return tx.OrderItem.Delete().Where(entorderitem.OrderIDIn(orderIDs...)).Exec(ctx)
		}); err != nil {
			return err
		}
		if err := run("order_events", func() (int, error) {
			return tx.OrderEvent.Delete().Where(entorderevent.OrderIDIn(orderIDs...)).Exec(ctx)
		}); err != nil {
			return err
		}
	}
	if len(cartIDs) > 0 {
		if err := run("cart_items", func() (int, error) {
			return tx.CartItem.Delete().Where(entcartitem.CartIDIn(cartIDs...)).Exec(ctx)
		}); err != nil {
			return err
		}
	}
	if len(groupOrderIDs) > 0 {
		if err := run("group_participants", func() (int, error) {
			return tx.GroupParticipant.Delete().Where(entgroupparticipant.GroupOrderIDIn(groupOrderIDs...)).Exec(ctx)
		}); err != nil {
			return err
		}
	}
	if len(promoCodeIDs) > 0 {
		if err := run("promo_redemptions", func() (int, error) {
			return tx.PromoRedemption.Delete().Where(entpromoredemption.PromoCodeIDIn(promoCodeIDs...)).Exec(ctx)
		}); err != nil {
			return err
		}
	}
	if len(loyaltyAccountIDs) > 0 {
		if err := run("loyalty_transactions", func() (int, error) {
			return tx.LoyaltyTransaction.Delete().Where(entloyaltytransaction.AccountIDIn(loyaltyAccountIDs...)).Exec(ctx)
		}); err != nil {
			return err
		}
	}

	// --- 2) Children with their own tenant_id (delete before their parents). ---
	if err := run("delivery_windows", func() (int, error) {
		return tx.DeliveryWindow.Delete().Where(entdeliverywindow.TenantID(tenantID)).Exec(ctx)
	}); err != nil {
		return err
	}
	if err := run("order_assignments", func() (int, error) {
		return tx.OrderAssignment.Delete().Where(entorderassignment.TenantID(tenantID)).Exec(ctx)
	}); err != nil {
		return err
	}

	// --- 3) User-owned edge-FK children (profile/preferences), scoped to the tenant's users. ---
	if err := run("user_profiles", func() (int, error) {
		return tx.UserProfile.Delete().Where(entuserprofile.HasUserWith(entuser.TenantID(tenantID))).Exec(ctx)
	}); err != nil {
		return err
	}
	if err := run("user_preferences", func() (int, error) {
		return tx.UserPreference.Delete().Where(entuserpreference.HasUserWith(entuser.TenantID(tenantID))).Exec(ctx)
	}); err != nil {
		return err
	}

	// --- 4) TenantSetting (edge-FK to the tenant projection row). ---
	if err := run("tenant_settings", func() (int, error) {
		return tx.TenantSetting.Delete().Where(enttenantsetting.HasTenantWith(enttenant.ID(tenantID))).Exec(ctx)
	}); err != nil {
		return err
	}

	// --- 5) Parent / top-level tables, each scoped strictly by tenant_id. ---
	parentDeletes := []struct {
		table string
		fn    delFn
	}{
		{"orders", func() (int, error) { return tx.Order.Delete().Where(entorder.TenantID(tenantID)).Exec(ctx) }},
		{"carts", func() (int, error) { return tx.Cart.Delete().Where(entcart.TenantID(tenantID)).Exec(ctx) }},
		{"group_orders", func() (int, error) {
			return tx.GroupOrder.Delete().Where(entgrouporder.TenantID(tenantID)).Exec(ctx)
		}},
		{"promo_codes", func() (int, error) {
			return tx.PromoCode.Delete().Where(entpromocode.TenantID(tenantID)).Exec(ctx)
		}},
		{"loyalty_accounts", func() (int, error) {
			return tx.LoyaltyAccount.Delete().Where(entloyaltyaccount.TenantID(tenantID)).Exec(ctx)
		}},
		{"catalog_overrides", func() (int, error) {
			return tx.CatalogOverride.Delete().Where(entcatalogoverride.TenantID(tenantID)).Exec(ctx)
		}},
		{"outlet_ratings", func() (int, error) {
			return tx.OutletRating.Delete().Where(entoutletrating.TenantID(tenantID)).Exec(ctx)
		}},
		{"delivery_zones", func() (int, error) {
			return tx.DeliveryZone.Delete().Where(entdeliveryzone.TenantID(tenantID)).Exec(ctx)
		}},
		{"customer_addresses", func() (int, error) {
			return tx.CustomerAddress.Delete().Where(entcustomeraddress.TenantID(tenantID)).Exec(ctx)
		}},
		{"user_favorites", func() (int, error) {
			return tx.UserFavorite.Delete().Where(entuserfavorite.TenantID(tenantID)).Exec(ctx)
		}},
		{"user_role_assignments", func() (int, error) {
			return tx.UserRoleAssignment.Delete().Where(entuserroleassignment.TenantID(tenantID)).Exec(ctx)
		}},
		{"ordering_roles", func() (int, error) {
			return tx.OrderingRole.Delete().Where(entorderingrole.TenantID(tenantID)).Exec(ctx)
		}},
		{"service_configs", func() (int, error) {
			return tx.ServiceConfig.Delete().Where(entserviceconfig.TenantID(tenantID)).Exec(ctx)
		}},
		{"sla_metrics", func() (int, error) {
			return tx.SLAMetric.Delete().Where(entslametric.TenantID(tenantID)).Exec(ctx)
		}},
		{"audit_logs", func() (int, error) {
			return tx.AuditLog.Delete().Where(entauditlog.TenantID(tenantID)).Exec(ctx)
		}},
		{"google_business_connections", func() (int, error) {
			return tx.GoogleBusinessConnection.Delete().Where(entgooglebusinessconnection.TenantID(tenantID)).Exec(ctx)
		}},
		{"backups", func() (int, error) { return tx.Backup.Delete().Where(entbackup.TenantID(tenantID)).Exec(ctx) }},
		{"backup_settings", func() (int, error) {
			return tx.BackupSetting.Delete().Where(entbackupsetting.TenantID(tenantID)).Exec(ctx)
		}},
		{"data_deletion_jobs", func() (int, error) {
			return tx.DataDeletionJob.Delete().Where(entdatadeletionjob.TenantID(tenantID)).Exec(ctx)
		}},
		{"data_export_jobs", func() (int, error) {
			return tx.DataExportJob.Delete().Where(entdataexportjob.TenantID(tenantID)).Exec(ctx)
		}},
		{"data_subject_requests", func() (int, error) {
			return tx.DataSubjectRequest.Delete().Where(entdatasubjectrequest.TenantID(tenantID)).Exec(ctx)
		}},
		{"tenant_sync_events", func() (int, error) {
			return tx.TenantSyncEvent.Delete().Where(enttenantsyncevent.TenantID(tenantID)).Exec(ctx)
		}},
		{"outlets", func() (int, error) {
			return tx.Outlet.Delete().Where(entpurgeoutlet.TenantID(tenantID)).Exec(ctx)
		}},
		{"users", func() (int, error) { return tx.User.Delete().Where(entuser.TenantID(tenantID)).Exec(ctx) }},
		// Outbox last among data tables (it may hold not-yet-published events for this tenant; the
		// purge is already confirmed upstream so those are moot).
		{"outbox_events", func() (int, error) {
			return tx.OutboxEvent.Delete().Where(entoutboxevent.TenantID(tenantID)).Exec(ctx)
		}},
	}
	for _, d := range parentDeletes {
		if err := run(d.table, d.fn); err != nil {
			return err
		}
	}

	// --- 6) Finally the tenant projection row itself (its id IS the tenant UUID). ---
	if err := run("tenants", func() (int, error) {
		return tx.Tenant.Delete().Where(enttenant.ID(tenantID)).Exec(ctx)
	}); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit purge tx: %w", err)
	}
	committed = true

	total := 0
	fields := make([]zap.Field, 0, len(counts)+2)
	fields = append(fields, zap.String("tenant_id", tenantID.String()))
	for table, n := range counts {
		total += n
		fields = append(fields, zap.Int(table, n))
	}
	fields = append(fields, zap.Int("total_rows_deleted", total))
	c.log.Warn("tenant purge: per-table delete counts", fields...)
	return nil
}
