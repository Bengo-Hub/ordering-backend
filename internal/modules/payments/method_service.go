package payments

import (
	"context"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// PaymentMethodService provides payment method business logic.
type PaymentMethodService struct {
	repo   Repository
	logger *zap.Logger
}

// NewPaymentMethodService creates a new payment method service.
func NewPaymentMethodService(repo Repository, logger *zap.Logger) *PaymentMethodService {
	return &PaymentMethodService{
		repo:   repo,
		logger: logger,
	}
}

// CreatePaymentMethodRequest represents a request to create a payment method.
type CreatePaymentMethodRequest struct {
	TenantID      uuid.UUID         `json:"tenant_id"`
	UserID        uuid.UUID         `json:"user_id"`
	Provider      PaymentProvider   `json:"provider"`
	Type          PaymentMethodType `json:"type"`
	Mask          string            `json:"mask,omitempty"`
	Label         string            `json:"label,omitempty"`
	ExpMonth      *int              `json:"exp_month,omitempty"`
	ExpYear       *int              `json:"exp_year,omitempty"`
	Fingerprint   string            `json:"fingerprint,omitempty"`
	ProviderToken string            `json:"provider_token,omitempty"`
	SetAsDefault  bool              `json:"set_as_default"`
}

// CreatePaymentMethod creates a new payment method.
func (s *PaymentMethodService) CreatePaymentMethod(ctx context.Context, req CreatePaymentMethodRequest) (*PaymentMethod, error) {
	// Check for duplicate by fingerprint if provided
	if req.Fingerprint != "" {
		existing, err := s.repo.GetPaymentMethodByFingerprint(ctx, req.TenantID, req.Fingerprint)
		if err == nil && existing != nil {
			// Return existing method if fingerprint matches
			return existing, nil
		}
	}

	method := &PaymentMethod{
		TenantID:      req.TenantID,
		UserID:        req.UserID,
		Provider:      req.Provider,
		Type:          req.Type,
		Mask:          req.Mask,
		Label:         req.Label,
		ExpMonth:      req.ExpMonth,
		ExpYear:       req.ExpYear,
		IsDefault:     req.SetAsDefault,
		Fingerprint:   req.Fingerprint,
		ProviderToken: req.ProviderToken,
	}

	// If setting as default, first unset others
	if req.SetAsDefault {
		if err := s.repo.SetDefaultPaymentMethod(ctx, req.TenantID, req.UserID, uuid.Nil); err != nil {
			s.logger.Warn("failed to unset default payment methods",
				zap.Error(err),
				zap.String("user_id", req.UserID.String()))
		}
	}

	if err := s.repo.CreatePaymentMethod(ctx, method); err != nil {
		return nil, err
	}

	s.logger.Info("payment method created",
		zap.String("id", method.ID.String()),
		zap.String("user_id", req.UserID.String()),
		zap.String("provider", string(req.Provider)))

	return method, nil
}

// GetPaymentMethod retrieves a payment method.
func (s *PaymentMethodService) GetPaymentMethod(ctx context.Context, tenantID, id uuid.UUID) (*PaymentMethod, error) {
	return s.repo.GetPaymentMethod(ctx, tenantID, id)
}

// ListPaymentMethods lists payment methods for a user.
func (s *PaymentMethodService) ListPaymentMethods(ctx context.Context, tenantID, userID uuid.UUID) ([]PaymentMethod, error) {
	methods, _, err := s.repo.ListPaymentMethods(ctx, PaymentMethodFilter{
		TenantID: tenantID,
		UserID:   userID,
		Limit:    100, // Reasonable limit for a user's payment methods
	})
	return methods, err
}

// GetDefaultPaymentMethod retrieves the default payment method for a user.
func (s *PaymentMethodService) GetDefaultPaymentMethod(ctx context.Context, tenantID, userID uuid.UUID) (*PaymentMethod, error) {
	methods, _, err := s.repo.ListPaymentMethods(ctx, PaymentMethodFilter{
		TenantID: tenantID,
		UserID:   userID,
		Limit:    100,
	})
	if err != nil {
		return nil, err
	}

	for _, m := range methods {
		if m.IsDefault {
			return &m, nil
		}
	}

	// If no default set, return the first one
	if len(methods) > 0 {
		return &methods[0], nil
	}

	return nil, ErrPaymentMethodNotFound
}

// UpdatePaymentMethodRequest represents a request to update a payment method.
type UpdatePaymentMethodRequest struct {
	TenantID uuid.UUID `json:"tenant_id"`
	ID       uuid.UUID `json:"id"`
	Label    string    `json:"label,omitempty"`
}

// UpdatePaymentMethod updates a payment method.
func (s *PaymentMethodService) UpdatePaymentMethod(ctx context.Context, req UpdatePaymentMethodRequest) (*PaymentMethod, error) {
	method, err := s.repo.GetPaymentMethod(ctx, req.TenantID, req.ID)
	if err != nil {
		return nil, err
	}

	if req.Label != "" {
		method.Label = req.Label
	}

	if err := s.repo.UpdatePaymentMethod(ctx, method); err != nil {
		return nil, err
	}

	return method, nil
}

// SetDefaultPaymentMethod sets a payment method as the default.
func (s *PaymentMethodService) SetDefaultPaymentMethod(ctx context.Context, tenantID, userID, methodID uuid.UUID) error {
	// Verify the method exists and belongs to the user
	method, err := s.repo.GetPaymentMethod(ctx, tenantID, methodID)
	if err != nil {
		return err
	}

	if method.UserID != userID {
		return ErrUnauthorized
	}

	return s.repo.SetDefaultPaymentMethod(ctx, tenantID, userID, methodID)
}

// DeletePaymentMethod deletes a payment method.
func (s *PaymentMethodService) DeletePaymentMethod(ctx context.Context, tenantID, userID, methodID uuid.UUID) error {
	// Verify the method exists and belongs to the user
	method, err := s.repo.GetPaymentMethod(ctx, tenantID, methodID)
	if err != nil {
		return err
	}

	if method.UserID != userID {
		return ErrUnauthorized
	}

	return s.repo.DeletePaymentMethod(ctx, tenantID, methodID)
}
