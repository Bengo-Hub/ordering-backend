package ordering

import (
	"context"
	"encoding/json"
	"math"
	"time"

	"github.com/bengobox/ordering-backend/internal/modules/catalog"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// haversineDistance calculates the distance in km between two lat/lng points
// using the Haversine formula.
func haversineDistance(lat1, lng1, lat2, lng2 float64) float64 {
	const earthRadiusKm = 6371.0

	dLat := degreesToRadians(lat2 - lat1)
	dLng := degreesToRadians(lng2 - lng1)

	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(degreesToRadians(lat1))*math.Cos(degreesToRadians(lat2))*
			math.Sin(dLng/2)*math.Sin(dLng/2)
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))

	return earthRadiusKm * c
}

// degreesToRadians converts degrees to radians.
func degreesToRadians(degrees float64) float64 {
	return degrees * math.Pi / 180
}

// CartService provides shopping cart business logic.
type CartService struct {
	repo       Repository
	catalogSvc *catalog.ProxyService
	logger     *zap.Logger
}

// NewCartService creates a new cart service.
func NewCartService(repo Repository, catalogSvc *catalog.ProxyService, logger *zap.Logger) *CartService {
	return &CartService{
		repo:       repo,
		catalogSvc: catalogSvc,
		logger:     logger,
	}
}

// GetOrCreateCart gets the active cart for a user or session, creating one if it doesn't exist.
func (s *CartService) GetOrCreateCart(ctx context.Context, tenantID, outletID uuid.UUID, userID *uuid.UUID, sessionID string) (*Cart, error) {
	var cart *Cart
	var err error

	// Try to find existing active cart
	if userID != nil {
		cart, err = s.repo.GetActiveCartByUser(ctx, tenantID, outletID, *userID)
	} else if sessionID != "" {
		cart, err = s.repo.GetActiveCartBySession(ctx, tenantID, outletID, sessionID)
	} else {
		return nil, ErrUnauthorized
	}

	if err == nil && cart != nil {
		// Check if cart is expired
		if cart.ExpiresAt != nil && cart.ExpiresAt.Before(time.Now()) {
			// Mark as expired and create new cart
			cart.Status = CartStatusExpired
			if err := s.repo.UpdateCart(ctx, cart); err != nil {
				s.logger.Error("failed to expire cart", zap.Error(err))
			}
			cart = nil
		} else {
			// Load cart items
			items, err := s.repo.ListCartItems(ctx, cart.ID)
			if err == nil {
				cart.Items = items
			}
			return cart, nil
		}
	}

	// Create new cart
	expiresAt := time.Now().Add(CartExpirationDuration)
	cart = &Cart{
		TenantID:  tenantID,
		OutletID:  outletID,
		UserID:    userID,
		SessionID: sessionID,
		Status:    CartStatusActive,
		Currency:  DefaultCurrency,
		ExpiresAt: &expiresAt,
	}

	if err := s.repo.CreateCart(ctx, cart); err != nil {
		s.logger.Error("failed to create cart", zap.Error(err))
		return nil, err
	}

	s.logger.Info("cart created", zap.String("id", cart.ID.String()))
	return cart, nil
}

// GetCart retrieves a cart by ID.
func (s *CartService) GetCart(ctx context.Context, tenantID, cartID uuid.UUID) (*Cart, error) {
	cart, err := s.repo.GetCart(ctx, tenantID, cartID)
	if err != nil {
		return nil, err
	}

	// Load cart items
	items, err := s.repo.ListCartItems(ctx, cart.ID)
	if err == nil {
		cart.Items = items
	}

	return cart, nil
}

