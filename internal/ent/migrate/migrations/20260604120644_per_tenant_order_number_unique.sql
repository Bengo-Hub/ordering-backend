-- Drop the GLOBAL unique on order_number (made two tenants collide on the same day's NNNN).
DROP INDEX IF EXISTS "order_order_number";
-- Create per-tenant unique index "order_tenant_id_order_number" to table: "orders".
CREATE UNIQUE INDEX "order_tenant_id_order_number" ON "orders" ("tenant_id", "order_number");
