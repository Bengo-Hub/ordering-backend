package ordering

import (
	"context"
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
	catalogSvc *catalog.Service
	logger     *zap.Logger
}

// NewCartService creates a new cart service.
func NewCartService(repo Repository, catalogSvc *catalog.Service, logger *zap.Logger) *CartService {
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

	// Get catalog item to validate availability and price
	catalogItem, err := s.catalogSvc.GetCatalogItem(ctx, req.TenantID, req.CatalogItemID)
	if err != nil {
		return nil, ErrCatalogItemUnavailable
	}

	if !catalogItem.IsAvailable {
		return nil, ErrCatalogItemUnavailable
	}

	// Calculate unit price
	unitPrice := catalogItem.BasePrice

	// If variant specified, validate and adjust price
	if req.VariantID != nil {
		var found bool
		for _, v := range catalogItem.Variants {
			if v.ID == *req.VariantID {
				if !v.IsAvailable {
					return nil, ErrVariantUnavailable
				}
				unitPrice += v.PriceDelta
				found = true
				break
			}
		}
		if !found {
			return nil, ErrVariantUnavailable
		}
	}

	// Check if item already exists in cart
	existingItem, err := s.repo.GetCartItemByCatalogItem(ctx, cart.ID, req.CatalogItemID, req.VariantID)
	if err == nil && existingItem != nil {
		// Update quantity
		existingItem.Quantity += req.Quantity
		existingItem.TotalPrice = unitPrice * float64(existingItem.Quantity)
		if req.Notes != "" {
			existingItem.Notes = req.Notes
		}

		if err := s.repo.UpdateCartItem(ctx, existingItem); err != nil {
			s.logger.Error("failed to update cart item", zap.Error(err))
			return nil, err
		}
	} else {
		// Create new cart item
		item := &CartItem{
			CartID:       cart.ID,
			CatalogItemID: req.CatalogItemID,
			VariantID:    req.VariantID,
			NameSnapshot: catalogItem.Name,
			Quantity:     req.Quantity,
			UnitPrice:    unitPrice,
			TotalPrice:   unitPrice * float64(req.Quantity),
			Notes:        req.Notes,
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
			item.TotalPrice = item.UnitPrice * float64(item.Quantity)
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

	var subtotal float64
	for _, item := range items {
		subtotal += item.TotalPrice
	}

	cart.Subtotal = subtotal
	cart.Items = items

	// Extend expiration on activity
	expiresAt := time.Now().Add(CartExpirationDuration)
	cart.ExpiresAt = &expiresAt

	return s.repo.UpdateCart(ctx, cart)
}

// CalculateDeliveryFee calculates the delivery fee for a given delivery location.
// It first checks for tenant-configured delivery zones with fees. If no zones are
// configured, it falls back to distance-based calculation using the Haversine formula.
func (s *CartService) CalculateDeliveryFee(ctx context.Context, tenantID uuid.UUID, outletID *uuid.UUID, lat, lng float64) (float64, error) {
	// Check for configured delivery zones
	zones, err := s.repo.ListActiveDeliveryZones(ctx, tenantID, outletID)
	if err != nil {
		s.logger.Warn("failed to query delivery zones, falling back to distance-based fee", zap.Error(err))
	}

	// If a zone with a delivery_fee > 0 exists, use that fee
	for _, zone := range zones {
		if zone.DeliveryFee > 0 {
			return zone.DeliveryFee, nil
		}
	}

	// No zones configured or no zone with fee > 0: calculate based on distance
	if outletID == nil {
		// Cannot calculate distance without an outlet
		return DeliveryFeeBase, nil
	}

	outlet, err := s.catalogSvc.GetOutlet(ctx, tenantID, *outletID)
	if err != nil {
		s.logger.Warn("failed to get outlet for distance calculation, using base fee", zap.Error(err))
		return DeliveryFeeBase, nil
	}

	if outlet.Latitude == nil || outlet.Longitude == nil {
		// Outlet has no coordinates; return base fee
		return DeliveryFeeBase, nil
	}

	distanceKm := haversineDistance(*outlet.Latitude, *outlet.Longitude, lat, lng)
	fee := DeliveryFeeBase + (DeliveryFeePerKm * distanceKm)

	return math.Round(fee*100) / 100, nil // round to 2 decimal places
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
		existingItem, err := s.repo.GetCartItemByCatalogItem(ctx, userCart.ID, guestItem.CatalogItemID, guestItem.VariantID)
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
				CatalogItemID: guestItem.CatalogItemID,
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
