-- Add payment_method enum to orders table for COD/PoD workflow support.
-- Also extend payment_status with cod_pending and cod_collected values.

-- Add payment_method column with default 'mpesa' for existing orders
ALTER TABLE "orders" ADD COLUMN IF NOT EXISTS "payment_method" varchar DEFAULT 'mpesa' NOT NULL;

-- Create index for filtering by payment method
CREATE INDEX IF NOT EXISTS "order_payment_method" ON "orders" ("payment_method");
