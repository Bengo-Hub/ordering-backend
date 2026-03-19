package ordering

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// PromoService provides promo code business logic.
type PromoService struct {
	repo   Repository
	logger *zap.Logger
}

// NewPromoService creates a new promo service.
func NewPromoService(repo Repository, logger *zap.Logger) *PromoService {
	return &PromoService{
		repo:   repo,
		logger: logger,
	}
}

// ValidatePromoCode validates a promo code for the given cart.
func (s *PromoService) ValidatePromoCode(ctx context.Context, tenantID, outletID uuid.UUID, code string, subtotal float64, userID *uuid.UUID) (*PromoValidationResult, error) {
	code = strings.ToUpper(strings.TrimSpace(code))
	if code == "" {
		return &PromoValidationResult{
			Valid:        false,
			ErrorMessage: "promo code is required",
		}, nil
	}

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

	// Calculate subtotal from items
	var subtotal float64
	for _, item := range items {
		subtotal += item.TotalPrice
	}

	// Validate promo code
	result, err := s.ValidatePromoCode(ctx, tenantID, cart.OutletID, code, subtotal, userID)
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
