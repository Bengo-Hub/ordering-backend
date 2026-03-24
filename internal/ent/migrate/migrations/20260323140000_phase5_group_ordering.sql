-- Create "group_orders" table
CREATE TABLE "group_orders" (
    "id" uuid NOT NULL,
    "tenant_id" uuid NOT NULL,
    "host_user_id" uuid NOT NULL,
    "cart_id" uuid NOT NULL,
    "invite_code" character varying(6) NOT NULL,
    "status" character varying NOT NULL DEFAULT 'open',
    "max_participants" bigint NOT NULL DEFAULT 10,
    "expires_at" timestamptz NOT NULL,
    "created_at" timestamptz NOT NULL,
    "updated_at" timestamptz NOT NULL,
    PRIMARY KEY ("id")
);
-- Create index "grouporder_invite_code" to table: "group_orders"
CREATE UNIQUE INDEX "grouporder_invite_code" ON "group_orders" ("invite_code");
-- Create index "grouporder_tenant_id_host_user_id" to table: "group_orders"
CREATE INDEX "grouporder_tenant_id_host_user_id" ON "group_orders" ("tenant_id", "host_user_id");
-- Create index "grouporder_status" to table: "group_orders"
CREATE INDEX "grouporder_status" ON "group_orders" ("status");
-- Create index "grouporder_expires_at" to table: "group_orders"
CREATE INDEX "grouporder_expires_at" ON "group_orders" ("expires_at");

-- Create "group_participants" table
CREATE TABLE "group_participants" (
    "id" uuid NOT NULL,
    "group_order_id" uuid NOT NULL,
    "user_id" uuid NOT NULL,
    "user_name" character varying(255) NOT NULL,
    "joined_at" timestamptz NOT NULL,
    PRIMARY KEY ("id"),
    CONSTRAINT "group_participants_group_orders_participants" FOREIGN KEY ("group_order_id") REFERENCES "group_orders" ("id") ON UPDATE NO ACTION ON DELETE NO ACTION
);
-- Create index "groupparticipant_group_order_id_user_id" to table: "group_participants"
CREATE UNIQUE INDEX "groupparticipant_group_order_id_user_id" ON "group_participants" ("group_order_id", "user_id");
