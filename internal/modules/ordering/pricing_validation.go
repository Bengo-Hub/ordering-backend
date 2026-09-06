package ordering

import (
	"context"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/bengobox/ordering-backend/internal/modules/catalog"
	"github.com/bengobox/ordering-backend/internal/platform/posdiscounts"
)

// validateAndPriceItems re-derives each line's authoritative unit price from the catalog
// (never trusts the client-submitted unitPrice/totalPrice/modifier price_adjustment values),
// closing a price-tampering gap: a crafted checkout request could otherwise submit an
// arbitrary price for any item or modifier selection and be charged/recorded exactly that.
//
// For each item: looks up the real catalog item by SKU (the same lookup the storefront's own
// catalog API uses, so a legitimate client's numbers always match exactly — no surprise
// mismatches), matches each submitted modifier selection against the item's real modifier
// options (dropping any that don't exist — e.g. a stale cart after a menu change), applies any
// currently-active pos-api deal to the base price (see applyActiveDealDiscount — this is the
// 2026-09-06 fix for a real bug: the storefront's "Top Deals" grid could show a discounted price
// that checkout then silently overcharged, since this function used to consult only the plain
// catalog price), and recomputes unitPrice = discountedBasePrice + sum(validated modifier
// price_adjustment), totalPrice = unitPrice * quantity.
//
// Fails closed: if catalogSvc isn't wired (e.g. a test harness) items pass through unchanged;
// if a specific SKU can't be resolved, the whole checkout is rejected with
// ErrCatalogItemUnavailable rather than silently trusting the client's price for that line —
// checkout already requires inventory-api to be reachable (stock reservation fails fast on it),
// so this doesn't introduce a new availability dependency. The deal lookup itself is best-effort
// (same posture as ListDeals/ListBanners elsewhere in this package) — a promotions-service
// hiccup falls back to the plain catalog price rather than blocking checkout.
func (s *OrderService) validateAndPriceItems(ctx context.Context, tenantSlug string, tenantID uuid.UUID, items []CreateOrderItemInput) ([]CreateOrderItemInput, error) {
	if s.catalogSvc == nil {
		return items, nil
	}
	deals := s.activeDeals(ctx, tenantID)
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

		basePrice := applyActiveDealDiscount(item, deals)
		out[i].Modifiers = validatedModifiers
		out[i].UnitPrice = basePrice + modifierTotal
		out[i].TotalPrice = out[i].UnitPrice * float64(it.Quantity)
	}
	return out, nil
}

// activeDeals fetches the tenant's active, in-window pos-api deals for checkout-time discount
// matching. Best-effort: returns nil (no discount applied to anything) if the discounts client
// isn't wired or the lookup fails — never blocks checkout.
func (s *OrderService) activeDeals(ctx context.Context, tenantID uuid.UUID) []posdiscounts.Discount {
	if s.promoSvc == nil || s.promoSvc.discountsClient == nil {
		return nil
	}
	return s.promoSvc.discountsClient.ListDeals(ctx, tenantID)
}

// applyActiveDealDiscount returns item's checkout-time base price after applying the best
// matching active deal, or item.BasePrice unchanged if none match. This is the server-side
// mirror of ordering-frontend's promo-deals.ts (resolveDealItems + applyDeal) — kept in lockstep
// deliberately so the price shown on the storefront's deals grid is EXACTLY what checkout
// charges. BOGO and "all"-scope deals are skipped, same as the frontend: BOGO doesn't reduce to
// a single per-unit price, and "all"-scope deals are banner-appropriate, not a per-item concern.
func applyActiveDealDiscount(item *catalog.MergedCatalogItem, deals []posdiscounts.Discount) float64 {
	price := item.BasePrice
	for _, deal := range deals {
		rule := deal.Rule
		if rule == nil || rule.ScopeType == "all" || rule.DiscountType == "bogo" {
			continue
		}
		matches := false
		switch rule.ScopeType {
		case "item":
			for _, id := range rule.ScopeIDs {
				if id == item.InventorySKU || id == item.InventoryID.String() {
					matches = true
					break
				}
			}
		case "category":
			for _, id := range rule.ScopeIDs {
				if id == item.CategoryName {
					matches = true
					break
				}
			}
		}
		if !matches {
			continue
		}
		switch rule.DiscountType {
		case "percentage":
			discount := price * (rule.DiscountValue / 100)
			if rule.MaxDiscount > 0 && discount > rule.MaxDiscount {
				discount = rule.MaxDiscount
			}
			price -= discount
		case "fixed_amount":
			price -= rule.DiscountValue
		case "fixed_price":
			price = rule.DiscountValue
		}
		if price < 0 {
			price = 0
		}
		break // first matching deal wins, same as the frontend (one badge per item)
	}
	return price
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
