package ordering

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// MaxAddressesPerUser is the maximum number of addresses a user can have.
const MaxAddressesPerUser = 10

// AddressService provides address management business logic.
type AddressService struct {
	repo   Repository
	logger *zap.Logger
}

// NewAddressService creates a new address service.
func NewAddressService(repo Repository, logger *zap.Logger) *AddressService {
	return &AddressService{
		repo:   repo,
		logger: logger,
	}
}

// CreateAddressRequest represents a request to create an address.
type CreateAddressRequest struct {
	TenantID     uuid.UUID
	UserID       uuid.UUID
	Label        string
	AddressLine1 string
	AddressLine2 string
	City         string
	County       string
	PostalCode   string
	Country      string
	Latitude     *float64
	Longitude    *float64
	PlusCode     string
	ContactName  string
	ContactPhone string
	Instructions string
	IsDefault    bool
}

// CreateAddress creates a new customer address.
func (s *AddressService) CreateAddress(ctx context.Context, req CreateAddressRequest) (*CustomerAddress, error) {
	// Validate required fields
	if strings.TrimSpace(req.AddressLine1) == "" {
		return nil, ErrInvalidAddress
	}
	if strings.TrimSpace(req.City) == "" {
		return nil, ErrInvalidAddress
	}

	// Check address limit
	count, err := s.repo.CountUserAddresses(ctx, req.TenantID, req.UserID)
	if err != nil {
		return nil, err
	}
	if count >= MaxAddressesPerUser {
		return nil, ErrMaxAddressesReached
	}

	// Set default country
	country := req.Country
	if country == "" {
		country = "Kenya"
	}

	address := &CustomerAddress{
		TenantID:             req.TenantID,
		UserID:               req.UserID,
		Label:                strings.TrimSpace(req.Label),
		AddressLine1:         strings.TrimSpace(req.AddressLine1),
		AddressLine2:         strings.TrimSpace(req.AddressLine2),
		City:                 strings.TrimSpace(req.City),
		County:               strings.TrimSpace(req.County),
		PostalCode:           strings.TrimSpace(req.PostalCode),
		Country:              country,
		Latitude:             req.Latitude,
		Longitude:            req.Longitude,
		PlusCode:     strings.TrimSpace(req.PlusCode),
		ContactName:  strings.TrimSpace(req.ContactName),
		ContactPhone: strings.TrimSpace(req.ContactPhone),
		Instructions: strings.TrimSpace(req.Instructions),
		IsDefault:    req.IsDefault,
	}

	// If this is the first address or marked as default, set it as default
	if count == 0 || req.IsDefault {
		address.IsDefault = true
	}

	if err := s.repo.CreateAddress(ctx, address); err != nil {
		s.logger.Error("failed to create address", zap.Error(err))
		return nil, err
	}

	// If set as default, clear default from other addresses
	if address.IsDefault {
		if err := s.repo.SetDefaultAddress(ctx, req.TenantID, req.UserID, address.ID); err != nil {
			s.logger.Error("failed to set default address", zap.Error(err))
		}
	}

	s.logger.Info("address created",
		zap.String("id", address.ID.String()),
		zap.String("userId", req.UserID.String()))

	return address, nil
}

// GetAddress retrieves an address by ID.
func (s *AddressService) GetAddress(ctx context.Context, tenantID, addressID uuid.UUID) (*CustomerAddress, error) {
	return s.repo.GetAddress(ctx, tenantID, addressID)
}

// GetDefaultAddress retrieves the user's default address.
func (s *AddressService) GetDefaultAddress(ctx context.Context, tenantID, userID uuid.UUID) (*CustomerAddress, error) {
	return s.repo.GetDefaultAddress(ctx, tenantID, userID)
}

// ListAddresses lists all addresses for a user.
func (s *AddressService) ListAddresses(ctx context.Context, tenantID, userID uuid.UUID) ([]CustomerAddress, error) {
	return s.repo.ListAddresses(ctx, tenantID, userID)
}

// UpdateAddressRequest represents a request to update an address.
type UpdateAddressRequest struct {
	TenantID     uuid.UUID
	AddressID    uuid.UUID
	UserID       uuid.UUID
	Label        *string
	AddressLine1 *string
	AddressLine2 *string
	City         *string
	County       *string
	PostalCode   *string
	Country      *string
	Latitude     *float64
	Longitude    *float64
	PlusCode     *string
	ContactName  *string
	ContactPhone *string
	Instructions *string
	IsDefault    *bool
}

