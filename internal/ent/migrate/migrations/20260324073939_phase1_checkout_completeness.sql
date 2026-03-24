-- Modify "orders" table
ALTER TABLE "orders" ADD COLUMN "fulfillment_type" character varying NOT NULL DEFAULT 'delivery', ADD COLUMN "scheduled_for" timestamptz NULL, ADD COLUMN "packaging_fee" double precision NOT NULL DEFAULT 0, ADD COLUMN "service_fee" double precision NOT NULL DEFAULT 0, ADD COLUMN "small_order_fee" double precision NOT NULL DEFAULT 0, ADD COLUMN "reservation_id" uuid NULL;
-- Create index "order_fulfillment_type" to table: "orders"
CREATE INDEX "order_fulfillment_type" ON "orders" ("fulfillment_type");
-- Create index "order_scheduled_for" to table: "orders"
CREATE INDEX "order_scheduled_for" ON "orders" ("scheduled_for");
-- Create "outlet_ratings" table
CREATE TABLE "outlet_ratings" ("id" uuid NOT NULL, "tenant_id" uuid NOT NULL, "outlet_id" uuid NOT NULL, "average_rating" double precision NOT NULL DEFAULT 0, "total_ratings" bigint NOT NULL DEFAULT 0, "total_reviews" bigint NOT NULL DEFAULT 0, "five_star" bigint NOT NULL DEFAULT 0, "four_star" bigint NOT NULL DEFAULT 0, "three_star" bigint NOT NULL DEFAULT 0, "two_star" bigint NOT NULL DEFAULT 0, "one_star" bigint NOT NULL DEFAULT 0, "created_at" timestamptz NOT NULL, "updated_at" timestamptz NOT NULL, PRIMARY KEY ("id"));
-- Create index "outlet_ratings_outlet_id_key" to table: "outlet_ratings"
CREATE UNIQUE INDEX "outlet_ratings_outlet_id_key" ON "outlet_ratings" ("outlet_id");
-- Create index "outletrating_average_rating" to table: "outlet_ratings"
CREATE INDEX "outletrating_average_rating" ON "outlet_ratings" ("average_rating");
-- Create index "outletrating_tenant_id_outlet_id" to table: "outlet_ratings"
CREATE UNIQUE INDEX "outletrating_tenant_id_outlet_id" ON "outlet_ratings" ("tenant_id", "outlet_id");
