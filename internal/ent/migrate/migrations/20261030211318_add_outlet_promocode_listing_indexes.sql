-- Create index "outlet_tenant_id_status_use_case" to table: "outlets"
CREATE INDEX "outlet_tenant_id_status_use_case" ON "outlets" ("tenant_id", "status", "use_case");
-- Create index "promocode_tenant_id_is_active" to table: "promo_codes"
CREATE INDEX "promocode_tenant_id_is_active" ON "promo_codes" ("tenant_id", "is_active");
