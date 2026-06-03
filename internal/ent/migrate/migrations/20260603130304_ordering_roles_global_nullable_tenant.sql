-- Modify "ordering_roles" table
ALTER TABLE "ordering_roles" ALTER COLUMN "tenant_id" DROP NOT NULL;
-- Create index "orderingrole_role_code" to table: "ordering_roles"
CREATE INDEX "orderingrole_role_code" ON "ordering_roles" ("role_code");
