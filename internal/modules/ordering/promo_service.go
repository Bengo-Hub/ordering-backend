package ordering

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/bengobox/ordering-backend/internal/platform/posdiscounts"
)

// PromoService provides promo code business logic.
type PromoService struct {
	repo   Repository
	logger *zap.Logger
	// discountsClient evaluates promo codes against pos-api's Promotion+PromotionRule SoT (the
	// SAME schedule/meal_period/scope/BOGO evaluator the POS terminal uses) instead of the local
	// PromoCode model below, which has none of that logic. Nil/disabled (INTERNAL_SERVICE_KEY
	// unset) falls back to the local model — see SetDiscountsClient.
	discountsClient *posdiscounts.Client
}

// NewPromoService creates a new promo service.
func NewPromoService(repo Repository, logger *zap.Logger) *PromoService {
	return &PromoService{
		repo:   repo,
		logger: logger,
	}
}

// SetDiscountsClient wires the pos-api discount-evaluation S2S client (mirrors LoyaltyService.
// SetPOSLoyaltyClient's pattern) — set once at startup in app.go, after the client itself is
// constructed.
func (s *PromoService) SetDiscountsClient(c *posdiscounts.Client) { s.discountsClient = c }

// ValidatePromoCode validates a promo code against the cart's REAL items. items is used both to
// compute the subtotal (min-subtotal gate, legacy-path discount calc) and — when the pos-api
// discounts client is configured — as the cart lines the SoT evaluator scopes/schedules against.
func (s *PromoService) ValidatePromoCode(ctx context.Context, tenantID, outletID uuid.UUID, code string, items []CartItem, userID *uuid.UUID) (*PromoValidationResult, error) {
	code = strings.ToUpper(strings.TrimSpace(code))
	if code == "" {
		return &PromoValidationResult{
			Valid:        false,
			ErrorMessage: "promo code is required",
		}, nil
	}

	var subtotal float64
	for _, it := range items {
		subtotal += it.TotalPrice
	}

	// pos-api Promotion+PromotionRule is the platform discount source of truth — prefer it so a
	// code is scoped/scheduled identically everywhere (POS terminal, Add Sale, storefront). Falls
	// back to the local PromoCode model only when the S2S client isn't configured for this
	// environment. A CONFIGURED client that fails (timeout/5xx) fails CLOSED with a clear message
	// rather than silently falling back — a transient pos-api outage must never look like "code
	// not found" or, worse, quietly resolve against stale/duplicate local data.
	if s.discountsClient != nil && s.discountsClient.Enabled() {
		var outletPtr *uuid.UUID
		if outletID != uuid.Nil {
			outletPtr = &outletID
		}
		lines := make([]posdiscounts.ApplyLine, 0, len(items))
		for _, it := range items {
			line := posdiscounts.ApplyLine{SKU: it.InventorySKU, Quantity: float64(it.Quantity), UnitPrice: it.UnitPrice}
			// Category snapshot, taken at add-to-cart time (cart_service.go) — lets category-
			// scoped discounts match here the same way they do on the POS terminal.
			if cat, ok := it.Metadata["category"].(string); ok {
				line.Category = cat
			}
			lines = append(lines, line)
		}
		res, err := s.discountsClient.ApplyDiscount(ctx, tenantID, outletPtr, code, lines)
		if err != nil {
			s.logger.Error("pos-api discount evaluation failed", zap.Error(err))
			return &PromoValidationResult{Valid: false, ErrorMessage: "unable to validate promo code right now — please try again"}, nil
		}
		if !res.Valid {
			return &PromoValidationResult{Valid: false, ErrorMessage: res.Reason}, nil
		}
		amt, _ := strconv.ParseFloat(res.DiscountAmount, 64)
		var promoID *uuid.UUID
		if pid, perr := uuid.Parse(res.PromoID); perr == nil {
			promoID = &pid
		}
		return &PromoValidationResult{
			Valid:          true,
			PromoCodeID:    promoID,
			DiscountType:   PromoCodeTypeFixedAmount, // already a resolved KES amount, not a %/rate
			DiscountValue:  amt,
			DiscountAmount: amt,
		}, nil
	}

	// Legacy local-model path (no schedule/meal_period/scope logic) — transitional fallback only.
	// Get promo code
	promo, err := s.repo.GetPromoCodeByCode(ctx, tenantID, code)
	if err != nil {
		return &PromoValidationResult{
			Valid:        false,
			ErrorMessage: "promo code not found",
		}, nil
	}

	// Check if active
	if !promo.IsActive {
		return &PromoValidationResult{
			Valid:        false,
			ErrorMessage: "promo code is not active",
		}, nil
	}

	// Check outlet restriction
	if promo.OutletID != nil && *promo.OutletID != outletID {
		return &PromoValidationResult{
			Valid:        false,
			ErrorMessage: "promo code is not valid for this outlet",
		}, nil
	}

	// Check date range
	now := time.Now()
	if promo.StartsAt != nil && now.Before(*promo.StartsAt) {
		return &PromoValidationResult{
			Valid:        false,
			ErrorMessage: "promo code is not yet active",
		}, nil
	}

	if promo.EndsAt != nil && now.After(*promo.EndsAt) {
		return &PromoValidationResult{
			Valid:        false,
			ErrorMessage: "promo code has expired",
		}, nil
	}

	// Check max uses
	if promo.MaxUses != nil && promo.UsageCount >= *promo.MaxUses {
		return &PromoValidationResult{
			Valid:        false,
			ErrorMessage: "promo code has reached maximum uses",
		}, nil
	}

	// Check per-user limit if user is logged in
	if userID != nil && promo.MaxUsesPerUser != nil {
		userUsageCount, err := s.repo.CountUserPromoRedemptions(ctx, promo.ID, *userID)
		if err == nil && userUsageCount >= *promo.MaxUsesPerUser {
			return &PromoValidationResult{
				Valid:        false,
				ErrorMessage: "you have reached the maximum uses for this promo code",
			}, nil
		}
	}

	// Check minimum subtotal
	if promo.MinSubtotal > 0 && subtotal < promo.MinSubtotal {
		return &PromoValidationResult{
			Valid:        false,
			ErrorMessage: "cart subtotal does not meet minimum requirement",
		}, nil
	}

	// Calculate discount amount
	discountAmount := s.calculateDiscount(promo, subtotal)

	return &PromoValidationResult{
		Valid:          true,
		PromoCodeID:    &promo.ID,
		DiscountType:   promo.DiscountType,
		DiscountValue:  promo.DiscountValue,
		DiscountAmount: discountAmount,
	}, nil
}