// AddItem adds an item to the cart.
func (s *CartService) AddItem(ctx context.Context, req AddItemRequest) (*Cart, error) {
	// Get or create cart
	cart, err := s.GetOrCreateCart(ctx, req.TenantID, req.OutletID, req.UserID, req.SessionID)
	if err != nil {
		return nil, err
	}

	// Validate cart status
	if cart.Status != CartStatusActive {
		return nil, ErrCartAlreadyCheckedOut
	}

	// Validate quantity
	if req.Quantity <= 0 {
		return nil, ErrInvalidQuantity
	}

	// Resolve tenant slug for inventory-api call
	tenantInfo, err := s.repo.GetTenantByID(ctx, req.TenantID)
	if err != nil {
		s.logger.Error("failed to resolve tenant", zap.Error(err))
		return nil, ErrCatalogItemUnavailable
	}

	// Get catalog item to validate availability and price via proxy service
	catalogItem, err := s.catalogSvc.GetItem(ctx, tenantInfo.Slug, req.TenantID, req.InventorySKU, nil)
	if err != nil {
		return nil, ErrCatalogItemUnavailable
	}

	if !catalogItem.IsAvailable {
		return nil, ErrCatalogItemUnavailable
	}

	// Calculate unit price
	unitPrice := catalogItem.BasePrice

	// Calculate modifier total from selected modifiers
	var modifierTotal float64
	for _, m := range req.Modifiers {
		modifierTotal += m.PriceAdjustment
	}

	// Check if item already exists in cart (only merge if no modifiers or same modifiers)
	existingItem, err := s.repo.GetCartItemBySKU(ctx, cart.ID, req.InventorySKU, req.VariantID)
	hasModifiers := len(req.Modifiers) > 0
	if err == nil && existingItem != nil && !hasModifiers && len(existingItem.Modifiers) == 0 {
		// Update quantity (only merge items without modifiers)
		existingItem.Quantity += req.Quantity
		existingItem.TotalPrice = (unitPrice + existingItem.ModifierTotal) * float64(existingItem.Quantity)
		if req.Notes != "" {
			existingItem.Notes = req.Notes
		}

		if err := s.repo.UpdateCartItem(ctx, existingItem); err != nil {
			s.logger.Error("failed to update cart item", zap.Error(err))
			return nil, err
		}
	} else {
		// Create new cart item (always new line item when modifiers are present).
		// Snapshot the item's tax (rate/inclusive/code, enriched by inventory-api from treasury-api)
		// so cart/order VAT totals can be computed without re-fetching the catalog.
		meta := map[string]any{}
		if catalogItem.TaxRate != nil && *catalogItem.TaxRate > 0 {
			meta["tax_rate"] = *catalogItem.TaxRate
		}
		meta["tax_inclusive"] = catalogItem.TaxInclusive
		if catalogItem.TaxCodeID != "" {
			meta["tax_code_id"] = catalogItem.TaxCodeID
		}
		item := &CartItem{
			CartID:        cart.ID,
			InventorySKU:  req.InventorySKU,
			VariantID:     req.VariantID,
			NameSnapshot:  catalogItem.Name,
			Quantity:      req.Quantity,
			UnitPrice:     unitPrice,
			Modifiers:     req.Modifiers,
			ModifierTotal: modifierTotal,
			TotalPrice:    (unitPrice + modifierTotal) * float64(req.Quantity),
			Notes:         req.Notes,
			Metadata:      meta,
		}

		if err := s.repo.CreateCartItem(ctx, item); err != nil {
			s.logger.Error("failed to create cart item", zap.Error(err))
			return nil, err
		}
	}

	// Recalculate cart totals
	if err := s.recalculateCartTotals(ctx, cart); err != nil {
		s.logger.Error("failed to recalculate cart totals", zap.Error(err))
		return nil, err
	}

	// Reload cart with items
	return s.GetCart(ctx, req.TenantID, cart.ID)
}

