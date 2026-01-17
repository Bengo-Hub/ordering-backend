package orderinghandler

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/bengobox/ordering-backend/internal/http/handlers"
	identityhandler "github.com/bengobox/ordering-backend/internal/http/handlers/identity"
	"github.com/bengobox/ordering-backend/internal/modules/ordering"
)

// AddressHandler exposes customer address HTTP endpoints.
type AddressHandler struct {
	log        *zap.Logger
	addressSvc *ordering.AddressService
}

// NewAddressHandler constructs an AddressHandler instance.
func NewAddressHandler(log *zap.Logger, addressSvc *ordering.AddressService) *AddressHandler {
	return &AddressHandler{
		log:        log.Named("ordering.AddressHandler"),
		addressSvc: addressSvc,
	}
}

// Register mounts address routes on the supplied router.
func (h *AddressHandler) Register(r chi.Router, auth *identityhandler.Authenticator) {
	r.Route("/addresses", func(addressRouter chi.Router) {
		addressRouter.Use(auth.RequireAuth)

		addressRouter.Post("/", h.CreateAddress)
		addressRouter.Get("/", h.ListAddresses)
		addressRouter.Get("/default", h.GetDefaultAddress)
		addressRouter.Get("/{addressId}", h.GetAddress)
		addressRouter.Put("/{addressId}", h.UpdateAddress)
		addressRouter.Delete("/{addressId}", h.DeleteAddress)
		addressRouter.Put("/{addressId}/default", h.SetDefaultAddress)
	})
}

// --- Request/Response Types ---

// CreateAddressRequestDTO represents a request to create an address.
type CreateAddressRequestDTO struct {
	Label        string   `json:"label"`
	AddressLine1 string   `json:"addressLine1"`
	AddressLine2 string   `json:"addressLine2,omitempty"`
	City         string   `json:"city"`
	County       string   `json:"county,omitempty"`
	PostalCode   string   `json:"postalCode,omitempty"`
	Country      string   `json:"country,omitempty"`
	Latitude     *float64 `json:"latitude,omitempty"`
	Longitude    *float64 `json:"longitude,omitempty"`
	PlusCode     string   `json:"plusCode,omitempty"`
	ContactName  string   `json:"contactName,omitempty"`
	ContactPhone string   `json:"contactPhone,omitempty"`
	Instructions string   `json:"instructions,omitempty"`
	IsDefault    bool     `json:"isDefault,omitempty"`
}

// UpdateAddressRequestDTO represents a request to update an address.
type UpdateAddressRequestDTO struct {
	Label        *string  `json:"label,omitempty"`
	AddressLine1 *string  `json:"addressLine1,omitempty"`
	AddressLine2 *string  `json:"addressLine2,omitempty"`
	City         *string  `json:"city,omitempty"`
	County       *string  `json:"county,omitempty"`
	PostalCode   *string  `json:"postalCode,omitempty"`
	Country      *string  `json:"country,omitempty"`
	Latitude     *float64 `json:"latitude,omitempty"`
	Longitude    *float64 `json:"longitude,omitempty"`
	PlusCode     *string  `json:"plusCode,omitempty"`
	ContactName  *string  `json:"contactName,omitempty"`
	ContactPhone *string  `json:"contactPhone,omitempty"`
	Instructions *string  `json:"instructions,omitempty"`
	IsDefault    *bool    `json:"isDefault,omitempty"`
}

// --- Helper Functions ---

func (h *AddressHandler) handleError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ordering.ErrAddressNotFound):
		handlers.RespondError(w, http.StatusNotFound, err.Error())

	case errors.Is(err, ordering.ErrAddressAlreadyExists):
		handlers.RespondError(w, http.StatusConflict, err.Error())

	case errors.Is(err, ordering.ErrInvalidAddress),
		errors.Is(err, ordering.ErrMaxAddressesReached):
		handlers.RespondError(w, http.StatusBadRequest, err.Error())

	case errors.Is(err, ordering.ErrUnauthorized):
		handlers.RespondError(w, http.StatusForbidden, err.Error())

	default:
		h.log.Error("internal error", zap.Error(err))
		handlers.RespondError(w, http.StatusInternalServerError, "internal server error")
	}
}

// --- Handlers ---

