-- Create "google_business_connections" table
CREATE TABLE "google_business_connections" ("id" uuid NOT NULL, "tenant_id" uuid NOT NULL, "outlet_id" uuid NULL, "account_id" character varying NULL, "location_id" character varying NULL, "place_id" character varying NULL, "location_name" character varying NULL, "encrypted_tokens" text NULL, "token_expiry" timestamptz NULL, "status" character varying NOT NULL DEFAULT 'disconnected', "connected_at" timestamptz NULL, "connected_by" character varying NULL, "created_at" timestamptz NOT NULL, "updated_at" timestamptz NOT NULL, PRIMARY KEY ("id"));
-- Create index "googlebusinessconnection_tenant_id" to table: "google_business_connections"
CREATE INDEX "googlebusinessconnection_tenant_id" ON "google_business_connections" ("tenant_id");
-- Create index "googlebusinessconnection_tenant_id_outlet_id" to table: "google_business_connections"
CREATE UNIQUE INDEX "googlebusinessconnection_tenant_id_outlet_id" ON "google_business_connections" ("tenant_id", "outlet_id");
