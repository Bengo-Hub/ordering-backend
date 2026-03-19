# Catalog–Inventory Linkage (SKU & BOM)

## Overview

Catalog items in the ordering service are linked to inventory for stock keeping via **SKU**. Each catalog item has a unique SKU; the inventory service holds item master data and **recipes (BOMs)** that define which inventory components (and quantities per serving) are consumed when the item is sold.

## Data Flow

1. **Ordering-backend** owns catalogs (categories, items, pricing, availability). Each `CatalogItem` has a required `sku` field.
2. **Inventory-service** owns items (by SKU), BOMs (recipes), and stock. A BOM links a parent item (e.g. same SKU as the catalog item) to component items with quantity per serving.
3. When an order is placed, the ordering service can emit events or call inventory APIs so that inventory deducts recipe components according to the BOM.

## Linkage Rules

- **Single link**: Catalog item `sku` ↔ Inventory item `sku`. The same SKU in both systems identifies the same sellable product.
- **Recipe (BOM)**: Configured in inventory-service. Parent item = product (by SKU); components = inventory items + quantity per serving. No BOM data is stored in ordering-backend.
- **Seeded data**: Ordering seed creates catalog items with SKUs (e.g. `BEV-ESP-001`, `MIN-GRL-001`). To support stock deduction, create matching items and BOMs in inventory-service for those SKUs.

## Seed and BOM

- Ordering-backend seed script creates categories and catalog items with stable SKUs. Main courses, beverages, pastries, etc. are included.
- Inventory-service should seed or allow creation of items with the same SKUs and optional BOMs so that when orders are placed, recipe components can be deducted.

## Cross-Reference

- See **inventory-service** `docs/erd.md` for `item_boms` and `item_bom_components`.
- See **CROSS-SERVICE-DATA-OWNERSHIP.md** in this repo for ownership: ordering owns catalog/catalogs; inventory owns items, BOMs, and stock.