// CreateAddress creates a new customer address.
// @Summary Create address
// @Description Creates a new delivery address for the current user
// @Tags Addresses
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer token"
// @Param X-Tenant-ID header string true "Tenant ID"
// @Param payload body CreateAddressRequestDTO true "Address data"
// @Success 201 {object} ordering.CustomerAddress
// @Failure 400 {object} handlers.ErrorResponse
// @Failure 401 {object} handlers.ErrorResponse
// @Router /addresses [post]
func (h *AddressHandler) CreateAddress(w http.ResponseWriter, r *http.Request) {
	tenantID, err := getTenantID(r)
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid tenant")
		return
	}

	user, err := getUserFromContext(r)
	if err != nil {
		handlers.RespondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req CreateAddressRequestDTO
	if err := decodeJSON(r, &req); err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	address, err := h.addressSvc.CreateAddress(r.Context(), ordering.CreateAddressRequest{
		TenantID:     tenantID,
		UserID:       user.ID,
		Label:        req.Label,
		AddressLine1: req.AddressLine1,
		AddressLine2: req.AddressLine2,
		City:         req.City,
		County:       req.County,
		PostalCode:   req.PostalCode,
		Country:      req.Country,
		Latitude:     req.Latitude,
		Longitude:    req.Longitude,
		PlusCode:     req.PlusCode,
		ContactName:  req.ContactName,
		ContactPhone: req.ContactPhone,
		Instructions: req.Instructions,
		IsDefault:    req.IsDefault,
	})
	if err != nil {
		h.handleError(w, err)
		return
	}

	handlers.RespondJSON(w, http.StatusCreated, address)
}

// GetAddress retrieves an address by ID.
// @Summary Get address
// @Description Retrieves a delivery address by its ID
// @Tags Addresses
// @Produce json
// @Param Authorization header string true "Bearer token"
// @Param X-Tenant-ID header string true "Tenant ID"
// @Param addressId path string true "Address ID"
// @Success 200 {object} ordering.CustomerAddress
// @Failure 400 {object} handlers.ErrorResponse
// @Failure 401 {object} handlers.ErrorResponse
// @Failure 403 {object} handlers.ErrorResponse
// @Failure 404 {object} handlers.ErrorResponse
// @Router /addresses/{addressId} [get]
func (h *AddressHandler) GetAddress(w http.ResponseWriter, r *http.Request) {
	tenantID, err := getTenantID(r)
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid tenant")
		return
	}

	user, err := getUserFromContext(r)
	if err != nil {
		handlers.RespondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	addressID, err := uuid.Parse(chi.URLParam(r, "addressId"))
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid address ID")
		return
	}

	address, err := h.addressSvc.GetAddress(r.Context(), tenantID, addressID)
	if err != nil {
		h.handleError(w, err)
		return
	}

	// Verify ownership
	if address.UserID != user.ID {
		handlers.RespondError(w, http.StatusForbidden, "access denied")
		return
	}

	handlers.RespondJSON(w, http.StatusOK, address)
}

// GetDefaultAddress retrieves the user's default address.
// @Summary Get default address
// @Description Retrieves the current user's default delivery address
// @Tags Addresses
// @Produce json
// @Param Authorization header string true "Bearer token"
// @Param X-Tenant-ID header string true "Tenant ID"
// @Success 200 {object} ordering.CustomerAddress
// @Failure 400 {object} handlers.ErrorResponse
// @Failure 401 {object} handlers.ErrorResponse
// @Failure 404 {object} handlers.ErrorResponse
// @Router /addresses/default [get]
func (h *AddressHandler) GetDefaultAddress(w http.ResponseWriter, r *http.Request) {
	tenantID, err := getTenantID(r)
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid tenant")
		return
	}

	user, err := getUserFromContext(r)
	if err != nil {
		handlers.RespondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	address, err := h.addressSvc.GetDefaultAddress(r.Context(), tenantID, user.ID)
	if err != nil {
		h.handleError(w, err)
		return
	}

	handlers.RespondJSON(w, http.StatusOK, address)
}

// ListAddresses lists all addresses for the current user.
// @Summary List addresses
// @Description Lists all delivery addresses for the current user
// @Tags Addresses
// @Produce json
// @Param Authorization header string true "Bearer token"
// @Param X-Tenant-ID header string true "Tenant ID"
// @Success 200 {array} ordering.CustomerAddress
// @Failure 400 {object} handlers.ErrorResponse
// @Failure 401 {object} handlers.ErrorResponse
// @Router /addresses [get]
func (h *AddressHandler) ListAddresses(w http.ResponseWriter, r *http.Request) {
	tenantID, err := getTenantID(r)
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid tenant")
		return
	}

	user, err := getUserFromContext(r)
	if err != nil {
		handlers.RespondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	addresses, err := h.addressSvc.ListAddresses(r.Context(), tenantID, user.ID)
	if err != nil {
		h.handleError(w, err)
		return
	}

	handlers.RespondJSON(w, http.StatusOK, addresses)
}