// UpdateItem updates a cart item.
func (s *CartService) UpdateItem(ctx context.Context, req UpdateItemRequest) (*Cart, error) {
	cart, err := s.repo.GetCart(ctx, req.TenantID, req.CartID)
	if err != nil {
		return nil, err
	}

	if cart.Status != CartStatusActive {
		return nil, ErrCartAlreadyCheckedOut
	}

	item, err := s.repo.GetCartItem(ctx, req.CartID, req.ItemID)
	if err != nil {
		return nil, ErrCartItemNotFound
	}

	if req.Quantity != nil {
		if *req.Quantity <= 0 {
			// Remove item if quantity is zero or less
			if err := s.repo.DeleteCartItem(ctx, req.CartID, req.ItemID); err != nil {
				return nil, err
			}
		} else {
			item.Quantity = *req.Quantity
			item.TotalPrice = (item.UnitPrice + item.ModifierTotal) * float64(item.Quantity)
		}
	}

	if req.Notes != nil {
		item.Notes = *req.Notes
	}

	if req.Quantity == nil || *req.Quantity > 0 {
		if err := s.repo.UpdateCartItem(ctx, item); err != nil {
			return nil, err
		}
	}

	// Recalculate cart totals
	if err := s.recalculateCartTotals(ctx, cart); err != nil {
		return nil, err
	}

	return s.GetCart(ctx, req.TenantID, cart.ID)
}

// RemoveItem removes an item from the cart.
func (s *CartService) RemoveItem(ctx context.Context, tenantID, cartID, itemID uuid.UUID) (*Cart, error) {
	cart, err := s.repo.GetCart(ctx, tenantID, cartID)
	if err != nil {
		return nil, err
	}

	if cart.Status != CartStatusActive {
		return nil, ErrCartAlreadyCheckedOut
	}

	if err := s.repo.DeleteCartItem(ctx, cartID, itemID); err != nil {
		return nil, err
	}

	// Recalculate cart totals
	if err := s.recalculateCartTotals(ctx, cart); err != nil {
		return nil, err
	}

	return s.GetCart(ctx, tenantID, cart.ID)
}

// ClearCart removes all items from the cart.
func (s *CartService) ClearCart(ctx context.Context, tenantID, cartID uuid.UUID) (*Cart, error) {
	cart, err := s.repo.GetCart(ctx, tenantID, cartID)
	if err != nil {
		return nil, err
	}

	if cart.Status != CartStatusActive {
		return nil, ErrCartAlreadyCheckedOut
	}

	if err := s.repo.ClearCartItems(ctx, cartID); err != nil {
		return nil, err
	}

	// Reset cart totals
	cart.Subtotal = 0
	cart.DiscountTotal = 0
	cart.TaxTotal = 0
	cart.PromoCodeID = nil
	cart.LoyaltyPointsRedeemed = 0

	if err := s.repo.UpdateCart(ctx, cart); err != nil {
		return nil, err
	}

	return s.GetCart(ctx, tenantID, cart.ID)
}

// GetCartSummary calculates and returns the cart summary.
func (s *CartService) GetCartSummary(ctx context.Context, tenantID, cartID uuid.UUID) (*CartSummary, error) {
	cart, err := s.GetCart(ctx, tenantID, cartID)
	if err != nil {
		return nil, err
	}

	loyaltyDiscount := float64(cart.LoyaltyPointsRedeemed) * LoyaltyPointValue
	grandTotal := cart.Subtotal - cart.DiscountTotal - loyaltyDiscount + cart.TaxTotal + cart.DeliveryFee

	return &CartSummary{
		Subtotal:              cart.Subtotal,
		DiscountTotal:         cart.DiscountTotal,
		TaxTotal:              cart.TaxTotal,
		DeliveryFee:           cart.DeliveryFee,
		LoyaltyPointsRedeemed: cart.LoyaltyPointsRedeemed,
		LoyaltyDiscount:       loyaltyDiscount,
		GrandTotal:            grandTotal,
	}, nil
}

