-- Create index "order_scheduled_handoff_due" to table: "orders"
CREATE INDEX "order_scheduled_handoff_due" ON "orders" ("scheduled_for") WHERE (status = 'confirmed' AND scheduled_for IS NOT NULL);
-- Create index "order_stale_payment_placed_at" to table: "orders"
CREATE INDEX "order_stale_payment_placed_at" ON "orders" ("placed_at") WHERE (status = 'pending' AND payment_status = 'pending');
