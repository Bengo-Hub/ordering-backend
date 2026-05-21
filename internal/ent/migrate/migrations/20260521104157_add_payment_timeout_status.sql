-- Modify "orders" table
ALTER TABLE "orders" DROP CONSTRAINT "orders_users_orders", ALTER COLUMN "customer_id" DROP NOT NULL, ADD CONSTRAINT "orders_users_orders" FOREIGN KEY ("customer_id") REFERENCES "users" ("id") ON UPDATE NO ACTION ON DELETE SET NULL;
