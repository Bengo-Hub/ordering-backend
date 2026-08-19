package ordering

import (
	"context"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/bengobox/ordering-backend/internal/modules/catalog"
)

// validateAndPriceItems re-derives each line's authoritative unit price from the catalog
// (never trusts the client-submitted unitPrice/totalPrice/modifier price_adjustment values),
// closing a price-tampering gap: a crafted checkout request could otherwise submit an
// arbitrary price for any item or modifier selection and be charged/recorded exactly that.
//
// For each item: looks up the real catalog item by SKU (the same lookup the storefront's own
// catalog API uses, so a legitimate client's numbers always match exactly — no surprise
// mismatches), matches each submitted modifier selection against the item's real modifier
// options (dropping any that don't exist — e.g. a stale cart after a menu change), and
// recomputes unitPrice = item.BasePrice + sum(validated modifier price_adjustment),
// totalPrice = unitPrice * quantity.
//
// Fails closed: if catalogSvc isn't wired (e.g. a test harness) items pass through unchanged;
// if a specific SKU can't be resolved, the whole checkout is rejected with
// ErrCatalogItemUnavailable rather than silently trusting the client's price for that line —
// checkout already requires inventory-api to be reachable (stock reservation fails fast on it),
// so this doesn't introduce a new availability dependency.
func (s *OrderService) validateAndPriceItems(ctx context.Context, tenantSlug string, tenantID uuid.UUID, items []CreateOrderItemInput) ([]CreateOrderItemInput, error) {
	if s.catalogSvc == nil {
		return items, nil
	}
	out := make([]CreateOrderItemInput, len(items))
	for i, it := range items {
		out[i] = it
		if it.InventorySKU == "" {
			continue
		}
		item, err := s.catalogSvc.GetItem(ctx, tenantSlug, tenantID, it.InventorySKU, nil)
		if err != nil || item == nil {
			s.logger.Warn("price validation: item lookup failed",
				zap.String("sku", it.InventorySKU), zap.Error(err))
			return nil, ErrCatalogItemUnavailable
		}

		validatedModifiers := make([]CartItemModifier, 0, len(it.Modifiers))
		modifierTotal := 0.0
		for _, m := range it.Modifiers {
			option := findModifierOption(item.ModifierGroups, m.GroupID, m.OptionID)
			if option == nil {
				s.logger.Warn("price validation: unknown modifier option, dropping",
					zap.String("sku", it.InventorySKU), zap.String("optionId", m.OptionID.String()))
				continue
			}
			validatedModifiers = append(validatedModifiers, CartItemModifier{
				GroupID:         m.GroupID,
				GroupName:       m.GroupName,
				OptionID:        m.OptionID,
				OptionName:      option.Name,
				PriceAdjustment: option.PriceAdjustment, // server-authoritative — never the client's number
			})
			modifierTotal += option.PriceAdjustment
		}

		out[i].Modifiers = validatedModifiers
		out[i].UnitPrice = item.BasePrice + modifierTotal
		out[i].TotalPrice = out[i].UnitPrice * float64(it.Quantity)
	}
	return out, nil
}

// findModifierOption looks up a submitted (groupID, optionID) pair against an item's real
// modifier groups, returning nil when no match exists (deleted option, tampered id, etc.).
func findModifierOption(groups []catalog.CatalogModifierGroup, groupID, optionID uuid.UUID) *catalog.CatalogModifierOption {
	gid, oid := groupID.String(), optionID.String()
	for _, g := range groups {
		if g.ID != gid {
			continue
		}
		for i := range g.Options {
			if g.Options[i].ID == oid {
				return &g.Options[i]
			}
		}
	}
	return nil
}
