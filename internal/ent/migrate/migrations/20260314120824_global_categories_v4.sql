-- Modify "menu_categories" table
ALTER TABLE "menu_categories" ALTER COLUMN "tenant_id" DROP NOT NULL, ALTER COLUMN "cafe_id" DROP NOT NULL;