// calculateDiscount calculates the discount amount for a promo code.
func (s *PromoService) calculateDiscount(promo *PromoCode, subtotal float64) float64 {
	var discount float64

	switch promo.DiscountType {
	case PromoCodeTypePercentage:
		discount = subtotal * (promo.DiscountValue / 100)
	case PromoCodeTypeFixedAmount:
		discount = promo.DiscountValue
	case PromoCodeTypeFreeDelivery:
		// Delivery fee will be zeroed out separately
		discount = 0
	case PromoCodeTypeFreeItem:
		// Free item handling is more complex, handled separately
		discount = 0
	}

	// Apply max discount cap if set
	if promo.MaxDiscountAmount != nil && discount > *promo.MaxDiscountAmount {
		discount = *promo.MaxDiscountAmount
	}

	// Ensure discount doesn't exceed subtotal
	if discount > subtotal {
		discount = subtotal
	}

	return discount
}

// ApplyPromoToCart applies a validated promo code to a cart.
func (s *PromoService) ApplyPromoToCart(ctx context.Context, tenantID, cartID uuid.UUID, code string, userID *uuid.UUID) (*Cart, error) {
	// Get cart
	cart, err := s.repo.GetCart(ctx, tenantID, cartID)
	if err != nil {
		return nil, err
	}

	if cart.Status != CartStatusActive {
		return nil, ErrCartAlreadyCheckedOut
	}

	// Load cart items
	items, err := s.repo.ListCartItems(ctx, cartID)
	if err != nil {
		return nil, err
	}

	// Validate promo code against the cart's real items
	result, err := s.ValidatePromoCode(ctx, tenantID, cart.OutletID, code, items, userID)
	if err != nil {
		return nil, err
	}

	if !result.Valid {
		return nil, ErrPromoCodeNotFound
	}

	// Apply promo to cart
	cart.PromoCodeID = result.PromoCodeID
	cart.DiscountTotal = result.DiscountAmount

	// Handle free delivery
	if result.DiscountType == PromoCodeTypeFreeDelivery {
		cart.DeliveryFee = 0
	}

	if err := s.repo.UpdateCart(ctx, cart); err != nil {
		return nil, err
	}

	cart.Items = items
	return cart, nil
}

