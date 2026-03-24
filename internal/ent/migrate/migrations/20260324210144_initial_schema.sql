-- Create "service_configs" table
CREATE TABLE "service_configs" ("id" uuid NOT NULL, "tenant_id" uuid NULL, "config_key" character varying NOT NULL, "config_value" text NOT NULL, "config_type" character varying NOT NULL DEFAULT 'string', "description" character varying NULL, "is_secret" boolean NOT NULL DEFAULT false, "created_at" timestamptz NOT NULL, "updated_at" timestamptz NOT NULL, PRIMARY KEY ("id"));
-- Create index "serviceconfig_config_key" to table: "service_configs"
CREATE INDEX "serviceconfig_config_key" ON "service_configs" ("config_key");
-- Create index "serviceconfig_tenant_id_config_key" to table: "service_configs"
CREATE UNIQUE INDEX "serviceconfig_tenant_id_config_key" ON "service_configs" ("tenant_id", "config_key");
-- Create "delivery_zones" table
CREATE TABLE "delivery_zones" ("id" uuid NOT NULL, "tenant_id" uuid NOT NULL, "outlet_id" uuid NULL, "name" character varying NOT NULL, "slug" character varying NULL, "zone_polygon" jsonb NULL, "delivery_fee" double precision NOT NULL DEFAULT 0, "minimum_order" double precision NOT NULL DEFAULT 0, "estimated_time_minutes" bigint NOT NULL DEFAULT 30, "is_active" boolean NOT NULL DEFAULT true, "sort_order" bigint NOT NULL DEFAULT 0, "metadata" jsonb NOT NULL, "created_at" timestamptz NOT NULL, "updated_at" timestamptz NOT NULL, PRIMARY KEY ("id"));
-- Create index "deliveryzone_tenant_id" to table: "delivery_zones"
CREATE INDEX "deliveryzone_tenant_id" ON "delivery_zones" ("tenant_id");
-- Create index "deliveryzone_tenant_id_is_active" to table: "delivery_zones"
CREATE INDEX "deliveryzone_tenant_id_is_active" ON "delivery_zones" ("tenant_id", "is_active");
-- Create index "deliveryzone_tenant_id_outlet_id" to table: "delivery_zones"
CREATE INDEX "deliveryzone_tenant_id_outlet_id" ON "delivery_zones" ("tenant_id", "outlet_id");
-- Create index "deliveryzone_tenant_id_slug" to table: "delivery_zones"
CREATE UNIQUE INDEX "deliveryzone_tenant_id_slug" ON "delivery_zones" ("tenant_id", "slug");
-- Create "outlet_ratings" table
CREATE TABLE "outlet_ratings" ("id" uuid NOT NULL, "tenant_id" uuid NOT NULL, "outlet_id" uuid NOT NULL, "average_rating" double precision NOT NULL DEFAULT 0, "total_ratings" bigint NOT NULL DEFAULT 0, "total_reviews" bigint NOT NULL DEFAULT 0, "five_star" bigint NOT NULL DEFAULT 0, "four_star" bigint NOT NULL DEFAULT 0, "three_star" bigint NOT NULL DEFAULT 0, "two_star" bigint NOT NULL DEFAULT 0, "one_star" bigint NOT NULL DEFAULT 0, "created_at" timestamptz NOT NULL, "updated_at" timestamptz NOT NULL, PRIMARY KEY ("id"));
-- Create index "outlet_ratings_outlet_id_key" to table: "outlet_ratings"
CREATE UNIQUE INDEX "outlet_ratings_outlet_id_key" ON "outlet_ratings" ("outlet_id");
-- Create index "outletrating_average_rating" to table: "outlet_ratings"
CREATE INDEX "outletrating_average_rating" ON "outlet_ratings" ("average_rating");
-- Create index "outletrating_tenant_id_outlet_id" to table: "outlet_ratings"
CREATE UNIQUE INDEX "outletrating_tenant_id_outlet_id" ON "outlet_ratings" ("tenant_id", "outlet_id");
-- Create "catalog_overrides" table
CREATE TABLE "catalog_overrides" ("id" uuid NOT NULL, "tenant_id" uuid NOT NULL, "outlet_id" uuid NOT NULL, "inventory_sku" character varying NOT NULL, "base_price" double precision NOT NULL DEFAULT 0, "currency" character varying NOT NULL DEFAULT 'KES', "is_available" boolean NOT NULL DEFAULT true, "is_featured" boolean NOT NULL DEFAULT false, "lead_time_minutes" bigint NULL, "display_order" bigint NOT NULL DEFAULT 0, "display_section" character varying NOT NULL DEFAULT 'default', "packaging_fee" double precision NOT NULL DEFAULT 0, "service_fee_percent" double precision NOT NULL DEFAULT 0, "requires_age_verification" boolean NOT NULL DEFAULT false, "item_type" character varying NULL, "variant_options" jsonb NULL, "image_url_override" character varying NULL, "created_at" timestamptz NOT NULL, "updated_at" timestamptz NOT NULL, PRIMARY KEY ("id"));
-- Create index "catalogoverride_inventory_sku" to table: "catalog_overrides"
CREATE INDEX "catalogoverride_inventory_sku" ON "catalog_overrides" ("inventory_sku");
-- Create index "catalogoverride_tenant_id_display_section" to table: "catalog_overrides"
CREATE INDEX "catalogoverride_tenant_id_display_section" ON "catalog_overrides" ("tenant_id", "display_section");
-- Create index "catalogoverride_tenant_id_is_available" to table: "catalog_overrides"
CREATE INDEX "catalogoverride_tenant_id_is_available" ON "catalog_overrides" ("tenant_id", "is_available");
-- Create index "catalogoverride_tenant_id_outlet_id" to table: "catalog_overrides"
CREATE INDEX "catalogoverride_tenant_id_outlet_id" ON "catalog_overrides" ("tenant_id", "outlet_id");
-- Create index "catalogoverride_tenant_id_outlet_id_inventory_sku" to table: "catalog_overrides"
CREATE UNIQUE INDEX "catalogoverride_tenant_id_outlet_id_inventory_sku" ON "catalog_overrides" ("tenant_id", "outlet_id", "inventory_sku");
-- Create "outbox_events" table
CREATE TABLE "outbox_events" ("id" uuid NOT NULL, "tenant_id" uuid NOT NULL, "aggregate_type" character varying NOT NULL, "aggregate_id" uuid NOT NULL, "event_type" character varying NOT NULL, "payload" bytea NOT NULL, "status" character varying NOT NULL DEFAULT 'PENDING', "attempts" bigint NOT NULL DEFAULT 0, "last_attempt_at" timestamptz NULL, "published_at" timestamptz NULL, "error_message" character varying NULL, "created_at" timestamptz NOT NULL, PRIMARY KEY ("id"));
-- Create index "outboxevent_aggregate_type_aggregate_id" to table: "outbox_events"
CREATE INDEX "outboxevent_aggregate_type_aggregate_id" ON "outbox_events" ("aggregate_type", "aggregate_id");
-- Create index "outboxevent_status_created_at" to table: "outbox_events"
CREATE INDEX "outboxevent_status_created_at" ON "outbox_events" ("status", "created_at");
-- Create index "outboxevent_tenant_id_status" to table: "outbox_events"
CREATE INDEX "outboxevent_tenant_id_status" ON "outbox_events" ("tenant_id", "status");
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
-- Create "user_favorites" table
CREATE TABLE "user_favorites" ("id" uuid NOT NULL, "tenant_id" uuid NOT NULL, "user_id" uuid NOT NULL, "inventory_sku" character varying NOT NULL, "created_at" timestamptz NOT NULL, PRIMARY KEY ("id"));
-- Create index "userfavorite_inventory_sku" to table: "user_favorites"
CREATE INDEX "userfavorite_inventory_sku" ON "user_favorites" ("inventory_sku");
-- Create index "userfavorite_tenant_id_user_id" to table: "user_favorites"
CREATE INDEX "userfavorite_tenant_id_user_id" ON "user_favorites" ("tenant_id", "user_id");
-- Create index "userfavorite_tenant_id_user_id_inventory_sku" to table: "user_favorites"
CREATE UNIQUE INDEX "userfavorite_tenant_id_user_id_inventory_sku" ON "user_favorites" ("tenant_id", "user_id", "inventory_sku");
-- Create "rate_limit_configs" table
CREATE TABLE "rate_limit_configs" ("id" uuid NOT NULL, "service_name" character varying NOT NULL, "key_type" character varying NOT NULL, "endpoint_pattern" character varying NOT NULL DEFAULT '*', "requests_per_window" bigint NOT NULL DEFAULT 60, "window_seconds" bigint NOT NULL DEFAULT 60, "burst_multiplier" double precision NOT NULL DEFAULT 1.5, "is_active" boolean NOT NULL DEFAULT true, "description" character varying NULL, "created_at" timestamptz NOT NULL, "updated_at" timestamptz NOT NULL, PRIMARY KEY ("id"));
-- Create index "ratelimitconfig_is_active" to table: "rate_limit_configs"
CREATE INDEX "ratelimitconfig_is_active" ON "rate_limit_configs" ("is_active");
-- Create index "ratelimitconfig_service_name" to table: "rate_limit_configs"
CREATE INDEX "ratelimitconfig_service_name" ON "rate_limit_configs" ("service_name");
-- Create index "ratelimitconfig_service_name_key_type_endpoint_pattern" to table: "rate_limit_configs"
CREATE UNIQUE INDEX "ratelimitconfig_service_name_key_type_endpoint_pattern" ON "rate_limit_configs" ("service_name", "key_type", "endpoint_pattern");
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
-- Create "tenants" table
CREATE TABLE "tenants" ("id" uuid NOT NULL, "name" character varying NOT NULL, "slug" character varying NOT NULL, "status" character varying NOT NULL DEFAULT 'active', "use_case" character varying NULL, "sync_status" character varying NOT NULL DEFAULT 'synced', "last_sync_at" timestamptz NULL, "created_at" timestamptz NOT NULL, "updated_at" timestamptz NOT NULL, PRIMARY KEY ("id"));
-- Create index "tenant_slug" to table: "tenants"
CREATE UNIQUE INDEX "tenant_slug" ON "tenants" ("slug");
-- Create index "tenant_status" to table: "tenants"
CREATE INDEX "tenant_status" ON "tenants" ("status");
-- Create index "tenants_slug_key" to table: "tenants"
CREATE UNIQUE INDEX "tenants_slug_key" ON "tenants" ("slug");
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
-- Create "users" table
CREATE TABLE "users" ("id" uuid NOT NULL, "auth_service_user_id" uuid NULL, "email" character varying NOT NULL, "password_hash" character varying NULL, "sync_status" character varying NOT NULL DEFAULT 'pending', "sync_at" timestamptz NULL, "full_name" character varying NOT NULL, "phone" character varying NULL, "status" character varying NOT NULL DEFAULT 'active', "primary_role" character varying NULL, "locale" character varying NOT NULL DEFAULT 'en', "timezone" character varying NOT NULL DEFAULT 'Africa/Nairobi', "last_login_at" timestamptz NULL, "metadata" jsonb NOT NULL, "created_at" timestamptz NOT NULL, "updated_at" timestamptz NOT NULL, "tenant_id" uuid NOT NULL, PRIMARY KEY ("id"), CONSTRAINT "users_tenants_users" FOREIGN KEY ("tenant_id") REFERENCES "tenants" ("id") ON UPDATE NO ACTION ON DELETE NO ACTION);
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
-- Create "carts" table
CREATE TABLE "carts" ("id" uuid NOT NULL, "tenant_id" uuid NOT NULL, "outlet_id" uuid NOT NULL, "session_id" character varying NULL, "status" character varying NOT NULL DEFAULT 'active', "currency" character varying NOT NULL DEFAULT 'KES', "subtotal" double precision NOT NULL DEFAULT 0, "discount_total" double precision NOT NULL DEFAULT 0, "tax_total" double precision NOT NULL DEFAULT 0, "delivery_fee" double precision NOT NULL DEFAULT 0, "loyalty_points_redeemed" bigint NOT NULL DEFAULT 0, "promo_code_id" uuid NULL, "expires_at" timestamptz NULL, "created_at" timestamptz NOT NULL, "updated_at" timestamptz NOT NULL, "user_id" uuid NULL, PRIMARY KEY ("id"), CONSTRAINT "carts_users_carts" FOREIGN KEY ("user_id") REFERENCES "users" ("id") ON UPDATE NO ACTION ON DELETE SET NULL);
-- Create index "cart_expires_at" to table: "carts"
CREATE INDEX "cart_expires_at" ON "carts" ("expires_at");
-- Create index "cart_status" to table: "carts"
CREATE INDEX "cart_status" ON "carts" ("status");
-- Create index "cart_tenant_id_session_id_status" to table: "carts"
CREATE UNIQUE INDEX "cart_tenant_id_session_id_status" ON "carts" ("tenant_id", "session_id", "status") WHERE (((status)::text = 'active'::text) AND (session_id IS NOT NULL));
-- Create index "cart_tenant_id_user_id_status" to table: "carts"
CREATE UNIQUE INDEX "cart_tenant_id_user_id_status" ON "carts" ("tenant_id", "user_id", "status") WHERE (((status)::text = 'active'::text) AND (user_id IS NOT NULL));
-- Create "cart_items" table
CREATE TABLE "cart_items" ("id" uuid NOT NULL, "inventory_sku" character varying NOT NULL, "variant_id" uuid NULL, "name_snapshot" character varying NOT NULL, "variant_name_snapshot" character varying NULL, "quantity" bigint NOT NULL DEFAULT 1, "unit_price" double precision NOT NULL, "total_price" double precision NOT NULL, "notes" text NULL, "modifiers" jsonb NULL, "metadata" jsonb NULL, "created_at" timestamptz NOT NULL, "updated_at" timestamptz NOT NULL, "cart_id" uuid NOT NULL, PRIMARY KEY ("id"), CONSTRAINT "cart_items_carts_items" FOREIGN KEY ("cart_id") REFERENCES "carts" ("id") ON UPDATE NO ACTION ON DELETE NO ACTION);
-- Create index "cartitem_cart_id" to table: "cart_items"
CREATE INDEX "cartitem_cart_id" ON "cart_items" ("cart_id");
-- Create index "cartitem_inventory_sku" to table: "cart_items"
CREATE INDEX "cartitem_inventory_sku" ON "cart_items" ("inventory_sku");
-- Create "customer_addresses" table
CREATE TABLE "customer_addresses" ("id" uuid NOT NULL, "tenant_id" uuid NOT NULL, "label" character varying NOT NULL, "address_line1" character varying NOT NULL, "address_line2" character varying NULL, "city" character varying NOT NULL, "county" character varying NULL, "postal_code" character varying NULL, "country" character varying NOT NULL DEFAULT 'KE', "latitude" double precision NULL, "longitude" double precision NULL, "plus_code" character varying NULL, "instructions" text NULL, "contact_name" character varying NULL, "contact_phone" character varying NULL, "is_default" boolean NOT NULL DEFAULT false, "is_verified" boolean NOT NULL DEFAULT false, "created_at" timestamptz NOT NULL, "updated_at" timestamptz NOT NULL, "user_id" uuid NOT NULL, PRIMARY KEY ("id"), CONSTRAINT "customer_addresses_users_addresses" FOREIGN KEY ("user_id") REFERENCES "users" ("id") ON UPDATE NO ACTION ON DELETE NO ACTION);
-- Create index "customeraddress_latitude_longitude" to table: "customer_addresses"
CREATE INDEX "customeraddress_latitude_longitude" ON "customer_addresses" ("latitude", "longitude");
-- Create index "customeraddress_tenant_id_user_id" to table: "customer_addresses"
CREATE INDEX "customeraddress_tenant_id_user_id" ON "customer_addresses" ("tenant_id", "user_id");
-- Create index "customeraddress_user_id_is_default" to table: "customer_addresses"
CREATE INDEX "customeraddress_user_id_is_default" ON "customer_addresses" ("user_id", "is_default");
-- Create "outlets" table
CREATE TABLE "outlets" ("id" uuid NOT NULL, "name" character varying NOT NULL, "slug" character varying NOT NULL, "description" text NULL, "address" character varying NULL, "phone" character varying NULL, "email" character varying NULL, "location" character varying NULL, "latitude" double precision NULL, "longitude" double precision NULL, "opening_hours" jsonb NULL, "image_url" character varying NULL DEFAULT '', "use_case" character varying NULL, "supports_pickup" boolean NOT NULL DEFAULT false, "status" character varying NOT NULL DEFAULT 'active', "created_at" timestamptz NOT NULL, "updated_at" timestamptz NOT NULL, "tenant_id" uuid NOT NULL, PRIMARY KEY ("id"), CONSTRAINT "outlets_tenants_outlets" FOREIGN KEY ("tenant_id") REFERENCES "tenants" ("id") ON UPDATE NO ACTION ON DELETE NO ACTION);
-- Create index "outlet_status" to table: "outlets"
CREATE INDEX "outlet_status" ON "outlets" ("status");
-- Create index "outlet_tenant_id_slug" to table: "outlets"
CREATE UNIQUE INDEX "outlet_tenant_id_slug" ON "outlets" ("tenant_id", "slug");
-- Create "orders" table
CREATE TABLE "orders" ("id" uuid NOT NULL, "tenant_id" uuid NOT NULL, "cart_id" uuid NULL, "order_number" character varying NOT NULL, "status" character varying NOT NULL DEFAULT 'pending', "payment_status" character varying NOT NULL DEFAULT 'pending', "payment_intent_id" uuid NULL, "currency" character varying NOT NULL DEFAULT 'KES', "subtotal" double precision NOT NULL, "discount_total" double precision NOT NULL DEFAULT 0, "tax_total" double precision NOT NULL DEFAULT 0, "delivery_fee" double precision NOT NULL DEFAULT 0, "fulfillment_type" character varying NOT NULL DEFAULT 'delivery', "scheduled_for" timestamptz NULL, "packaging_fee" double precision NOT NULL DEFAULT 0, "service_fee" double precision NOT NULL DEFAULT 0, "small_order_fee" double precision NOT NULL DEFAULT 0, "reservation_id" uuid NULL, "appointment_id" uuid NULL, "staff_preference_id" uuid NULL, "preferred_carrier" character varying NULL, "tip_total" double precision NOT NULL DEFAULT 0, "grand_total" double precision NOT NULL, "loyalty_points_earned" bigint NOT NULL DEFAULT 0, "loyalty_points_redeemed" bigint NOT NULL DEFAULT 0, "promo_code_id" uuid NULL, "instructions" text NULL, "channel" character varying NOT NULL DEFAULT 'web', "source" character varying NULL, "idempotency_key" character varying NULL, "placed_at" timestamptz NULL, "confirmed_at" timestamptz NULL, "ready_at" timestamptz NULL, "delivered_at" timestamptz NULL, "completed_at" timestamptz NULL, "cancelled_at" timestamptz NULL, "cancellation_reason" text NULL, "rating" bigint NULL, "rating_comment" text NULL, "rated_at" timestamptz NULL, "metadata" jsonb NULL, "created_at" timestamptz NOT NULL, "updated_at" timestamptz NOT NULL, "delivery_address_id" uuid NULL, "outlet_id" uuid NOT NULL, "customer_id" uuid NOT NULL, PRIMARY KEY ("id"), CONSTRAINT "orders_customer_addresses_orders" FOREIGN KEY ("delivery_address_id") REFERENCES "customer_addresses" ("id") ON UPDATE NO ACTION ON DELETE SET NULL, CONSTRAINT "orders_outlets_orders" FOREIGN KEY ("outlet_id") REFERENCES "outlets" ("id") ON UPDATE NO ACTION ON DELETE NO ACTION, CONSTRAINT "orders_users_orders" FOREIGN KEY ("customer_id") REFERENCES "users" ("id") ON UPDATE NO ACTION ON DELETE NO ACTION);
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
-- Create index "order_fulfillment_type" to table: "orders"
CREATE INDEX "order_fulfillment_type" ON "orders" ("fulfillment_type");
-- Create index "order_idempotency_key" to table: "orders"
CREATE UNIQUE INDEX "order_idempotency_key" ON "orders" ("idempotency_key") WHERE (idempotency_key IS NOT NULL);
-- Create index "order_order_number" to table: "orders"
CREATE UNIQUE INDEX "order_order_number" ON "orders" ("order_number");
-- Create index "order_placed_at" to table: "orders"
CREATE INDEX "order_placed_at" ON "orders" ("placed_at");
-- Create index "order_scheduled_for" to table: "orders"
CREATE INDEX "order_scheduled_for" ON "orders" ("scheduled_for");
-- Create index "order_tenant_id_customer_id" to table: "orders"
CREATE INDEX "order_tenant_id_customer_id" ON "orders" ("tenant_id", "customer_id");
-- Create index "order_tenant_id_customer_id_status" to table: "orders"
CREATE INDEX "order_tenant_id_customer_id_status" ON "orders" ("tenant_id", "customer_id", "status");
-- Create index "order_tenant_id_outlet_id" to table: "orders"
CREATE INDEX "order_tenant_id_outlet_id" ON "orders" ("tenant_id", "outlet_id");
-- Create index "order_tenant_id_outlet_id_status" to table: "orders"
CREATE INDEX "order_tenant_id_outlet_id_status" ON "orders" ("tenant_id", "outlet_id", "status");
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
-- Create "group_orders" table
CREATE TABLE "group_orders" ("id" uuid NOT NULL, "tenant_id" uuid NOT NULL, "host_user_id" uuid NOT NULL, "cart_id" uuid NOT NULL, "invite_code" character varying NOT NULL, "status" character varying NOT NULL DEFAULT 'open', "max_participants" bigint NOT NULL DEFAULT 10, "expires_at" timestamptz NOT NULL, "created_at" timestamptz NOT NULL, "updated_at" timestamptz NOT NULL, PRIMARY KEY ("id"));
-- Create index "grouporder_expires_at" to table: "group_orders"
CREATE INDEX "grouporder_expires_at" ON "group_orders" ("expires_at");
-- Create index "grouporder_invite_code" to table: "group_orders"
CREATE UNIQUE INDEX "grouporder_invite_code" ON "group_orders" ("invite_code");
-- Create index "grouporder_status" to table: "group_orders"
CREATE INDEX "grouporder_status" ON "group_orders" ("status");
-- Create index "grouporder_tenant_id_host_user_id" to table: "group_orders"
CREATE INDEX "grouporder_tenant_id_host_user_id" ON "group_orders" ("tenant_id", "host_user_id");
-- Create "group_participants" table
CREATE TABLE "group_participants" ("id" uuid NOT NULL, "user_id" uuid NOT NULL, "user_name" character varying NOT NULL, "joined_at" timestamptz NOT NULL, "group_order_id" uuid NOT NULL, PRIMARY KEY ("id"), CONSTRAINT "group_participants_group_orders_participants" FOREIGN KEY ("group_order_id") REFERENCES "group_orders" ("id") ON UPDATE NO ACTION ON DELETE NO ACTION);
-- Create index "groupparticipant_group_order_id_user_id" to table: "group_participants"
CREATE UNIQUE INDEX "groupparticipant_group_order_id_user_id" ON "group_participants" ("group_order_id", "user_id");
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
-- Create "order_events" table
CREATE TABLE "order_events" ("id" uuid NOT NULL, "event_type" character varying NOT NULL, "from_status" character varying NULL, "to_status" character varying NULL, "payload" jsonb NULL, "actor_user_id" uuid NULL, "actor_type" character varying NULL, "ip_address" character varying NULL, "occurred_at" timestamptz NOT NULL, "order_id" uuid NOT NULL, PRIMARY KEY ("id"), CONSTRAINT "order_events_orders_events" FOREIGN KEY ("order_id") REFERENCES "orders" ("id") ON UPDATE NO ACTION ON DELETE NO ACTION);
-- Create index "orderevent_event_type" to table: "order_events"
CREATE INDEX "orderevent_event_type" ON "order_events" ("event_type");
-- Create index "orderevent_occurred_at" to table: "order_events"
CREATE INDEX "orderevent_occurred_at" ON "order_events" ("occurred_at");
-- Create index "orderevent_order_id" to table: "order_events"
CREATE INDEX "orderevent_order_id" ON "order_events" ("order_id");
-- Create "order_items" table
CREATE TABLE "order_items" ("id" uuid NOT NULL, "inventory_sku" character varying NOT NULL, "variant_id" uuid NULL, "name_snapshot" character varying NOT NULL, "variant_name_snapshot" character varying NULL, "quantity" bigint NOT NULL, "unit_price" double precision NOT NULL, "total_price" double precision NOT NULL, "notes" text NULL, "modifiers" jsonb NULL, "item_type" character varying NULL, "service_start_time" timestamptz NULL, "duration_minutes" bigint NULL, "metadata" jsonb NULL, "created_at" timestamptz NOT NULL, "order_id" uuid NOT NULL, PRIMARY KEY ("id"), CONSTRAINT "order_items_orders_items" FOREIGN KEY ("order_id") REFERENCES "orders" ("id") ON UPDATE NO ACTION ON DELETE NO ACTION);
-- Create index "orderitem_inventory_sku" to table: "order_items"
CREATE INDEX "orderitem_inventory_sku" ON "order_items" ("inventory_sku");
-- Create index "orderitem_order_id" to table: "order_items"
CREATE INDEX "orderitem_order_id" ON "order_items" ("order_id");
-- Create "promo_codes" table
CREATE TABLE "promo_codes" ("id" uuid NOT NULL, "tenant_id" uuid NOT NULL, "outlet_id" uuid NULL, "code" character varying NOT NULL, "name" character varying NOT NULL, "description" text NULL, "discount_type" character varying NOT NULL, "discount_value" double precision NOT NULL, "max_discount_amount" double precision NULL, "min_subtotal" double precision NOT NULL DEFAULT 0, "max_uses" bigint NULL, "max_uses_per_user" bigint NULL, "usage_count" bigint NOT NULL DEFAULT 0, "is_active" boolean NOT NULL DEFAULT true, "first_order_only" boolean NOT NULL DEFAULT false, "starts_at" timestamptz NULL, "ends_at" timestamptz NULL, "eligible_categories" jsonb NULL, "eligible_items" jsonb NULL, "metadata" jsonb NULL, "created_at" timestamptz NOT NULL, "updated_at" timestamptz NOT NULL, PRIMARY KEY ("id"));
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
-- Create "permissions" table
CREATE TABLE "permissions" ("id" uuid NOT NULL, "name" character varying NOT NULL, "module" character varying NOT NULL, "description" character varying NULL, "created_at" timestamptz NOT NULL, "updated_at" timestamptz NOT NULL, PRIMARY KEY ("id"));
-- Create "roles" table
CREATE TABLE "roles" ("id" character varying NOT NULL, "name" character varying NOT NULL, "description" character varying NULL, "scope" character varying NOT NULL DEFAULT 'tenant', "system_role" boolean NOT NULL DEFAULT true, "created_at" timestamptz NOT NULL, "updated_at" timestamptz NOT NULL, PRIMARY KEY ("id"));
-- Create "role_legacy_permissions" table
CREATE TABLE "role_legacy_permissions" ("role_id" character varying NOT NULL, "permission_id" uuid NOT NULL, PRIMARY KEY ("role_id", "permission_id"), CONSTRAINT "role_legacy_permissions_permission_id" FOREIGN KEY ("permission_id") REFERENCES "permissions" ("id") ON UPDATE NO ACTION ON DELETE CASCADE, CONSTRAINT "role_legacy_permissions_role_id" FOREIGN KEY ("role_id") REFERENCES "roles" ("id") ON UPDATE NO ACTION ON DELETE CASCADE);
-- Create "ordering_permissions" table
CREATE TABLE "ordering_permissions" ("id" uuid NOT NULL, "permission_code" character varying NOT NULL, "name" character varying NOT NULL, "module" character varying NOT NULL, "action" character varying NOT NULL, "resource" character varying NULL, "description" text NULL, "created_at" timestamptz NOT NULL, PRIMARY KEY ("id"));
-- Create index "ordering_permissions_permission_code_key" to table: "ordering_permissions"
CREATE UNIQUE INDEX "ordering_permissions_permission_code_key" ON "ordering_permissions" ("permission_code");
-- Create index "orderingpermission_action" to table: "ordering_permissions"
CREATE INDEX "orderingpermission_action" ON "ordering_permissions" ("action");
-- Create index "orderingpermission_module" to table: "ordering_permissions"
CREATE INDEX "orderingpermission_module" ON "ordering_permissions" ("module");
-- Create index "orderingpermission_module_action" to table: "ordering_permissions"
CREATE INDEX "orderingpermission_module_action" ON "ordering_permissions" ("module", "action");
-- Create index "orderingpermission_permission_code" to table: "ordering_permissions"
CREATE UNIQUE INDEX "orderingpermission_permission_code" ON "ordering_permissions" ("permission_code");
-- Create "ordering_roles" table
CREATE TABLE "ordering_roles" ("id" uuid NOT NULL, "tenant_id" uuid NOT NULL, "role_code" character varying NOT NULL, "name" character varying NOT NULL, "description" text NULL, "is_system_role" boolean NOT NULL DEFAULT false, "created_at" timestamptz NOT NULL, "updated_at" timestamptz NOT NULL, PRIMARY KEY ("id"));
-- Create index "orderingrole_is_system_role" to table: "ordering_roles"
CREATE INDEX "orderingrole_is_system_role" ON "ordering_roles" ("is_system_role");
-- Create index "orderingrole_tenant_id" to table: "ordering_roles"
CREATE INDEX "orderingrole_tenant_id" ON "ordering_roles" ("tenant_id");
-- Create index "orderingrole_tenant_id_role_code" to table: "ordering_roles"
CREATE UNIQUE INDEX "orderingrole_tenant_id_role_code" ON "ordering_roles" ("tenant_id", "role_code");
-- Create "role_permissions" table
CREATE TABLE "role_permissions" ("id" bigint NOT NULL GENERATED BY DEFAULT AS IDENTITY, "role_id" uuid NOT NULL, "permission_id" uuid NOT NULL, PRIMARY KEY ("id"), CONSTRAINT "role_permissions_ordering_permissions_permission" FOREIGN KEY ("permission_id") REFERENCES "ordering_permissions" ("id") ON UPDATE NO ACTION ON DELETE NO ACTION, CONSTRAINT "role_permissions_ordering_roles_role" FOREIGN KEY ("role_id") REFERENCES "ordering_roles" ("id") ON UPDATE NO ACTION ON DELETE NO ACTION);
-- Create index "rolepermission_permission_id" to table: "role_permissions"
CREATE INDEX "rolepermission_permission_id" ON "role_permissions" ("permission_id");
-- Create index "rolepermission_role_id" to table: "role_permissions"
CREATE INDEX "rolepermission_role_id" ON "role_permissions" ("role_id");
-- Create index "rolepermission_role_id_permission_id" to table: "role_permissions"
CREATE UNIQUE INDEX "rolepermission_role_id_permission_id" ON "role_permissions" ("role_id", "permission_id");
-- Create "tenant_settings" table
CREATE TABLE "tenant_settings" ("id" bigint NOT NULL GENERATED BY DEFAULT AS IDENTITY, "brand_palette" jsonb NOT NULL, "locales" jsonb NOT NULL, "features" jsonb NOT NULL, "created_at" timestamptz NOT NULL, "updated_at" timestamptz NOT NULL, "tenant_settings" uuid NOT NULL, PRIMARY KEY ("id"), CONSTRAINT "tenant_settings_tenants_settings" FOREIGN KEY ("tenant_settings") REFERENCES "tenants" ("id") ON UPDATE NO ACTION ON DELETE NO ACTION);
-- Create index "tenant_settings_tenant_settings_key" to table: "tenant_settings"
CREATE UNIQUE INDEX "tenant_settings_tenant_settings_key" ON "tenant_settings" ("tenant_settings");
-- Create "tenant_sync_events" table
CREATE TABLE "tenant_sync_events" ("id" uuid NOT NULL, "tenant_slug" character varying NOT NULL, "destination_service" character varying NOT NULL, "payload" jsonb NOT NULL, "status" character varying NOT NULL DEFAULT 'pending', "attempts" bigint NOT NULL DEFAULT 0, "synced_at" timestamptz NULL, "created_at" timestamptz NOT NULL, "updated_at" timestamptz NOT NULL, "tenant_id" uuid NOT NULL, PRIMARY KEY ("id"), CONSTRAINT "tenant_sync_events_tenants_sync_events" FOREIGN KEY ("tenant_id") REFERENCES "tenants" ("id") ON UPDATE NO ACTION ON DELETE NO ACTION);
-- Create index "tenantsyncevent_tenant_id_destination_service" to table: "tenant_sync_events"
CREATE UNIQUE INDEX "tenantsyncevent_tenant_id_destination_service" ON "tenant_sync_events" ("tenant_id", "destination_service");
-- Create "user_preferences" table
CREATE TABLE "user_preferences" ("id" bigint NOT NULL GENERATED BY DEFAULT AS IDENTITY, "theme" character varying NOT NULL DEFAULT 'system', "language" character varying NOT NULL DEFAULT 'en', "notify_email" boolean NOT NULL DEFAULT true, "notify_sms" boolean NOT NULL DEFAULT false, "notify_push" boolean NOT NULL DEFAULT true, "timezone" character varying NOT NULL DEFAULT 'Africa/Nairobi', "created_at" timestamptz NOT NULL, "updated_at" timestamptz NOT NULL, "user_preferences" uuid NULL, PRIMARY KEY ("id"), CONSTRAINT "user_preferences_users_preferences" FOREIGN KEY ("user_preferences") REFERENCES "users" ("id") ON UPDATE NO ACTION ON DELETE SET NULL);
-- Create index "user_preferences_user_preferences_key" to table: "user_preferences"
CREATE UNIQUE INDEX "user_preferences_user_preferences_key" ON "user_preferences" ("user_preferences");
-- Create "user_profiles" table
CREATE TABLE "user_profiles" ("id" bigint NOT NULL GENERATED BY DEFAULT AS IDENTITY, "avatar_url" character varying NULL, "bio" character varying NULL, "preferences_json" jsonb NOT NULL, "created_at" timestamptz NOT NULL, "updated_at" timestamptz NOT NULL, "user_profile" uuid NULL, PRIMARY KEY ("id"), CONSTRAINT "user_profiles_users_profile" FOREIGN KEY ("user_profile") REFERENCES "users" ("id") ON UPDATE NO ACTION ON DELETE SET NULL);
-- Create index "user_profiles_user_profile_key" to table: "user_profiles"
CREATE UNIQUE INDEX "user_profiles_user_profile_key" ON "user_profiles" ("user_profile");
-- Create "user_role_assignments" table
CREATE TABLE "user_role_assignments" ("id" uuid NOT NULL, "tenant_id" uuid NOT NULL, "assigned_by" uuid NOT NULL, "assigned_at" timestamptz NOT NULL, "expires_at" timestamptz NULL, "user_id" uuid NOT NULL, "role_id" uuid NOT NULL, PRIMARY KEY ("id"), CONSTRAINT "user_role_assignments_ordering_roles_role" FOREIGN KEY ("role_id") REFERENCES "ordering_roles" ("id") ON UPDATE NO ACTION ON DELETE NO ACTION, CONSTRAINT "user_role_assignments_users_user" FOREIGN KEY ("user_id") REFERENCES "users" ("id") ON UPDATE NO ACTION ON DELETE NO ACTION);
-- Create index "userroleassignment_expires_at" to table: "user_role_assignments"
CREATE INDEX "userroleassignment_expires_at" ON "user_role_assignments" ("expires_at");
-- Create index "userroleassignment_role_id" to table: "user_role_assignments"
CREATE INDEX "userroleassignment_role_id" ON "user_role_assignments" ("role_id");
-- Create index "userroleassignment_tenant_id" to table: "user_role_assignments"
CREATE INDEX "userroleassignment_tenant_id" ON "user_role_assignments" ("tenant_id");
-- Create index "userroleassignment_tenant_id_user_id_role_id" to table: "user_role_assignments"
CREATE UNIQUE INDEX "userroleassignment_tenant_id_user_id_role_id" ON "user_role_assignments" ("tenant_id", "user_id", "role_id");
-- Create index "userroleassignment_user_id" to table: "user_role_assignments"
CREATE INDEX "userroleassignment_user_id" ON "user_role_assignments" ("user_id");
-- Create "user_roles" table
CREATE TABLE "user_roles" ("user_id" uuid NOT NULL, "role_id" character varying NOT NULL, PRIMARY KEY ("user_id", "role_id"), CONSTRAINT "user_roles_role_id" FOREIGN KEY ("role_id") REFERENCES "roles" ("id") ON UPDATE NO ACTION ON DELETE CASCADE, CONSTRAINT "user_roles_user_id" FOREIGN KEY ("user_id") REFERENCES "users" ("id") ON UPDATE NO ACTION ON DELETE CASCADE);
