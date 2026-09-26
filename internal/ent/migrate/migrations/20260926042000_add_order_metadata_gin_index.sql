-- Create index "order_metadata_gin" to table: "orders"
CREATE INDEX "order_metadata_gin" ON "orders" USING GIN ("metadata" jsonb_path_ops);
