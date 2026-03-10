-- Create "audit_logs" table
CREATE TABLE "audit_logs" ("id" uuid NOT NULL, "tenant_id" uuid NULL, "user_id" uuid NULL, "action" character varying NOT NULL, "resource_type" character varying NULL, "resource_id" character varying NULL, "http_method" character varying NULL, "path" character varying NULL, "status_code" bigint NULL, "ip_address" character varying NULL, "user_agent" character varying NULL, "request_body" jsonb NULL, "context" jsonb NULL, "duration_ms" bigint NULL, "occurred_at" timestamptz NOT NULL, PRIMARY KEY ("id"));
-- Create index "auditlog_action_occurred_at" to table: "audit_logs"
CREATE INDEX "auditlog_action_occurred_at" ON "audit_logs" ("action", "occurred_at");
-- Create index "auditlog_resource_type_resource_id" to table: "audit_logs"
CREATE INDEX "auditlog_resource_type_resource_id" ON "audit_logs" ("resource_type", "resource_id");
-- Create index "auditlog_tenant_id_occurred_at" to table: "audit_logs"
CREATE INDEX "auditlog_tenant_id_occurred_at" ON "audit_logs" ("tenant_id", "occurred_at");
-- Create index "auditlog_user_id_occurred_at" to table: "audit_logs"
CREATE INDEX "auditlog_user_id_occurred_at" ON "audit_logs" ("user_id", "occurred_at");
-- Create "data_export_jobs" table
CREATE TABLE "data_export_jobs" ("id" uuid NOT NULL, "tenant_id" uuid NOT NULL, "user_id" uuid NOT NULL, "format" character varying NOT NULL DEFAULT 'json', "status" character varying NOT NULL DEFAULT 'pending', "included_data" jsonb NULL, "storage_url" text NULL, "error_message" text NULL, "file_size_bytes" bigint NULL, "records_exported" bigint NULL, "requested_at" timestamptz NOT NULL, "started_at" timestamptz NULL, "completed_at" timestamptz NULL, "expires_at" timestamptz NULL, "created_at" timestamptz NOT NULL, "updated_at" timestamptz NOT NULL, PRIMARY KEY ("id"));
-- Create index "dataexportjob_expires_at" to table: "data_export_jobs"
CREATE INDEX "dataexportjob_expires_at" ON "data_export_jobs" ("expires_at");
-- Create index "dataexportjob_requested_at" to table: "data_export_jobs"
CREATE INDEX "dataexportjob_requested_at" ON "data_export_jobs" ("requested_at");
-- Create index "dataexportjob_status" to table: "data_export_jobs"
CREATE INDEX "dataexportjob_status" ON "data_export_jobs" ("status");
-- Create index "dataexportjob_tenant_id_status" to table: "data_export_jobs"
CREATE INDEX "dataexportjob_tenant_id_status" ON "data_export_jobs" ("tenant_id", "status");
-- Create index "dataexportjob_tenant_id_user_id" to table: "data_export_jobs"
CREATE INDEX "dataexportjob_tenant_id_user_id" ON "data_export_jobs" ("tenant_id", "user_id");
-- Create index "dataexportjob_user_id" to table: "data_export_jobs"
CREATE INDEX "dataexportjob_user_id" ON "data_export_jobs" ("user_id");
-- Create "notification_templates" table
CREATE TABLE "notification_templates" ("id" uuid NOT NULL, "tenant_id" uuid NOT NULL, "channel" character varying NOT NULL, "event_key" character varying NOT NULL, "locale" character varying NOT NULL DEFAULT 'en', "subject" character varying NULL, "body" text NOT NULL, "data_schema" jsonb NULL, "is_active" boolean NOT NULL DEFAULT true, "created_at" timestamptz NOT NULL, "updated_at" timestamptz NOT NULL, PRIMARY KEY ("id"));
-- Create index "notificationtemplate_is_active" to table: "notification_templates"
CREATE INDEX "notificationtemplate_is_active" ON "notification_templates" ("is_active");
-- Create index "notificationtemplate_tenant_id_channel_event_key_locale" to table: "notification_templates"
CREATE UNIQUE INDEX "notificationtemplate_tenant_id_channel_event_key_locale" ON "notification_templates" ("tenant_id", "channel", "event_key", "locale");
-- Create index "notificationtemplate_tenant_id_event_key" to table: "notification_templates"
CREATE INDEX "notificationtemplate_tenant_id_event_key" ON "notification_templates" ("tenant_id", "event_key");
-- Create "notification_subscriptions" table
CREATE TABLE "notification_subscriptions" ("id" uuid NOT NULL, "tenant_id" uuid NOT NULL, "user_id" uuid NOT NULL, "channel" character varying NOT NULL, "event_key" character varying NOT NULL, "is_subscribed" boolean NOT NULL DEFAULT true, "updated_at" timestamptz NOT NULL, PRIMARY KEY ("id"));
-- Create index "notificationsubscription_is_subscribed" to table: "notification_subscriptions"
CREATE INDEX "notificationsubscription_is_subscribed" ON "notification_subscriptions" ("is_subscribed");
-- Create index "notificationsubscription_tenant_id_user_id" to table: "notification_subscriptions"
CREATE INDEX "notificationsubscription_tenant_id_user_id" ON "notification_subscriptions" ("tenant_id", "user_id");
-- Create index "notificationsubscription_tenant_id_user_id_channel_event_key" to table: "notification_subscriptions"
CREATE UNIQUE INDEX "notificationsubscription_tenant_id_user_id_channel_event_key" ON "notification_subscriptions" ("tenant_id", "user_id", "channel", "event_key");
-- Create "notification_events" table
CREATE TABLE "notification_events" ("id" uuid NOT NULL, "tenant_id" uuid NOT NULL, "user_id" uuid NULL, "event_key" character varying NOT NULL, "payload" jsonb NOT NULL, "order_id" uuid NULL, "status" character varying NOT NULL DEFAULT 'pending', "attempts" bigint NOT NULL DEFAULT 0, "last_attempt_at" timestamptz NULL, "error_message" text NULL, "error_code" character varying NULL, "external_id" character varying NULL, "sent_at" timestamptz NULL, "delivered_at" timestamptz NULL, "created_at" timestamptz NOT NULL, "updated_at" timestamptz NOT NULL, PRIMARY KEY ("id"));
-- Create index "notificationevent_created_at" to table: "notification_events"
CREATE INDEX "notificationevent_created_at" ON "notification_events" ("created_at");
-- Create index "notificationevent_external_id" to table: "notification_events"
CREATE INDEX "notificationevent_external_id" ON "notification_events" ("external_id");
-- Create index "notificationevent_order_id" to table: "notification_events"
CREATE INDEX "notificationevent_order_id" ON "notification_events" ("order_id");
-- Create index "notificationevent_sent_at" to table: "notification_events"
CREATE INDEX "notificationevent_sent_at" ON "notification_events" ("sent_at");
-- Create index "notificationevent_status_attempts_last_attempt_at" to table: "notification_events"
CREATE INDEX "notificationevent_status_attempts_last_attempt_at" ON "notification_events" ("status", "attempts", "last_attempt_at");
-- Create index "notificationevent_status_created_at" to table: "notification_events"
CREATE INDEX "notificationevent_status_created_at" ON "notification_events" ("status", "created_at");
-- Create index "notificationevent_tenant_id_event_key" to table: "notification_events"
CREATE INDEX "notificationevent_tenant_id_event_key" ON "notification_events" ("tenant_id", "event_key");
-- Create index "notificationevent_tenant_id_status" to table: "notification_events"
CREATE INDEX "notificationevent_tenant_id_status" ON "notification_events" ("tenant_id", "status");
-- Create index "notificationevent_tenant_id_user_id" to table: "notification_events"
CREATE INDEX "notificationevent_tenant_id_user_id" ON "notification_events" ("tenant_id", "user_id");
-- Create "data_deletion_jobs" table
CREATE TABLE "data_deletion_jobs" ("id" uuid NOT NULL, "tenant_id" uuid NOT NULL, "user_id" uuid NOT NULL, "deletion_type" character varying NOT NULL DEFAULT 'soft', "status" character varying NOT NULL DEFAULT 'pending', "reason" text NULL, "confirmed" boolean NOT NULL DEFAULT false, "retention_days" bigint NOT NULL DEFAULT 30, "error_message" text NULL, "deletion_summary" jsonb NULL, "requested_at" timestamptz NOT NULL, "scheduled_for" timestamptz NULL, "started_at" timestamptz NULL, "completed_at" timestamptz NULL, "created_at" timestamptz NOT NULL, "updated_at" timestamptz NOT NULL, PRIMARY KEY ("id"));
-- Create index "datadeletionjob_requested_at" to table: "data_deletion_jobs"
CREATE INDEX "datadeletionjob_requested_at" ON "data_deletion_jobs" ("requested_at");
-- Create index "datadeletionjob_scheduled_for" to table: "data_deletion_jobs"
CREATE INDEX "datadeletionjob_scheduled_for" ON "data_deletion_jobs" ("scheduled_for");
-- Create index "datadeletionjob_status" to table: "data_deletion_jobs"
CREATE INDEX "datadeletionjob_status" ON "data_deletion_jobs" ("status");
-- Create index "datadeletionjob_tenant_id_status" to table: "data_deletion_jobs"
CREATE INDEX "datadeletionjob_tenant_id_status" ON "data_deletion_jobs" ("tenant_id", "status");
-- Create index "datadeletionjob_tenant_id_user_id" to table: "data_deletion_jobs"
CREATE INDEX "datadeletionjob_tenant_id_user_id" ON "data_deletion_jobs" ("tenant_id", "user_id");
-- Create index "datadeletionjob_user_id" to table: "data_deletion_jobs"
CREATE INDEX "datadeletionjob_user_id" ON "data_deletion_jobs" ("user_id");
-- Create "logistics_events" table
CREATE TABLE "logistics_events" ("id" uuid NOT NULL, "tenant_id" uuid NULL, "external_id" character varying NOT NULL, "event_type" character varying NOT NULL, "order_id" uuid NULL, "assignment_id" uuid NULL, "logistics_task_id" character varying NULL, "rider_id" character varying NULL, "payload" jsonb NOT NULL, "headers" jsonb NULL, "signature" character varying NULL, "signature_valid" boolean NULL, "status" character varying NOT NULL DEFAULT 'pending', "retry_count" bigint NOT NULL DEFAULT 0, "last_retry_at" timestamptz NULL, "error_message" text NULL, "error_code" character varying NULL, "ip_address" character varying NULL, "received_at" timestamptz NOT NULL, "processed_at" timestamptz NULL, "created_at" timestamptz NOT NULL, "updated_at" timestamptz NOT NULL, PRIMARY KEY ("id"));
-- Create index "logistics_events_external_id_key" to table: "logistics_events"
CREATE UNIQUE INDEX "logistics_events_external_id_key" ON "logistics_events" ("external_id");
-- Create index "logisticsevent_assignment_id" to table: "logistics_events"
CREATE INDEX "logisticsevent_assignment_id" ON "logistics_events" ("assignment_id");
-- Create index "logisticsevent_created_at" to table: "logistics_events"
CREATE INDEX "logisticsevent_created_at" ON "logistics_events" ("created_at");
-- Create index "logisticsevent_logistics_task_id" to table: "logistics_events"
CREATE INDEX "logisticsevent_logistics_task_id" ON "logistics_events" ("logistics_task_id");
-- Create index "logisticsevent_order_id" to table: "logistics_events"
CREATE INDEX "logisticsevent_order_id" ON "logistics_events" ("order_id");
-- Create index "logisticsevent_received_at" to table: "logistics_events"
CREATE INDEX "logisticsevent_received_at" ON "logistics_events" ("received_at");
-- Create index "logisticsevent_status" to table: "logistics_events"
CREATE INDEX "logisticsevent_status" ON "logistics_events" ("status");
-- Create index "logisticsevent_tenant_id_event_type" to table: "logistics_events"
CREATE INDEX "logisticsevent_tenant_id_event_type" ON "logistics_events" ("tenant_id", "event_type");
-- Create "data_subject_requests" table
CREATE TABLE "data_subject_requests" ("id" uuid NOT NULL, "tenant_id" uuid NOT NULL, "user_id" uuid NOT NULL, "request_type" character varying NOT NULL, "status" character varying NOT NULL DEFAULT 'pending', "description" text NULL, "notes" text NULL, "result_url" text NULL, "submitted_at" timestamptz NOT NULL, "processed_at" timestamptz NULL, "created_at" timestamptz NOT NULL, "updated_at" timestamptz NOT NULL, PRIMARY KEY ("id"));
-- Create index "datasubjectrequest_status" to table: "data_subject_requests"
CREATE INDEX "datasubjectrequest_status" ON "data_subject_requests" ("status");
-- Create index "datasubjectrequest_submitted_at" to table: "data_subject_requests"
CREATE INDEX "datasubjectrequest_submitted_at" ON "data_subject_requests" ("submitted_at");
-- Create index "datasubjectrequest_tenant_id_request_type" to table: "data_subject_requests"
CREATE INDEX "datasubjectrequest_tenant_id_request_type" ON "data_subject_requests" ("tenant_id", "request_type");
-- Create index "datasubjectrequest_tenant_id_status" to table: "data_subject_requests"
CREATE INDEX "datasubjectrequest_tenant_id_status" ON "data_subject_requests" ("tenant_id", "status");
-- Create index "datasubjectrequest_tenant_id_user_id" to table: "data_subject_requests"
CREATE INDEX "datasubjectrequest_tenant_id_user_id" ON "data_subject_requests" ("tenant_id", "user_id");
-- Create index "datasubjectrequest_user_id" to table: "data_subject_requests"
CREATE INDEX "datasubjectrequest_user_id" ON "data_subject_requests" ("user_id");
-- Create "outbox_events" table
CREATE TABLE "outbox_events" ("id" uuid NOT NULL, "tenant_id" uuid NOT NULL, "aggregate_type" character varying NOT NULL, "aggregate_id" uuid NOT NULL, "event_type" character varying NOT NULL, "payload" bytea NOT NULL, "status" character varying NOT NULL DEFAULT 'PENDING', "attempts" bigint NOT NULL DEFAULT 0, "last_attempt_at" timestamptz NULL, "published_at" timestamptz NULL, "error_message" character varying NULL, "created_at" timestamptz NOT NULL, PRIMARY KEY ("id"));
-- Create index "outboxevent_aggregate_type_aggregate_id" to table: "outbox_events"
CREATE INDEX "outboxevent_aggregate_type_aggregate_id" ON "outbox_events" ("aggregate_type", "aggregate_id");
-- Create index "outboxevent_status_created_at" to table: "outbox_events"
CREATE INDEX "outboxevent_status_created_at" ON "outbox_events" ("status", "created_at");
-- Create index "outboxevent_tenant_id_status" to table: "outbox_events"
CREATE INDEX "outboxevent_tenant_id_status" ON "outbox_events" ("tenant_id", "status");
-- Create "sla_metrics" table
CREATE TABLE "sla_metrics" ("id" uuid NOT NULL, "tenant_id" uuid NOT NULL, "order_id" uuid NOT NULL, "metric_type" character varying NOT NULL, "target_seconds" bigint NOT NULL, "actual_seconds" bigint NULL, "status" character varying NOT NULL DEFAULT 'tracking', "breach_percentage" double precision NULL, "started_at" timestamptz NOT NULL, "ended_at" timestamptz NULL, "measured_at" timestamptz NOT NULL, "metadata" jsonb NULL, "created_at" timestamptz NOT NULL, "updated_at" timestamptz NOT NULL, PRIMARY KEY ("id"));
-- Create index "slametric_measured_at" to table: "sla_metrics"
CREATE INDEX "slametric_measured_at" ON "sla_metrics" ("measured_at");
-- Create index "slametric_order_id_metric_type" to table: "sla_metrics"
CREATE UNIQUE INDEX "slametric_order_id_metric_type" ON "sla_metrics" ("order_id", "metric_type");
-- Create index "slametric_status" to table: "sla_metrics"
CREATE INDEX "slametric_status" ON "sla_metrics" ("status");
-- Create index "slametric_tenant_id_measured_at" to table: "sla_metrics"
CREATE INDEX "slametric_tenant_id_measured_at" ON "sla_metrics" ("tenant_id", "measured_at");
-- Create index "slametric_tenant_id_metric_type" to table: "sla_metrics"
CREATE INDEX "slametric_tenant_id_metric_type" ON "sla_metrics" ("tenant_id", "metric_type");
-- Create index "slametric_tenant_id_status" to table: "sla_metrics"
CREATE INDEX "slametric_tenant_id_status" ON "sla_metrics" ("tenant_id", "status");
-- Create "treasury_events" table
CREATE TABLE "treasury_events" ("id" uuid NOT NULL, "tenant_id" uuid NULL, "external_id" character varying NOT NULL, "event_type" character varying NOT NULL, "provider" character varying NULL, "order_id" uuid NULL, "payment_id" uuid NULL, "payment_intent_id" uuid NULL, "refund_id" uuid NULL, "payload" jsonb NOT NULL, "headers" jsonb NULL, "signature" character varying NULL, "signature_valid" boolean NULL, "status" character varying NOT NULL DEFAULT 'pending', "retry_count" bigint NOT NULL DEFAULT 0, "last_retry_at" timestamptz NULL, "error_message" text NULL, "error_code" character varying NULL, "ip_address" character varying NULL, "received_at" timestamptz NOT NULL, "processed_at" timestamptz NULL, "created_at" timestamptz NOT NULL, "updated_at" timestamptz NOT NULL, PRIMARY KEY ("id"));
-- Create index "treasury_events_external_id_key" to table: "treasury_events"
CREATE UNIQUE INDEX "treasury_events_external_id_key" ON "treasury_events" ("external_id");
-- Create index "treasuryevent_created_at" to table: "treasury_events"
CREATE INDEX "treasuryevent_created_at" ON "treasury_events" ("created_at");
-- Create index "treasuryevent_order_id" to table: "treasury_events"
CREATE INDEX "treasuryevent_order_id" ON "treasury_events" ("order_id");
-- Create index "treasuryevent_payment_id" to table: "treasury_events"
CREATE INDEX "treasuryevent_payment_id" ON "treasury_events" ("payment_id");
-- Create index "treasuryevent_payment_intent_id" to table: "treasury_events"
CREATE INDEX "treasuryevent_payment_intent_id" ON "treasury_events" ("payment_intent_id");
-- Create index "treasuryevent_received_at" to table: "treasury_events"
CREATE INDEX "treasuryevent_received_at" ON "treasury_events" ("received_at");
-- Create index "treasuryevent_status" to table: "treasury_events"
CREATE INDEX "treasuryevent_status" ON "treasury_events" ("status");
-- Create index "treasuryevent_tenant_id_event_type" to table: "treasury_events"
CREATE INDEX "treasuryevent_tenant_id_event_type" ON "treasury_events" ("tenant_id", "event_type");
-- Create "tenants" table
CREATE TABLE "tenants" ("id" uuid NOT NULL, "name" character varying NOT NULL, "slug" character varying NOT NULL, "status" character varying NOT NULL DEFAULT 'active', "contact_email" character varying NULL, "contact_phone" character varying NULL, "logo_url" character varying NULL, "website" character varying NULL, "country" character varying NULL DEFAULT 'KE', "timezone" character varying NULL DEFAULT 'Africa/Nairobi', "brand_colors" jsonb NULL, "org_size" character varying NULL, "use_case" character varying NULL, "subscription_plan" character varying NULL, "subscription_status" character varying NULL, "subscription_expires_at" timestamptz NULL, "subscription_id" character varying NULL, "tier_limits" jsonb NULL, "metadata" jsonb NULL, "created_at" timestamptz NOT NULL, "updated_at" timestamptz NOT NULL, PRIMARY KEY ("id"));
-- Create index "tenant_slug" to table: "tenants"
CREATE UNIQUE INDEX "tenant_slug" ON "tenants" ("slug");
-- Create index "tenant_status" to table: "tenants"
CREATE INDEX "tenant_status" ON "tenants" ("status");
-- Create index "tenant_subscription_plan" to table: "tenants"
CREATE INDEX "tenant_subscription_plan" ON "tenants" ("subscription_plan");
-- Create index "tenants_slug_key" to table: "tenants"
CREATE UNIQUE INDEX "tenants_slug_key" ON "tenants" ("slug");
-- Create "two_factor_settings" table
CREATE TABLE "two_factor_settings" ("id" bigint NOT NULL GENERATED BY DEFAULT AS IDENTITY, "method" character varying NOT NULL DEFAULT 'totp', "secret" character varying NULL, "backup_phone" character varying NULL, "enabled" boolean NOT NULL DEFAULT false, "created_at" timestamptz NOT NULL, "updated_at" timestamptz NOT NULL, PRIMARY KEY ("id"));
-- Create "users" table
CREATE TABLE "users" ("id" uuid NOT NULL, "auth_service_user_id" uuid NULL, "email" character varying NOT NULL, "password_hash" character varying NULL, "sync_status" character varying NOT NULL DEFAULT 'pending', "sync_at" timestamptz NULL, "full_name" character varying NOT NULL, "phone" character varying NULL, "status" character varying NOT NULL DEFAULT 'active', "primary_role" character varying NULL, "locale" character varying NOT NULL DEFAULT 'en', "timezone" character varying NOT NULL DEFAULT 'Africa/Nairobi', "last_login_at" timestamptz NULL, "metadata" jsonb NOT NULL, "created_at" timestamptz NOT NULL, "updated_at" timestamptz NOT NULL, "tenant_id" uuid NOT NULL, "two_factor_setting_user" bigint NULL, PRIMARY KEY ("id"), CONSTRAINT "users_tenants_users" FOREIGN KEY ("tenant_id") REFERENCES "tenants" ("id") ON UPDATE NO ACTION ON DELETE NO ACTION, CONSTRAINT "users_two_factor_settings_user" FOREIGN KEY ("two_factor_setting_user") REFERENCES "two_factor_settings" ("id") ON UPDATE NO ACTION ON DELETE SET NULL);
-- Create index "user_created_at" to table: "users"
CREATE INDEX "user_created_at" ON "users" ("created_at");
-- Create index "user_email" to table: "users"
CREATE INDEX "user_email" ON "users" ("email");
-- Create index "user_last_login_at" to table: "users"
CREATE INDEX "user_last_login_at" ON "users" ("last_login_at");
-- Create index "user_phone" to table: "users"
CREATE INDEX "user_phone" ON "users" ("phone");
-- Create index "user_sync_status" to table: "users"
CREATE INDEX "user_sync_status" ON "users" ("sync_status");
-- Create index "user_tenant_id_auth_service_user_id" to table: "users"
CREATE INDEX "user_tenant_id_auth_service_user_id" ON "users" ("tenant_id", "auth_service_user_id");
-- Create index "user_tenant_id_email" to table: "users"
CREATE UNIQUE INDEX "user_tenant_id_email" ON "users" ("tenant_id", "email");
-- Create index "user_tenant_id_primary_role" to table: "users"
CREATE INDEX "user_tenant_id_primary_role" ON "users" ("tenant_id", "primary_role");
-- Create index "user_tenant_id_status" to table: "users"
CREATE INDEX "user_tenant_id_status" ON "users" ("tenant_id", "status");
-- Create index "users_auth_service_user_id_key" to table: "users"
CREATE UNIQUE INDEX "users_auth_service_user_id_key" ON "users" ("auth_service_user_id");
-- Create index "users_two_factor_setting_user_key" to table: "users"
CREATE UNIQUE INDEX "users_two_factor_setting_user_key" ON "users" ("two_factor_setting_user");
-- Create "backup_codes" table
CREATE TABLE "backup_codes" ("id" uuid NOT NULL, "code_hash" character varying NOT NULL, "used_at" timestamptz NULL, "created_at" timestamptz NOT NULL, "backup_code_user" uuid NOT NULL, PRIMARY KEY ("id"), CONSTRAINT "backup_codes_users_user" FOREIGN KEY ("backup_code_user") REFERENCES "users" ("id") ON UPDATE NO ACTION ON DELETE NO ACTION);
-- Create "carts" table
CREATE TABLE "carts" ("id" uuid NOT NULL, "tenant_id" uuid NOT NULL, "cafe_id" uuid NOT NULL, "session_id" character varying NULL, "status" character varying NOT NULL DEFAULT 'active', "currency" character varying NOT NULL DEFAULT 'KES', "subtotal" double precision NOT NULL DEFAULT 0, "discount_total" double precision NOT NULL DEFAULT 0, "tax_total" double precision NOT NULL DEFAULT 0, "delivery_fee" double precision NOT NULL DEFAULT 0, "loyalty_points_redeemed" bigint NOT NULL DEFAULT 0, "promo_code_id" uuid NULL, "expires_at" timestamptz NULL, "created_at" timestamptz NOT NULL, "updated_at" timestamptz NOT NULL, "user_id" uuid NULL, PRIMARY KEY ("id"), CONSTRAINT "carts_users_carts" FOREIGN KEY ("user_id") REFERENCES "users" ("id") ON UPDATE NO ACTION ON DELETE SET NULL);
-- Create index "cart_expires_at" to table: "carts"
CREATE INDEX "cart_expires_at" ON "carts" ("expires_at");
-- Create index "cart_status" to table: "carts"
CREATE INDEX "cart_status" ON "carts" ("status");
-- Create index "cart_tenant_id_session_id_status" to table: "carts"
CREATE UNIQUE INDEX "cart_tenant_id_session_id_status" ON "carts" ("tenant_id", "session_id", "status") WHERE (((status)::text = 'active'::text) AND (session_id IS NOT NULL));
-- Create index "cart_tenant_id_user_id_status" to table: "carts"
CREATE UNIQUE INDEX "cart_tenant_id_user_id_status" ON "carts" ("tenant_id", "user_id", "status") WHERE (((status)::text = 'active'::text) AND (user_id IS NOT NULL));
-- Create "menu_categories" table
CREATE TABLE "menu_categories" ("id" uuid NOT NULL, "tenant_id" uuid NOT NULL, "cafe_id" uuid NOT NULL, "name" character varying NOT NULL, "description" text NULL, "display_order" bigint NOT NULL DEFAULT 0, "is_active" boolean NOT NULL DEFAULT true, "image_url" character varying NULL, "created_at" timestamptz NOT NULL, "updated_at" timestamptz NOT NULL, "parent_id" uuid NULL, PRIMARY KEY ("id"), CONSTRAINT "menu_categories_menu_categories_children" FOREIGN KEY ("parent_id") REFERENCES "menu_categories" ("id") ON UPDATE NO ACTION ON DELETE SET NULL);
-- Create index "menucategory_display_order" to table: "menu_categories"
CREATE INDEX "menucategory_display_order" ON "menu_categories" ("display_order");
-- Create index "menucategory_tenant_id_cafe_id" to table: "menu_categories"
CREATE INDEX "menucategory_tenant_id_cafe_id" ON "menu_categories" ("tenant_id", "cafe_id");
-- Create index "menucategory_tenant_id_is_active" to table: "menu_categories"
CREATE INDEX "menucategory_tenant_id_is_active" ON "menu_categories" ("tenant_id", "is_active");
-- Create "menu_items" table
CREATE TABLE "menu_items" ("id" uuid NOT NULL, "tenant_id" uuid NOT NULL, "cafe_id" uuid NOT NULL, "name" character varying NOT NULL, "description" text NULL, "base_price" double precision NOT NULL, "currency" character varying NOT NULL DEFAULT 'KES', "is_available" boolean NOT NULL DEFAULT true, "lead_time_minutes" bigint NULL, "image_url" character varying NULL, "nutrition_json" jsonb NULL, "sku" character varying NULL, "display_order" bigint NOT NULL DEFAULT 0, "created_at" timestamptz NOT NULL, "updated_at" timestamptz NOT NULL, "category_id" uuid NOT NULL, PRIMARY KEY ("id"), CONSTRAINT "menu_items_menu_categories_items" FOREIGN KEY ("category_id") REFERENCES "menu_categories" ("id") ON UPDATE NO ACTION ON DELETE NO ACTION);
-- Create index "menuitem_display_order" to table: "menu_items"
CREATE INDEX "menuitem_display_order" ON "menu_items" ("display_order");
-- Create index "menuitem_sku" to table: "menu_items"
CREATE INDEX "menuitem_sku" ON "menu_items" ("sku");
-- Create index "menuitem_tenant_id_cafe_id" to table: "menu_items"
CREATE INDEX "menuitem_tenant_id_cafe_id" ON "menu_items" ("tenant_id", "cafe_id");
-- Create index "menuitem_tenant_id_category_id" to table: "menu_items"
CREATE INDEX "menuitem_tenant_id_category_id" ON "menu_items" ("tenant_id", "category_id");
-- Create index "menuitem_tenant_id_is_available" to table: "menu_items"
CREATE INDEX "menuitem_tenant_id_is_available" ON "menu_items" ("tenant_id", "is_available");
-- Create "menu_item_variants" table
CREATE TABLE "menu_item_variants" ("id" uuid NOT NULL, "name" character varying NOT NULL, "price_delta" double precision NOT NULL DEFAULT 0, "is_available" boolean NOT NULL DEFAULT true, "sku" character varying NULL, "display_order" bigint NOT NULL DEFAULT 0, "created_at" timestamptz NOT NULL, "updated_at" timestamptz NOT NULL, "menu_item_id" uuid NOT NULL, PRIMARY KEY ("id"), CONSTRAINT "menu_item_variants_menu_items_variants" FOREIGN KEY ("menu_item_id") REFERENCES "menu_items" ("id") ON UPDATE NO ACTION ON DELETE NO ACTION);
-- Create index "menuitemvariant_is_available" to table: "menu_item_variants"
CREATE INDEX "menuitemvariant_is_available" ON "menu_item_variants" ("is_available");
-- Create index "menuitemvariant_menu_item_id" to table: "menu_item_variants"
CREATE INDEX "menuitemvariant_menu_item_id" ON "menu_item_variants" ("menu_item_id");
-- Create index "menuitemvariant_sku" to table: "menu_item_variants"
CREATE INDEX "menuitemvariant_sku" ON "menu_item_variants" ("sku");
-- Create "cart_items" table
CREATE TABLE "cart_items" ("id" uuid NOT NULL, "name_snapshot" character varying NOT NULL, "variant_name_snapshot" character varying NULL, "quantity" bigint NOT NULL DEFAULT 1, "unit_price" double precision NOT NULL, "total_price" double precision NOT NULL, "notes" text NULL, "modifiers" jsonb NULL, "metadata" jsonb NULL, "created_at" timestamptz NOT NULL, "updated_at" timestamptz NOT NULL, "cart_id" uuid NOT NULL, "menu_item_id" uuid NOT NULL, "variant_id" uuid NULL, PRIMARY KEY ("id"), CONSTRAINT "cart_items_carts_items" FOREIGN KEY ("cart_id") REFERENCES "carts" ("id") ON UPDATE NO ACTION ON DELETE NO ACTION, CONSTRAINT "cart_items_menu_item_variants_cart_items" FOREIGN KEY ("variant_id") REFERENCES "menu_item_variants" ("id") ON UPDATE NO ACTION ON DELETE SET NULL, CONSTRAINT "cart_items_menu_items_cart_items" FOREIGN KEY ("menu_item_id") REFERENCES "menu_items" ("id") ON UPDATE NO ACTION ON DELETE NO ACTION);
-- Create index "cartitem_cart_id" to table: "cart_items"
CREATE INDEX "cartitem_cart_id" ON "cart_items" ("cart_id");
-- Create index "cartitem_cart_id_menu_item_id_variant_id" to table: "cart_items"
CREATE UNIQUE INDEX "cartitem_cart_id_menu_item_id_variant_id" ON "cart_items" ("cart_id", "menu_item_id", "variant_id");
-- Create index "cartitem_menu_item_id" to table: "cart_items"
CREATE INDEX "cartitem_menu_item_id" ON "cart_items" ("menu_item_id");
-- Create "customer_addresses" table
CREATE TABLE "customer_addresses" ("id" uuid NOT NULL, "tenant_id" uuid NOT NULL, "label" character varying NOT NULL, "address_line1" character varying NOT NULL, "address_line2" character varying NULL, "city" character varying NOT NULL, "county" character varying NULL, "postal_code" character varying NULL, "country" character varying NOT NULL DEFAULT 'KE', "latitude" double precision NULL, "longitude" double precision NULL, "plus_code" character varying NULL, "instructions" text NULL, "contact_name" character varying NULL, "contact_phone" character varying NULL, "is_default" boolean NOT NULL DEFAULT false, "is_verified" boolean NOT NULL DEFAULT false, "created_at" timestamptz NOT NULL, "updated_at" timestamptz NOT NULL, "user_id" uuid NOT NULL, PRIMARY KEY ("id"), CONSTRAINT "customer_addresses_users_addresses" FOREIGN KEY ("user_id") REFERENCES "users" ("id") ON UPDATE NO ACTION ON DELETE NO ACTION);
-- Create index "customeraddress_latitude_longitude" to table: "customer_addresses"
CREATE INDEX "customeraddress_latitude_longitude" ON "customer_addresses" ("latitude", "longitude");
-- Create index "customeraddress_tenant_id_user_id" to table: "customer_addresses"
CREATE INDEX "customeraddress_tenant_id_user_id" ON "customer_addresses" ("tenant_id", "user_id");
-- Create index "customeraddress_user_id_is_default" to table: "customer_addresses"
CREATE INDEX "customeraddress_user_id_is_default" ON "customer_addresses" ("user_id", "is_default");
-- Create "orders" table
CREATE TABLE "orders" ("id" uuid NOT NULL, "tenant_id" uuid NOT NULL, "cafe_id" uuid NOT NULL, "cart_id" uuid NULL, "order_number" character varying NOT NULL, "status" character varying NOT NULL DEFAULT 'pending', "payment_status" character varying NOT NULL DEFAULT 'pending', "currency" character varying NOT NULL DEFAULT 'KES', "subtotal" double precision NOT NULL, "discount_total" double precision NOT NULL DEFAULT 0, "tax_total" double precision NOT NULL DEFAULT 0, "delivery_fee" double precision NOT NULL DEFAULT 0, "tip_total" double precision NOT NULL DEFAULT 0, "grand_total" double precision NOT NULL, "loyalty_points_earned" bigint NOT NULL DEFAULT 0, "loyalty_points_redeemed" bigint NOT NULL DEFAULT 0, "promo_code_id" uuid NULL, "instructions" text NULL, "channel" character varying NOT NULL DEFAULT 'web', "source" character varying NULL, "idempotency_key" character varying NULL, "placed_at" timestamptz NULL, "confirmed_at" timestamptz NULL, "ready_at" timestamptz NULL, "delivered_at" timestamptz NULL, "completed_at" timestamptz NULL, "cancelled_at" timestamptz NULL, "cancellation_reason" text NULL, "rating" bigint NULL, "rating_comment" text NULL, "rated_at" timestamptz NULL, "metadata" jsonb NULL, "created_at" timestamptz NOT NULL, "updated_at" timestamptz NOT NULL, "delivery_address_id" uuid NULL, "customer_id" uuid NOT NULL, PRIMARY KEY ("id"), CONSTRAINT "orders_customer_addresses_orders" FOREIGN KEY ("delivery_address_id") REFERENCES "customer_addresses" ("id") ON UPDATE NO ACTION ON DELETE SET NULL, CONSTRAINT "orders_users_orders" FOREIGN KEY ("customer_id") REFERENCES "users" ("id") ON UPDATE NO ACTION ON DELETE NO ACTION);
-- Create index "order_channel" to table: "orders"
CREATE INDEX "order_channel" ON "orders" ("channel");
-- Create index "order_completed_at" to table: "orders"
CREATE INDEX "order_completed_at" ON "orders" ("completed_at");
-- Create index "order_created_at" to table: "orders"
CREATE INDEX "order_created_at" ON "orders" ("created_at");
-- Create index "order_delivered_at" to table: "orders"
CREATE INDEX "order_delivered_at" ON "orders" ("delivered_at");
-- Create index "order_delivery_address_id" to table: "orders"
CREATE INDEX "order_delivery_address_id" ON "orders" ("delivery_address_id");
-- Create index "order_idempotency_key" to table: "orders"
CREATE UNIQUE INDEX "order_idempotency_key" ON "orders" ("idempotency_key") WHERE (idempotency_key IS NOT NULL);
-- Create index "order_order_number" to table: "orders"
CREATE UNIQUE INDEX "order_order_number" ON "orders" ("order_number");
-- Create index "order_placed_at" to table: "orders"
CREATE INDEX "order_placed_at" ON "orders" ("placed_at");
-- Create index "order_tenant_id_cafe_id" to table: "orders"
CREATE INDEX "order_tenant_id_cafe_id" ON "orders" ("tenant_id", "cafe_id");
-- Create index "order_tenant_id_cafe_id_status" to table: "orders"
CREATE INDEX "order_tenant_id_cafe_id_status" ON "orders" ("tenant_id", "cafe_id", "status");
-- Create index "order_tenant_id_customer_id" to table: "orders"
CREATE INDEX "order_tenant_id_customer_id" ON "orders" ("tenant_id", "customer_id");
-- Create index "order_tenant_id_customer_id_status" to table: "orders"
CREATE INDEX "order_tenant_id_customer_id_status" ON "orders" ("tenant_id", "customer_id", "status");
-- Create index "order_tenant_id_payment_status" to table: "orders"
CREATE INDEX "order_tenant_id_payment_status" ON "orders" ("tenant_id", "payment_status");
-- Create index "order_tenant_id_status" to table: "orders"
CREATE INDEX "order_tenant_id_status" ON "orders" ("tenant_id", "status");
-- Create index "order_tenant_id_status_created_at" to table: "orders"
CREATE INDEX "order_tenant_id_status_created_at" ON "orders" ("tenant_id", "status", "created_at");
-- Create "order_assignments" table
CREATE TABLE "order_assignments" ("id" uuid NOT NULL, "tenant_id" uuid NOT NULL, "logistics_task_id" character varying NOT NULL, "rider_id" character varying NULL, "status" character varying NOT NULL DEFAULT 'pending', "priority" character varying NOT NULL DEFAULT 'normal', "special_instructions" text NULL, "rejection_reason" character varying NULL, "cancellation_reason" character varying NULL, "failure_reason" character varying NULL, "attempt_count" bigint NOT NULL DEFAULT 1, "metadata" jsonb NULL, "assigned_at" timestamptz NULL, "accepted_at" timestamptz NULL, "picked_up_at" timestamptz NULL, "completed_at" timestamptz NULL, "cancelled_at" timestamptz NULL, "created_at" timestamptz NOT NULL, "updated_at" timestamptz NOT NULL, "order_id" uuid NOT NULL, PRIMARY KEY ("id"), CONSTRAINT "order_assignments_orders_assignments" FOREIGN KEY ("order_id") REFERENCES "orders" ("id") ON UPDATE NO ACTION ON DELETE NO ACTION);
-- Create index "orderassignment_created_at" to table: "order_assignments"
CREATE INDEX "orderassignment_created_at" ON "order_assignments" ("created_at");
-- Create index "orderassignment_logistics_task_id" to table: "order_assignments"
CREATE UNIQUE INDEX "orderassignment_logistics_task_id" ON "order_assignments" ("logistics_task_id");
-- Create index "orderassignment_rider_id" to table: "order_assignments"
CREATE INDEX "orderassignment_rider_id" ON "order_assignments" ("rider_id");
-- Create index "orderassignment_status" to table: "order_assignments"
CREATE INDEX "orderassignment_status" ON "order_assignments" ("status");
-- Create index "orderassignment_tenant_id_order_id" to table: "order_assignments"
CREATE INDEX "orderassignment_tenant_id_order_id" ON "order_assignments" ("tenant_id", "order_id");
-- Create "delivery_windows" table
CREATE TABLE "delivery_windows" ("id" uuid NOT NULL, "tenant_id" uuid NOT NULL, "order_id" uuid NOT NULL, "eta_start" timestamptz NOT NULL, "eta_end" timestamptz NOT NULL, "eta_minutes" bigint NULL, "distance_km" double precision NULL, "actual_arrival" timestamptz NULL, "actual_dropoff" timestamptz NULL, "source" character varying NOT NULL DEFAULT 'logistics', "is_current" boolean NOT NULL DEFAULT true, "route_info" jsonb NULL, "created_at" timestamptz NOT NULL, "updated_at" timestamptz NOT NULL, "assignment_id" uuid NOT NULL, PRIMARY KEY ("id"), CONSTRAINT "delivery_windows_order_assignments_delivery_windows" FOREIGN KEY ("assignment_id") REFERENCES "order_assignments" ("id") ON UPDATE NO ACTION ON DELETE NO ACTION);
-- Create index "deliverywindow_assignment_id" to table: "delivery_windows"
CREATE INDEX "deliverywindow_assignment_id" ON "delivery_windows" ("assignment_id");
-- Create index "deliverywindow_created_at" to table: "delivery_windows"
CREATE INDEX "deliverywindow_created_at" ON "delivery_windows" ("created_at");
-- Create index "deliverywindow_is_current" to table: "delivery_windows"
CREATE INDEX "deliverywindow_is_current" ON "delivery_windows" ("is_current");
-- Create index "deliverywindow_tenant_id_order_id" to table: "delivery_windows"
CREATE INDEX "deliverywindow_tenant_id_order_id" ON "delivery_windows" ("tenant_id", "order_id");
-- Create "loyalty_accounts" table
CREATE TABLE "loyalty_accounts" ("id" uuid NOT NULL, "tenant_id" uuid NOT NULL, "balance_points" bigint NOT NULL DEFAULT 0, "lifetime_points" bigint NOT NULL DEFAULT 0, "redeemed_points" bigint NOT NULL DEFAULT 0, "tier" character varying NOT NULL DEFAULT 'bronze', "tier_progress" bigint NOT NULL DEFAULT 0, "tier_expires_at" timestamptz NULL, "created_at" timestamptz NOT NULL, "updated_at" timestamptz NOT NULL, "user_id" uuid NOT NULL, PRIMARY KEY ("id"), CONSTRAINT "loyalty_accounts_users_loyalty_account" FOREIGN KEY ("user_id") REFERENCES "users" ("id") ON UPDATE NO ACTION ON DELETE NO ACTION);
-- Create index "loyalty_accounts_user_id_key" to table: "loyalty_accounts"
CREATE UNIQUE INDEX "loyalty_accounts_user_id_key" ON "loyalty_accounts" ("user_id");
-- Create index "loyaltyaccount_tenant_id_user_id" to table: "loyalty_accounts"
CREATE UNIQUE INDEX "loyaltyaccount_tenant_id_user_id" ON "loyalty_accounts" ("tenant_id", "user_id");
-- Create index "loyaltyaccount_tier" to table: "loyalty_accounts"
CREATE INDEX "loyaltyaccount_tier" ON "loyalty_accounts" ("tier");
-- Create "loyalty_transactions" table
CREATE TABLE "loyalty_transactions" ("id" uuid NOT NULL, "order_id" uuid NULL, "points" bigint NOT NULL, "balance_after" bigint NOT NULL, "transaction_type" character varying NOT NULL, "description" text NULL, "reference" character varying NULL, "metadata" jsonb NULL, "occurred_at" timestamptz NOT NULL, "account_id" uuid NOT NULL, PRIMARY KEY ("id"), CONSTRAINT "loyalty_transactions_loyalty_accounts_transactions" FOREIGN KEY ("account_id") REFERENCES "loyalty_accounts" ("id") ON UPDATE NO ACTION ON DELETE NO ACTION);
-- Create index "loyaltytransaction_account_id" to table: "loyalty_transactions"
CREATE INDEX "loyaltytransaction_account_id" ON "loyalty_transactions" ("account_id");
-- Create index "loyaltytransaction_occurred_at" to table: "loyalty_transactions"
CREATE INDEX "loyaltytransaction_occurred_at" ON "loyalty_transactions" ("occurred_at");
-- Create index "loyaltytransaction_order_id" to table: "loyalty_transactions"
CREATE INDEX "loyaltytransaction_order_id" ON "loyalty_transactions" ("order_id");
-- Create index "loyaltytransaction_transaction_type" to table: "loyalty_transactions"
CREATE INDEX "loyaltytransaction_transaction_type" ON "loyalty_transactions" ("transaction_type");
-- Create "menu_item_assets" table
CREATE TABLE "menu_item_assets" ("id" uuid NOT NULL, "asset_type" character varying NOT NULL, "url" character varying NOT NULL, "metadata" jsonb NULL, "display_order" bigint NOT NULL DEFAULT 0, "created_at" timestamptz NOT NULL, "menu_item_id" uuid NOT NULL, PRIMARY KEY ("id"), CONSTRAINT "menu_item_assets_menu_items_assets" FOREIGN KEY ("menu_item_id") REFERENCES "menu_items" ("id") ON UPDATE NO ACTION ON DELETE NO ACTION);
-- Create index "menuitemasset_asset_type" to table: "menu_item_assets"
CREATE INDEX "menuitemasset_asset_type" ON "menu_item_assets" ("asset_type");
-- Create index "menuitemasset_menu_item_id" to table: "menu_item_assets"
CREATE INDEX "menuitemasset_menu_item_id" ON "menu_item_assets" ("menu_item_id");
-- Create "dietary_tags" table
CREATE TABLE "dietary_tags" ("id" bigint NOT NULL GENERATED BY DEFAULT AS IDENTITY, "code" character varying NOT NULL, "label" character varying NOT NULL, "description" text NULL, "icon_url" character varying NULL, "created_at" timestamptz NOT NULL, PRIMARY KEY ("id"));
-- Create index "dietary_tags_code_key" to table: "dietary_tags"
CREATE UNIQUE INDEX "dietary_tags_code_key" ON "dietary_tags" ("code");
-- Create "menu_item_dietary_tags" table
CREATE TABLE "menu_item_dietary_tags" ("menu_item_id" uuid NOT NULL, "dietary_tag_id" bigint NOT NULL, PRIMARY KEY ("menu_item_id", "dietary_tag_id"), CONSTRAINT "menu_item_dietary_tags_dietary_tag_id" FOREIGN KEY ("dietary_tag_id") REFERENCES "dietary_tags" ("id") ON UPDATE NO ACTION ON DELETE CASCADE, CONSTRAINT "menu_item_dietary_tags_menu_item_id" FOREIGN KEY ("menu_item_id") REFERENCES "menu_items" ("id") ON UPDATE NO ACTION ON DELETE CASCADE);
-- Create "menu_item_schedules" table
CREATE TABLE "menu_item_schedules" ("id" uuid NOT NULL, "day_of_week" bigint NOT NULL, "time_start" character varying NOT NULL, "time_end" character varying NOT NULL, "created_at" timestamptz NOT NULL, "menu_item_id" uuid NOT NULL, PRIMARY KEY ("id"), CONSTRAINT "menu_item_schedules_menu_items_schedules" FOREIGN KEY ("menu_item_id") REFERENCES "menu_items" ("id") ON UPDATE NO ACTION ON DELETE NO ACTION);
-- Create index "menuitemschedule_day_of_week" to table: "menu_item_schedules"
CREATE INDEX "menuitemschedule_day_of_week" ON "menu_item_schedules" ("day_of_week");
-- Create index "menuitemschedule_menu_item_id" to table: "menu_item_schedules"
CREATE INDEX "menuitemschedule_menu_item_id" ON "menu_item_schedules" ("menu_item_id");
-- Create "menu_item_translations" table
CREATE TABLE "menu_item_translations" ("id" uuid NOT NULL, "locale" character varying NOT NULL, "name" character varying NOT NULL, "description" text NULL, "created_at" timestamptz NOT NULL, "updated_at" timestamptz NOT NULL, "menu_item_id" uuid NOT NULL, PRIMARY KEY ("id"), CONSTRAINT "menu_item_translations_menu_items_translations" FOREIGN KEY ("menu_item_id") REFERENCES "menu_items" ("id") ON UPDATE NO ACTION ON DELETE NO ACTION);
-- Create index "menuitemtranslation_menu_item_id_locale" to table: "menu_item_translations"
CREATE UNIQUE INDEX "menuitemtranslation_menu_item_id_locale" ON "menu_item_translations" ("menu_item_id", "locale");
-- Create "oauth_accounts" table
CREATE TABLE "oauth_accounts" ("id" uuid NOT NULL, "provider" character varying NOT NULL, "provider_account_id" character varying NOT NULL, "access_token" character varying NULL, "refresh_token" character varying NULL, "expires_at" timestamptz NULL, "scopes" jsonb NULL, "metadata" jsonb NOT NULL, "created_at" timestamptz NOT NULL, "updated_at" timestamptz NOT NULL, "user_oauth_accounts" uuid NULL, PRIMARY KEY ("id"), CONSTRAINT "oauth_accounts_users_oauth_accounts" FOREIGN KEY ("user_oauth_accounts") REFERENCES "users" ("id") ON UPDATE NO ACTION ON DELETE SET NULL);
-- Create index "oauthaccount_provider_provider_account_id" to table: "oauth_accounts"
CREATE UNIQUE INDEX "oauthaccount_provider_provider_account_id" ON "oauth_accounts" ("provider", "provider_account_id");
-- Create "order_events" table
CREATE TABLE "order_events" ("id" uuid NOT NULL, "event_type" character varying NOT NULL, "from_status" character varying NULL, "to_status" character varying NULL, "payload" jsonb NULL, "actor_user_id" uuid NULL, "actor_type" character varying NULL, "ip_address" character varying NULL, "occurred_at" timestamptz NOT NULL, "order_id" uuid NOT NULL, PRIMARY KEY ("id"), CONSTRAINT "order_events_orders_events" FOREIGN KEY ("order_id") REFERENCES "orders" ("id") ON UPDATE NO ACTION ON DELETE NO ACTION);
-- Create index "orderevent_event_type" to table: "order_events"
CREATE INDEX "orderevent_event_type" ON "order_events" ("event_type");
-- Create index "orderevent_occurred_at" to table: "order_events"
CREATE INDEX "orderevent_occurred_at" ON "order_events" ("occurred_at");
-- Create index "orderevent_order_id" to table: "order_events"
CREATE INDEX "orderevent_order_id" ON "order_events" ("order_id");
-- Create "order_items" table
CREATE TABLE "order_items" ("id" uuid NOT NULL, "menu_item_id" uuid NOT NULL, "variant_id" uuid NULL, "name_snapshot" character varying NOT NULL, "variant_name_snapshot" character varying NULL, "quantity" bigint NOT NULL, "unit_price" double precision NOT NULL, "total_price" double precision NOT NULL, "notes" text NULL, "modifiers" jsonb NULL, "metadata" jsonb NULL, "created_at" timestamptz NOT NULL, "order_id" uuid NOT NULL, PRIMARY KEY ("id"), CONSTRAINT "order_items_orders_items" FOREIGN KEY ("order_id") REFERENCES "orders" ("id") ON UPDATE NO ACTION ON DELETE NO ACTION);
-- Create index "orderitem_menu_item_id" to table: "order_items"
CREATE INDEX "orderitem_menu_item_id" ON "order_items" ("menu_item_id");
-- Create index "orderitem_order_id" to table: "order_items"
CREATE INDEX "orderitem_order_id" ON "order_items" ("order_id");
-- Create "payment_methods" table
CREATE TABLE "payment_methods" ("id" uuid NOT NULL, "tenant_id" uuid NOT NULL, "provider" character varying NOT NULL, "type" character varying NOT NULL, "mask" character varying NULL, "label" character varying NULL, "exp_month" bigint NULL, "exp_year" bigint NULL, "is_default" boolean NOT NULL DEFAULT false, "fingerprint" character varying NULL, "provider_token" character varying NULL, "metadata" jsonb NULL, "created_at" timestamptz NOT NULL, "updated_at" timestamptz NOT NULL, "user_id" uuid NOT NULL, PRIMARY KEY ("id"), CONSTRAINT "payment_methods_users_payment_methods" FOREIGN KEY ("user_id") REFERENCES "users" ("id") ON UPDATE NO ACTION ON DELETE NO ACTION);
-- Create index "paymentmethod_provider_provider_token" to table: "payment_methods"
CREATE INDEX "paymentmethod_provider_provider_token" ON "payment_methods" ("provider", "provider_token");
-- Create index "paymentmethod_tenant_id_fingerprint" to table: "payment_methods"
CREATE UNIQUE INDEX "paymentmethod_tenant_id_fingerprint" ON "payment_methods" ("tenant_id", "fingerprint");
-- Create index "paymentmethod_tenant_id_user_id" to table: "payment_methods"
CREATE INDEX "paymentmethod_tenant_id_user_id" ON "payment_methods" ("tenant_id", "user_id");
-- Create "payment_intents" table
CREATE TABLE "payment_intents" ("id" uuid NOT NULL, "tenant_id" uuid NOT NULL, "provider" character varying NOT NULL, "provider_intent_id" character varying NULL, "client_secret" character varying NULL, "status" character varying NOT NULL DEFAULT 'pending', "amount" double precision NOT NULL, "currency" character varying NOT NULL DEFAULT 'KES', "description" character varying NULL, "idempotency_key" character varying NULL, "mpesa_checkout_request_id" character varying NULL, "mpesa_phone_number" character varying NULL, "retry_count" bigint NOT NULL DEFAULT 0, "last_retry_at" timestamptz NULL, "error_message" text NULL, "error_code" character varying NULL, "metadata" jsonb NULL, "expires_at" timestamptz NULL, "created_at" timestamptz NOT NULL, "updated_at" timestamptz NOT NULL, "order_id" uuid NOT NULL, "payment_method_id" uuid NULL, PRIMARY KEY ("id"), CONSTRAINT "payment_intents_orders_payment_intents" FOREIGN KEY ("order_id") REFERENCES "orders" ("id") ON UPDATE NO ACTION ON DELETE NO ACTION, CONSTRAINT "payment_intents_payment_methods_payment_intents" FOREIGN KEY ("payment_method_id") REFERENCES "payment_methods" ("id") ON UPDATE NO ACTION ON DELETE SET NULL);
-- Create index "paymentintent_created_at" to table: "payment_intents"
CREATE INDEX "paymentintent_created_at" ON "payment_intents" ("created_at");
-- Create index "paymentintent_idempotency_key" to table: "payment_intents"
CREATE UNIQUE INDEX "paymentintent_idempotency_key" ON "payment_intents" ("idempotency_key") WHERE (idempotency_key IS NOT NULL);
-- Create index "paymentintent_mpesa_checkout_request_id" to table: "payment_intents"
CREATE INDEX "paymentintent_mpesa_checkout_request_id" ON "payment_intents" ("mpesa_checkout_request_id");
-- Create index "paymentintent_provider_provider_intent_id" to table: "payment_intents"
CREATE INDEX "paymentintent_provider_provider_intent_id" ON "payment_intents" ("provider", "provider_intent_id");
-- Create index "paymentintent_status" to table: "payment_intents"
CREATE INDEX "paymentintent_status" ON "payment_intents" ("status");
-- Create index "paymentintent_tenant_id_order_id" to table: "payment_intents"
CREATE INDEX "paymentintent_tenant_id_order_id" ON "payment_intents" ("tenant_id", "order_id");
-- Create "payments" table
CREATE TABLE "payments" ("id" uuid NOT NULL, "tenant_id" uuid NOT NULL, "amount" double precision NOT NULL, "currency" character varying NOT NULL DEFAULT 'KES', "status" character varying NOT NULL DEFAULT 'pending', "provider" character varying NOT NULL, "provider_reference" character varying NULL, "provider_receipt" character varying NULL, "mpesa_transaction_id" character varying NULL, "mpesa_phone_number" character varying NULL, "refunded_amount" double precision NOT NULL DEFAULT 0, "provider_response" jsonb NULL, "metadata" jsonb NULL, "processed_at" timestamptz NULL, "captured_at" timestamptz NULL, "created_at" timestamptz NOT NULL, "updated_at" timestamptz NOT NULL, "order_id" uuid NOT NULL, "payment_intent_id" uuid NOT NULL, PRIMARY KEY ("id"), CONSTRAINT "payments_orders_payments" FOREIGN KEY ("order_id") REFERENCES "orders" ("id") ON UPDATE NO ACTION ON DELETE NO ACTION, CONSTRAINT "payments_payment_intents_payments" FOREIGN KEY ("payment_intent_id") REFERENCES "payment_intents" ("id") ON UPDATE NO ACTION ON DELETE NO ACTION);
-- Create index "payment_captured_at" to table: "payments"
CREATE INDEX "payment_captured_at" ON "payments" ("captured_at");
-- Create index "payment_created_at" to table: "payments"
CREATE INDEX "payment_created_at" ON "payments" ("created_at");
-- Create index "payment_mpesa_phone_number" to table: "payments"
CREATE INDEX "payment_mpesa_phone_number" ON "payments" ("mpesa_phone_number");
-- Create index "payment_mpesa_transaction_id" to table: "payments"
CREATE INDEX "payment_mpesa_transaction_id" ON "payments" ("mpesa_transaction_id");
-- Create index "payment_payment_intent_id" to table: "payments"
CREATE INDEX "payment_payment_intent_id" ON "payments" ("payment_intent_id");
-- Create index "payment_processed_at" to table: "payments"
CREATE INDEX "payment_processed_at" ON "payments" ("processed_at");
-- Create index "payment_provider_provider_reference" to table: "payments"
CREATE INDEX "payment_provider_provider_reference" ON "payments" ("provider", "provider_reference");
-- Create index "payment_status" to table: "payments"
CREATE INDEX "payment_status" ON "payments" ("status");
-- Create index "payment_tenant_id_order_id" to table: "payments"
CREATE INDEX "payment_tenant_id_order_id" ON "payments" ("tenant_id", "order_id");
-- Create index "payment_tenant_id_provider" to table: "payments"
CREATE INDEX "payment_tenant_id_provider" ON "payments" ("tenant_id", "provider");
-- Create index "payment_tenant_id_status" to table: "payments"
CREATE INDEX "payment_tenant_id_status" ON "payments" ("tenant_id", "status");
-- Create index "payment_tenant_id_status_created_at" to table: "payments"
CREATE INDEX "payment_tenant_id_status_created_at" ON "payments" ("tenant_id", "status", "created_at");
-- Create "promo_codes" table
CREATE TABLE "promo_codes" ("id" uuid NOT NULL, "tenant_id" uuid NOT NULL, "cafe_id" uuid NULL, "code" character varying NOT NULL, "name" character varying NOT NULL, "description" text NULL, "discount_type" character varying NOT NULL, "discount_value" double precision NOT NULL, "max_discount_amount" double precision NULL, "min_subtotal" double precision NOT NULL DEFAULT 0, "max_uses" bigint NULL, "max_uses_per_user" bigint NULL, "usage_count" bigint NOT NULL DEFAULT 0, "is_active" boolean NOT NULL DEFAULT true, "first_order_only" boolean NOT NULL DEFAULT false, "starts_at" timestamptz NULL, "ends_at" timestamptz NULL, "eligible_categories" jsonb NULL, "eligible_items" jsonb NULL, "metadata" jsonb NULL, "created_at" timestamptz NOT NULL, "updated_at" timestamptz NOT NULL, PRIMARY KEY ("id"));
-- Create index "promocode_is_active" to table: "promo_codes"
CREATE INDEX "promocode_is_active" ON "promo_codes" ("is_active");
-- Create index "promocode_starts_at_ends_at" to table: "promo_codes"
CREATE INDEX "promocode_starts_at_ends_at" ON "promo_codes" ("starts_at", "ends_at");
-- Create index "promocode_tenant_id_code" to table: "promo_codes"
CREATE UNIQUE INDEX "promocode_tenant_id_code" ON "promo_codes" ("tenant_id", "code");
-- Create "promo_redemptions" table
CREATE TABLE "promo_redemptions" ("id" uuid NOT NULL, "order_id" uuid NOT NULL, "user_id" uuid NOT NULL, "discount_amount" double precision NOT NULL, "redeemed_at" timestamptz NOT NULL, "promo_code_id" uuid NOT NULL, PRIMARY KEY ("id"), CONSTRAINT "promo_redemptions_promo_codes_redemptions" FOREIGN KEY ("promo_code_id") REFERENCES "promo_codes" ("id") ON UPDATE NO ACTION ON DELETE NO ACTION);
-- Create index "promoredemption_order_id" to table: "promo_redemptions"
CREATE UNIQUE INDEX "promoredemption_order_id" ON "promo_redemptions" ("order_id");
-- Create index "promoredemption_promo_code_id" to table: "promo_redemptions"
CREATE INDEX "promoredemption_promo_code_id" ON "promo_redemptions" ("promo_code_id");
-- Create index "promoredemption_promo_code_id_user_id" to table: "promo_redemptions"
CREATE INDEX "promoredemption_promo_code_id_user_id" ON "promo_redemptions" ("promo_code_id", "user_id");
-- Create index "promoredemption_user_id" to table: "promo_redemptions"
CREATE INDEX "promoredemption_user_id" ON "promo_redemptions" ("user_id");
-- Create "proof_of_delivery" table
CREATE TABLE "proof_of_delivery" ("id" uuid NOT NULL, "tenant_id" uuid NOT NULL, "order_id" uuid NOT NULL, "logistics_task_id" character varying NOT NULL, "type" character varying NOT NULL, "signature_url" character varying NULL, "photo_urls" jsonb NULL, "otp_verified" boolean NOT NULL DEFAULT false, "otp_code" character varying NULL, "recipient_name" character varying NULL, "recipient_relation" character varying NULL, "delivery_latitude" double precision NULL, "delivery_longitude" double precision NULL, "rider_notes" character varying NULL, "customer_rating" character varying NULL, "customer_feedback" character varying NULL, "is_verified" boolean NOT NULL DEFAULT false, "verified_by" character varying NULL, "metadata" jsonb NULL, "delivered_at" timestamptz NOT NULL, "verified_at" timestamptz NULL, "created_at" timestamptz NOT NULL, "updated_at" timestamptz NOT NULL, "assignment_id" uuid NOT NULL, PRIMARY KEY ("id"), CONSTRAINT "proof_of_delivery_order_assignments_proof_of_delivery" FOREIGN KEY ("assignment_id") REFERENCES "order_assignments" ("id") ON UPDATE NO ACTION ON DELETE NO ACTION);
-- Create index "proof_of_delivery_assignment_id_key" to table: "proof_of_delivery"
CREATE UNIQUE INDEX "proof_of_delivery_assignment_id_key" ON "proof_of_delivery" ("assignment_id");
-- Create index "proofofdelivery_assignment_id" to table: "proof_of_delivery"
CREATE UNIQUE INDEX "proofofdelivery_assignment_id" ON "proof_of_delivery" ("assignment_id");
-- Create index "proofofdelivery_delivered_at" to table: "proof_of_delivery"
CREATE INDEX "proofofdelivery_delivered_at" ON "proof_of_delivery" ("delivered_at");
-- Create index "proofofdelivery_is_verified" to table: "proof_of_delivery"
CREATE INDEX "proofofdelivery_is_verified" ON "proof_of_delivery" ("is_verified");
-- Create index "proofofdelivery_logistics_task_id" to table: "proof_of_delivery"
CREATE INDEX "proofofdelivery_logistics_task_id" ON "proof_of_delivery" ("logistics_task_id");
-- Create index "proofofdelivery_tenant_id_order_id" to table: "proof_of_delivery"
CREATE INDEX "proofofdelivery_tenant_id_order_id" ON "proof_of_delivery" ("tenant_id", "order_id");
-- Create "refunds" table
CREATE TABLE "refunds" ("id" uuid NOT NULL, "tenant_id" uuid NOT NULL, "order_id" uuid NOT NULL, "amount" double precision NOT NULL, "currency" character varying NOT NULL DEFAULT 'KES', "status" character varying NOT NULL DEFAULT 'pending', "reason" character varying NOT NULL, "reason_notes" text NULL, "provider" character varying NOT NULL, "provider_refund_id" character varying NULL, "provider_reference" character varying NULL, "requested_by" uuid NULL, "approved_by" uuid NULL, "error_message" text NULL, "error_code" character varying NULL, "provider_response" jsonb NULL, "metadata" jsonb NULL, "requested_at" timestamptz NOT NULL, "approved_at" timestamptz NULL, "processed_at" timestamptz NULL, "created_at" timestamptz NOT NULL, "updated_at" timestamptz NOT NULL, "payment_id" uuid NOT NULL, PRIMARY KEY ("id"), CONSTRAINT "refunds_payments_refunds" FOREIGN KEY ("payment_id") REFERENCES "payments" ("id") ON UPDATE NO ACTION ON DELETE NO ACTION);
-- Create index "refund_created_at" to table: "refunds"
CREATE INDEX "refund_created_at" ON "refunds" ("created_at");
-- Create index "refund_provider_provider_refund_id" to table: "refunds"
CREATE INDEX "refund_provider_provider_refund_id" ON "refunds" ("provider", "provider_refund_id");
-- Create index "refund_status" to table: "refunds"
CREATE INDEX "refund_status" ON "refunds" ("status");
-- Create index "refund_tenant_id_order_id" to table: "refunds"
CREATE INDEX "refund_tenant_id_order_id" ON "refunds" ("tenant_id", "order_id");
-- Create index "refund_tenant_id_payment_id" to table: "refunds"
CREATE INDEX "refund_tenant_id_payment_id" ON "refunds" ("tenant_id", "payment_id");
-- Create "permissions" table
CREATE TABLE "permissions" ("id" uuid NOT NULL, "name" character varying NOT NULL, "module" character varying NOT NULL, "description" character varying NULL, "created_at" timestamptz NOT NULL, "updated_at" timestamptz NOT NULL, PRIMARY KEY ("id"));
-- Create "roles" table
CREATE TABLE "roles" ("id" character varying NOT NULL, "name" character varying NOT NULL, "description" character varying NULL, "scope" character varying NOT NULL DEFAULT 'tenant', "system_role" boolean NOT NULL DEFAULT true, "created_at" timestamptz NOT NULL, "updated_at" timestamptz NOT NULL, PRIMARY KEY ("id"));
-- Create "role_permissions" table
CREATE TABLE "role_permissions" ("role_id" character varying NOT NULL, "permission_id" uuid NOT NULL, PRIMARY KEY ("role_id", "permission_id"), CONSTRAINT "role_permissions_permission_id" FOREIGN KEY ("permission_id") REFERENCES "permissions" ("id") ON UPDATE NO ACTION ON DELETE CASCADE, CONSTRAINT "role_permissions_role_id" FOREIGN KEY ("role_id") REFERENCES "roles" ("id") ON UPDATE NO ACTION ON DELETE CASCADE);
-- Create "sessions" table
CREATE TABLE "sessions" ("id" uuid NOT NULL, "refresh_token_hash" character varying NOT NULL, "user_agent" character varying NULL, "ip_address" character varying NULL, "device_id" uuid NULL, "expires_at" timestamptz NOT NULL, "revoked_at" timestamptz NULL, "created_at" timestamptz NOT NULL, "updated_at" timestamptz NOT NULL, "tenant_id" uuid NOT NULL, "user_id" uuid NOT NULL, PRIMARY KEY ("id"), CONSTRAINT "sessions_tenants_sessions" FOREIGN KEY ("tenant_id") REFERENCES "tenants" ("id") ON UPDATE NO ACTION ON DELETE NO ACTION, CONSTRAINT "sessions_users_sessions" FOREIGN KEY ("user_id") REFERENCES "users" ("id") ON UPDATE NO ACTION ON DELETE NO ACTION);
-- Create index "sessions_refresh_token_hash_key" to table: "sessions"
CREATE UNIQUE INDEX "sessions_refresh_token_hash_key" ON "sessions" ("refresh_token_hash");
-- Create "tenant_settings" table
CREATE TABLE "tenant_settings" ("id" bigint NOT NULL GENERATED BY DEFAULT AS IDENTITY, "brand_palette" jsonb NOT NULL, "locales" jsonb NOT NULL, "features" jsonb NOT NULL, "created_at" timestamptz NOT NULL, "updated_at" timestamptz NOT NULL, "tenant_settings" uuid NOT NULL, PRIMARY KEY ("id"), CONSTRAINT "tenant_settings_tenants_settings" FOREIGN KEY ("tenant_settings") REFERENCES "tenants" ("id") ON UPDATE NO ACTION ON DELETE NO ACTION);
-- Create index "tenant_settings_tenant_settings_key" to table: "tenant_settings"
CREATE UNIQUE INDEX "tenant_settings_tenant_settings_key" ON "tenant_settings" ("tenant_settings");
-- Create "tenant_sync_events" table
CREATE TABLE "tenant_sync_events" ("id" uuid NOT NULL, "tenant_slug" character varying NOT NULL, "destination_service" character varying NOT NULL, "payload" jsonb NOT NULL, "status" character varying NOT NULL DEFAULT 'pending', "attempts" bigint NOT NULL DEFAULT 0, "synced_at" timestamptz NULL, "created_at" timestamptz NOT NULL, "updated_at" timestamptz NOT NULL, "tenant_id" uuid NOT NULL, PRIMARY KEY ("id"), CONSTRAINT "tenant_sync_events_tenants_sync_events" FOREIGN KEY ("tenant_id") REFERENCES "tenants" ("id") ON UPDATE NO ACTION ON DELETE NO ACTION);
-- Create index "tenantsyncevent_tenant_id_destination_service" to table: "tenant_sync_events"
CREATE UNIQUE INDEX "tenantsyncevent_tenant_id_destination_service" ON "tenant_sync_events" ("tenant_id", "destination_service");
-- Create "devices" table
CREATE TABLE "devices" ("id" uuid NOT NULL, "platform" character varying NULL, "device_name" character varying NULL, "push_token" character varying NULL, "last_seen_at" timestamptz NULL, "created_at" timestamptz NOT NULL, PRIMARY KEY ("id"));
-- Create "user_devices" table
CREATE TABLE "user_devices" ("user_id" uuid NOT NULL, "device_id" uuid NOT NULL, PRIMARY KEY ("user_id", "device_id"), CONSTRAINT "user_devices_device_id" FOREIGN KEY ("device_id") REFERENCES "devices" ("id") ON UPDATE NO ACTION ON DELETE CASCADE, CONSTRAINT "user_devices_user_id" FOREIGN KEY ("user_id") REFERENCES "users" ("id") ON UPDATE NO ACTION ON DELETE CASCADE);
-- Create "user_preferences" table
CREATE TABLE "user_preferences" ("id" bigint NOT NULL GENERATED BY DEFAULT AS IDENTITY, "theme" character varying NOT NULL DEFAULT 'system', "language" character varying NOT NULL DEFAULT 'en', "notify_email" boolean NOT NULL DEFAULT true, "notify_sms" boolean NOT NULL DEFAULT false, "notify_push" boolean NOT NULL DEFAULT true, "timezone" character varying NOT NULL DEFAULT 'Africa/Nairobi', "created_at" timestamptz NOT NULL, "updated_at" timestamptz NOT NULL, "user_preferences" uuid NULL, PRIMARY KEY ("id"), CONSTRAINT "user_preferences_users_preferences" FOREIGN KEY ("user_preferences") REFERENCES "users" ("id") ON UPDATE NO ACTION ON DELETE SET NULL);
-- Create index "user_preferences_user_preferences_key" to table: "user_preferences"
CREATE UNIQUE INDEX "user_preferences_user_preferences_key" ON "user_preferences" ("user_preferences");
-- Create "user_profiles" table
CREATE TABLE "user_profiles" ("id" bigint NOT NULL GENERATED BY DEFAULT AS IDENTITY, "avatar_url" character varying NULL, "bio" character varying NULL, "preferences_json" jsonb NOT NULL, "created_at" timestamptz NOT NULL, "updated_at" timestamptz NOT NULL, "user_profile" uuid NULL, PRIMARY KEY ("id"), CONSTRAINT "user_profiles_users_profile" FOREIGN KEY ("user_profile") REFERENCES "users" ("id") ON UPDATE NO ACTION ON DELETE SET NULL);
-- Create index "user_profiles_user_profile_key" to table: "user_profiles"
CREATE UNIQUE INDEX "user_profiles_user_profile_key" ON "user_profiles" ("user_profile");
-- Create "user_roles" table
CREATE TABLE "user_roles" ("user_id" uuid NOT NULL, "role_id" character varying NOT NULL, PRIMARY KEY ("user_id", "role_id"), CONSTRAINT "user_roles_role_id" FOREIGN KEY ("role_id") REFERENCES "roles" ("id") ON UPDATE NO ACTION ON DELETE CASCADE, CONSTRAINT "user_roles_user_id" FOREIGN KEY ("user_id") REFERENCES "users" ("id") ON UPDATE NO ACTION ON DELETE CASCADE);