// UpdateAddress updates an existing address.
func (s *AddressService) UpdateAddress(ctx context.Context, req UpdateAddressRequest) (*CustomerAddress, error) {
	address, err := s.repo.GetAddress(ctx, req.TenantID, req.AddressID)
	if err != nil {
		return nil, err
	}

	// Verify ownership
	if address.UserID != req.UserID {
		return nil, ErrUnauthorized
	}

	if req.Label != nil {
		address.Label = strings.TrimSpace(*req.Label)
	}
	if req.AddressLine1 != nil {
		address.AddressLine1 = strings.TrimSpace(*req.AddressLine1)
	}
	if req.AddressLine2 != nil {
		address.AddressLine2 = strings.TrimSpace(*req.AddressLine2)
	}
	if req.City != nil {
		address.City = strings.TrimSpace(*req.City)
	}
	if req.County != nil {
		address.County = strings.TrimSpace(*req.County)
	}
	if req.PostalCode != nil {
		address.PostalCode = strings.TrimSpace(*req.PostalCode)
	}
	if req.Country != nil {
		address.Country = strings.TrimSpace(*req.Country)
	}
	if req.Latitude != nil {
		address.Latitude = req.Latitude
	}
	if req.Longitude != nil {
		address.Longitude = req.Longitude
	}
	if req.PlusCode != nil {
		address.PlusCode = strings.TrimSpace(*req.PlusCode)
	}
	if req.ContactName != nil {
		address.ContactName = strings.TrimSpace(*req.ContactName)
	}
	if req.ContactPhone != nil {
		address.ContactPhone = strings.TrimSpace(*req.ContactPhone)
	}
	if req.Instructions != nil {
		address.Instructions = strings.TrimSpace(*req.Instructions)
	}
	if req.IsDefault != nil {
		address.IsDefault = *req.IsDefault
	}

	// Validate required fields
	if address.AddressLine1 == "" || address.City == "" {
		return nil, ErrInvalidAddress
	}

	if err := s.repo.UpdateAddress(ctx, address); err != nil {
		s.logger.Error("failed to update address", zap.Error(err))
		return nil, err
	}

	// Update default address if needed
	if req.IsDefault != nil && *req.IsDefault {
		if err := s.repo.SetDefaultAddress(ctx, req.TenantID, req.UserID, address.ID); err != nil {
			s.logger.Error("failed to set default address", zap.Error(err))
		}
	}

	s.logger.Info("address updated", zap.String("id", address.ID.String()))

	return address, nil
}

// DeleteAddress deletes an address.
func (s *AddressService) DeleteAddress(ctx context.Context, tenantID, addressID, userID uuid.UUID) error {
	address, err := s.repo.GetAddress(ctx, tenantID, addressID)
	if err != nil {
		return err
	}

	// Verify ownership
	if address.UserID != userID {
		return ErrUnauthorized
	}

	if err := s.repo.DeleteAddress(ctx, tenantID, addressID); err != nil {
		s.logger.Error("failed to delete address", zap.Error(err))
		return err
	}

	// If this was the default address, set another one as default
	if address.IsDefault {
		addresses, err := s.repo.ListAddresses(ctx, tenantID, userID)
		if err == nil && len(addresses) > 0 {
			// Set the first remaining address as default
			if err := s.repo.SetDefaultAddress(ctx, tenantID, userID, addresses[0].ID); err != nil {
				s.logger.Error("failed to set new default address", zap.Error(err))
			}
		}
	}

	s.logger.Info("address deleted", zap.String("id", addressID.String()))

	return nil
}

// SetDefaultAddress sets an address as the user's default.
func (s *AddressService) SetDefaultAddress(ctx context.Context, tenantID, userID, addressID uuid.UUID) error {
	address, err := s.repo.GetAddress(ctx, tenantID, addressID)
	if err != nil {
		return err
	}

	// Verify ownership
	if address.UserID != userID {
		return ErrUnauthorized
	}

	return s.repo.SetDefaultAddress(ctx, tenantID, userID, addressID)
}
