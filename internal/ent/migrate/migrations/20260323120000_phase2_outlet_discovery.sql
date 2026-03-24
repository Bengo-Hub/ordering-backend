-- Add "supports_pickup" column to "outlets" table
ALTER TABLE "outlets" ADD COLUMN "supports_pickup" boolean NOT NULL DEFAULT false;