// recalculateCartTotals recalculates the cart totals based on items.
func (s *CartService) recalculateCartTotals(ctx context.Context, cart *Cart) error {
	items, err := s.repo.ListCartItems(ctx, cart.ID)
	if err != nil {
		return err
	}

	// Subtotal is net-of-tax; TaxTotal is the VAT portion. For tax-inclusive items the tax is
	// computed backwards from the line total (net = total/(1+rate)); for exclusive items it is
	// added on top. Grand total = Subtotal + TaxTotal (see GetCartSummary), so both cases reconcile
	// to the displayed gross price. Rates/flags are snapshotted from the catalog (inventory→treasury).
	var subtotal, taxTotal float64
	for _, item := range items {
		// TotalPrice already includes modifiers: (unitPrice + modifierTotal) * quantity
		lineTotal := item.TotalPrice
		rate, _ := item.Metadata["tax_rate"].(float64)
		inclusive, _ := item.Metadata["tax_inclusive"].(bool)
		if rate > 0 {
			if inclusive {
				net := lineTotal / (1 + rate/100)
				subtotal += net
				taxTotal += lineTotal - net
			} else {
				subtotal += lineTotal
				taxTotal += lineTotal * rate / 100
			}
		} else {
			subtotal += lineTotal
		}
	}

	cart.Subtotal = subtotal
	cart.TaxTotal = taxTotal
	cart.Items = items

	// Extend expiration on activity
	expiresAt := time.Now().Add(CartExpirationDuration)
	cart.ExpiresAt = &expiresAt

	return s.repo.UpdateCart(ctx, cart)
}

// loadDeliveryFeeConfig loads the tenant's configurable delivery fee rates.
func (s *CartService) loadDeliveryFeeConfig(ctx context.Context, tenantID uuid.UUID) (baseFee, perKm, freeMin float64) {
	featuresMap, err := s.repo.GetTenantFeatures(ctx, tenantID)
	if err != nil {
		return DeliveryFeeBase, DeliveryFeePerKm, FreeDeliveryMinimum
	}
	feeRaw, ok := featuresMap["fee_config"]
	if !ok {
		return DeliveryFeeBase, DeliveryFeePerKm, FreeDeliveryMinimum
	}
	b, err := json.Marshal(feeRaw)
	if err != nil {
		return DeliveryFeeBase, DeliveryFeePerKm, FreeDeliveryMinimum
	}
	var cfg FeeConfig
	if err := json.Unmarshal(b, &cfg); err != nil {
		return DeliveryFeeBase, DeliveryFeePerKm, FreeDeliveryMinimum
	}
	base := cfg.DeliveryFeeBase
	if base <= 0 {
		base = DeliveryFeeBase
	}
	perKmRate := cfg.DeliveryFeePerKm
	if perKmRate <= 0 {
		perKmRate = DeliveryFeePerKm
	}
	freeMinimum := cfg.FreeDeliveryMinimum
	if freeMinimum <= 0 {
		freeMinimum = FreeDeliveryMinimum
	}
	return base, perKmRate, freeMinimum
}

// CalculateDeliveryFee calculates the delivery fee for a given delivery location.
// Priority: 1) Zone with polygon match → use zone fee. 2) Zone without polygon (fallback) → use zone fee.
// 3) No zone match → auto-calculate: base fee + per-km rate using Haversine distance.
// Base fee and per-km rate are configurable per-tenant via TenantSetting.features["fee_config"].
func (s *CartService) CalculateDeliveryFee(ctx context.Context, tenantID uuid.UUID, outletID *uuid.UUID, lat, lng float64) (float64, error) {
	baseFee, perKmRate, _ := s.loadDeliveryFeeConfig(ctx, tenantID)

	// Check for configured delivery zones with polygon match
	zones, err := s.repo.ListActiveDeliveryZones(ctx, tenantID, outletID)
	if err != nil {
		s.logger.Warn("failed to query delivery zones, falling back to distance-based fee", zap.Error(err))
	}

	// Priority 1: zone whose geo-fence polygon actually contains the delivery point
	for _, zone := range zones {
		if zone.ZonePolygon != nil && zone.DeliveryFee > 0 && pointInZonePolygon(lat, lng, zone.ZonePolygon) {
			return zone.DeliveryFee, nil
		}
	}

	// Priority 2: fallback zone (no polygon) with explicit fee
	for _, zone := range zones {
		if zone.ZonePolygon == nil && zone.DeliveryFee > 0 {
			return zone.DeliveryFee, nil
		}
	}

	// Priority 3: auto-calculate from distance (base + per-km)
	if outletID == nil {
		return baseFee, nil
	}

	outlet, err := s.catalogSvc.GetOutlet(ctx, tenantID, *outletID)
	if err != nil {
		s.logger.Warn("failed to get outlet for distance calculation, using base fee", zap.Error(err))
		return baseFee, nil
	}

	if outlet.Latitude == nil || outlet.Longitude == nil {
		return baseFee, nil
	}

	distanceKm := haversineDistance(*outlet.Latitude, *outlet.Longitude, lat, lng)
	fee := baseFee + (perKmRate * distanceKm)

	return math.Round(fee*100) / 100, nil
}

