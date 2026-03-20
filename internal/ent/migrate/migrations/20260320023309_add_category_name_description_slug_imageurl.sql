-- Modify "catalog_categories" table
ALTER TABLE "catalog_categories" ADD COLUMN "name" character varying NOT NULL, ADD COLUMN "slug" character varying NULL, ADD COLUMN "description" text NULL, ADD COLUMN "image_url" character varying NULL;
-- Modify "catalog_items" table
ALTER TABLE "catalog_items" ADD COLUMN "name" character varying NOT NULL, ADD COLUMN "description" character varying NULL, ADD COLUMN "base_price" double precision NOT NULL DEFAULT 0, ADD COLUMN "currency" character varying NOT NULL DEFAULT 'KES';