// UpdateAddress updates an existing address.
// @Summary Update address
// @Description Updates an existing delivery address
// @Tags Addresses
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer token"
// @Param X-Tenant-ID header string true "Tenant ID"
// @Param addressId path string true "Address ID"
// @Param payload body UpdateAddressRequestDTO true "Updated address data"
// @Success 200 {object} ordering.CustomerAddress
// @Failure 400 {object} handlers.ErrorResponse
// @Failure 401 {object} handlers.ErrorResponse
// @Failure 403 {object} handlers.ErrorResponse
// @Failure 404 {object} handlers.ErrorResponse
// @Router /addresses/{addressId} [put]
func (h *AddressHandler) UpdateAddress(w http.ResponseWriter, r *http.Request) {
	tenantID, err := getTenantID(r)
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid tenant")
		return
	}

	user, err := getUserFromContext(r)
	if err != nil {
		handlers.RespondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	addressID, err := uuid.Parse(chi.URLParam(r, "addressId"))
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid address ID")
		return
	}

	var req UpdateAddressRequestDTO
	if err := decodeJSON(r, &req); err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	address, err := h.addressSvc.UpdateAddress(r.Context(), ordering.UpdateAddressRequest{
		TenantID:     tenantID,
		AddressID:    addressID,
		UserID:       user.ID,
		Label:        req.Label,
		AddressLine1: req.AddressLine1,
		AddressLine2: req.AddressLine2,
		City:         req.City,
		County:       req.County,
		PostalCode:   req.PostalCode,
		Country:      req.Country,
		Latitude:     req.Latitude,
		Longitude:    req.Longitude,
		PlusCode:     req.PlusCode,
		ContactName:  req.ContactName,
		ContactPhone: req.ContactPhone,
		Instructions: req.Instructions,
		IsDefault:    req.IsDefault,
	})
	if err != nil {
		h.handleError(w, err)
		return
	}

	handlers.RespondJSON(w, http.StatusOK, address)
}

// DeleteAddress deletes an address.
// @Summary Delete address
// @Description Deletes a delivery address
// @Tags Addresses
// @Param Authorization header string true "Bearer token"
// @Param X-Tenant-ID header string true "Tenant ID"
// @Param addressId path string true "Address ID"
// @Success 204 "No Content"
// @Failure 400 {object} handlers.ErrorResponse
// @Failure 401 {object} handlers.ErrorResponse
// @Failure 403 {object} handlers.ErrorResponse
// @Failure 404 {object} handlers.ErrorResponse
// @Router /addresses/{addressId} [delete]
func (h *AddressHandler) DeleteAddress(w http.ResponseWriter, r *http.Request) {
	tenantID, err := getTenantID(r)
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid tenant")
		return
	}

	user, err := getUserFromContext(r)
	if err != nil {
		handlers.RespondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	addressID, err := uuid.Parse(chi.URLParam(r, "addressId"))
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid address ID")
		return
	}

	if err := h.addressSvc.DeleteAddress(r.Context(), tenantID, addressID, user.ID); err != nil {
		h.handleError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// SetDefaultAddress sets an address as the default.
// @Summary Set default address
// @Description Sets the specified address as the user's default delivery address
// @Tags Addresses
// @Param Authorization header string true "Bearer token"
// @Param X-Tenant-ID header string true "Tenant ID"
// @Param addressId path string true "Address ID"
// @Success 200 {object} map[string]string
// @Failure 400 {object} handlers.ErrorResponse
// @Failure 401 {object} handlers.ErrorResponse
// @Failure 403 {object} handlers.ErrorResponse
// @Failure 404 {object} handlers.ErrorResponse
// @Router /addresses/{addressId}/default [put]
func (h *AddressHandler) SetDefaultAddress(w http.ResponseWriter, r *http.Request) {
	tenantID, err := getTenantID(r)
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid tenant")
		return
	}

	user, err := getUserFromContext(r)
	if err != nil {
		handlers.RespondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	addressID, err := uuid.Parse(chi.URLParam(r, "addressId"))
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid address ID")
		return
	}

	if err := h.addressSvc.SetDefaultAddress(r.Context(), tenantID, user.ID, addressID); err != nil {
		h.handleError(w, err)
		return
	}

	handlers.RespondJSON(w, http.StatusOK, map[string]string{"status": "success"})
}
