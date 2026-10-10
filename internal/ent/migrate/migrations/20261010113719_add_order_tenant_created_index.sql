-- Create index "order_tenant_id_created_at" to table: "orders"
CREATE INDEX "order_tenant_id_created_at" ON "orders" ("tenant_id", "created_at");