// ExpireOldCarts marks old carts as expired.
func (s *CartService) ExpireOldCarts(ctx context.Context, tenantID uuid.UUID) (int, error) {
	count, err := s.repo.ExpireOldCarts(ctx, tenantID)
	if err != nil {
		s.logger.Error("failed to expire old carts", zap.Error(err))
		return 0, err
	}

	if count > 0 {
		s.logger.Info("expired old carts", zap.Int("count", count))
	}

	return count, nil
}

// MergeGuestCart merges a guest cart into a user's cart after login.
func (s *CartService) MergeGuestCart(ctx context.Context, tenantID, outletID uuid.UUID, sessionID string, userID uuid.UUID) (*Cart, error) {
	// Get guest cart
	guestCart, err := s.repo.GetActiveCartBySession(ctx, tenantID, outletID, sessionID)
	if err != nil || guestCart == nil {
		// No guest cart to merge, just get or create user cart
		return s.GetOrCreateCart(ctx, tenantID, outletID, &userID, "")
	}

	// Get or create user cart
	userCart, err := s.GetOrCreateCart(ctx, tenantID, outletID, &userID, "")
	if err != nil {
		return nil, err
	}

	// If they're the same cart, nothing to merge
	if guestCart.ID == userCart.ID {
		return userCart, nil
	}

	// Get guest cart items
	guestItems, err := s.repo.ListCartItems(ctx, guestCart.ID)
	if err != nil {
		return nil, err
	}

	// Merge items
	for _, guestItem := range guestItems {
		existingItem, err := s.repo.GetCartItemBySKU(ctx, userCart.ID, guestItem.InventorySKU, guestItem.VariantID)
		if err == nil && existingItem != nil {
			// Combine quantities
			existingItem.Quantity += guestItem.Quantity
			existingItem.TotalPrice = existingItem.UnitPrice * float64(existingItem.Quantity)
			if err := s.repo.UpdateCartItem(ctx, existingItem); err != nil {
				s.logger.Error("failed to update cart item during merge", zap.Error(err))
			}
		} else {
			// Move item to user cart
			newItem := &CartItem{
				CartID:       userCart.ID,
				InventorySKU: guestItem.InventorySKU,
				VariantID:    guestItem.VariantID,
				NameSnapshot: guestItem.NameSnapshot,
				Quantity:     guestItem.Quantity,
				UnitPrice:    guestItem.UnitPrice,
				TotalPrice:   guestItem.TotalPrice,
				Notes:        guestItem.Notes,
				Metadata:     guestItem.Metadata,
			}
			if err := s.repo.CreateCartItem(ctx, newItem); err != nil {
				s.logger.Error("failed to create cart item during merge", zap.Error(err))
			}
		}
	}

	// Mark guest cart as abandoned
	guestCart.Status = CartStatusAbandoned
	if err := s.repo.UpdateCart(ctx, guestCart); err != nil {
		s.logger.Error("failed to abandon guest cart", zap.Error(err))
	}

	// Recalculate user cart totals
	if err := s.recalculateCartTotals(ctx, userCart); err != nil {
		s.logger.Error("failed to recalculate cart totals after merge", zap.Error(err))
	}

	return s.GetCart(ctx, tenantID, userCart.ID)
}
