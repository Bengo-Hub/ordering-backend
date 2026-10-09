-- Delivery zones moved to logistics-api (geo_fences + delivery quote policy).
-- Data was backed up to delivery_zones_bk_20261009 before this ran.
DROP TABLE IF EXISTS "delivery_zones";