// RemovePromoFromCart removes the applied promo code from a cart.
func (s *PromoService) RemovePromoFromCart(ctx context.Context, tenantID, cartID uuid.UUID) (*Cart, error) {
	cart, err := s.repo.GetCart(ctx, tenantID, cartID)
	if err != nil {
		return nil, err
	}

	if cart.Status != CartStatusActive {
		return nil, ErrCartAlreadyCheckedOut
	}

	cart.PromoCodeID = nil
	cart.DiscountTotal = 0

	if err := s.repo.UpdateCart(ctx, cart); err != nil {
		return nil, err
	}

	// Load items
	items, err := s.repo.ListCartItems(ctx, cartID)
	if err == nil {
		cart.Items = items
	}

	return cart, nil
}

// CreatePromoCode creates a new promo code.
func (s *PromoService) CreatePromoCode(ctx context.Context, promo *PromoCode) error {
	promo.Code = strings.ToUpper(strings.TrimSpace(promo.Code))

	// Check for duplicate code
	existing, _ := s.repo.GetPromoCodeByCode(ctx, promo.TenantID, promo.Code)
	if existing != nil {
		return ErrPromoCodeNotFound // Should be already exists error
	}

	if err := s.repo.CreatePromoCode(ctx, promo); err != nil {
		s.logger.Error("failed to create promo code", zap.Error(err))
		return err
	}

	s.logger.Info("promo code created",
		zap.String("id", promo.ID.String()),
		zap.String("code", promo.Code))

	return nil
}

// GetPromoCode retrieves a promo code by ID.
func (s *PromoService) GetPromoCode(ctx context.Context, tenantID, promoID uuid.UUID) (*PromoCode, error) {
	return s.repo.GetPromoCode(ctx, tenantID, promoID)
}

// ListPromoCodes lists promo codes for a tenant.
func (s *PromoService) ListPromoCodes(ctx context.Context, tenantID uuid.UUID, isActive *bool) ([]PromoCode, error) {
	return s.repo.ListPromoCodes(ctx, tenantID, isActive)
}

// UpdatePromoCode updates a promo code.
func (s *PromoService) UpdatePromoCode(ctx context.Context, promo *PromoCode) error {
	promo.Code = strings.ToUpper(strings.TrimSpace(promo.Code))
	return s.repo.UpdatePromoCode(ctx, promo)
}

// DeletePromoCode deletes a promo code.
func (s *PromoService) DeletePromoCode(ctx context.Context, tenantID, promoID uuid.UUID) error {
	return s.repo.DeletePromoCode(ctx, tenantID, promoID)
}

// RecordPromoRedemption records a promo code redemption when an order is placed.
func (s *PromoService) RecordPromoRedemption(ctx context.Context, promoID, orderID, userID uuid.UUID, discountAmount float64) error {
	redemption := &PromoRedemption{
		PromoCodeID:    promoID,
		OrderID:        orderID,
		UserID:         userID,
		DiscountAmount: discountAmount,
		RedeemedAt:     time.Now(),
	}

	if err := s.repo.CreatePromoRedemption(ctx, redemption); err != nil {
		return err
	}

	// Increment usage count
	return s.repo.IncrementPromoUsage(ctx, promoID)
}
