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
