-- Modify "catalog_overrides" table
ALTER TABLE "catalog_overrides" ADD COLUMN "available_quantity" double precision NULL;
-- Modify "outlets" table
ALTER TABLE "outlets" ADD COLUMN "booking_deposit_percent" bigint NOT NULL DEFAULT 0;
