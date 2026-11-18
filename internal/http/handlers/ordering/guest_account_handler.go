package orderinghandler

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	ratelimit "github.com/Bengo-Hub/shared-ratelimit"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/bengobox/ordering-backend/internal/http/handlers"
	"github.com/bengobox/ordering-backend/internal/modules/ordering"
	"github.com/bengobox/ordering-backend/internal/platform/authaccount"
)

// GuestAccountClient creates a customer account in auth-api (authaccount.Client).
type GuestAccountClient interface {
	SendSignupCode(ctx context.Context, email, tenantSlug string) error
	VerifySignupCode(ctx context.Context, email, code string) error
	Register(ctx context.Context, in authaccount.RegisterInput) error
}

// SetGuestAccountClient enables POST /guest/account.
func (h *OrderHandler) SetGuestAccountClient(c GuestAccountClient) { h.accountClient = c }

// SetRateLimiter adds the per-order limit on POST /guest/account (the router limits per IP).
func (h *OrderHandler) SetRateLimiter(l *ratelimit.Limiter) { h.limiter = l }

// GuestAccountMinPasswordLength matches the storefront's check. auth-api's own policy still
// applies on top.
const GuestAccountMinPasswordLength = 8

// GuestAccountRequest: without email_code the email gets a sign-up code; with it (and the
// password) the account is created. Name, email and phone come from the order.
type GuestAccountRequest struct {
	OrderID   string `json:"order_id"`
	SessionID string `json:"session_id"`
	Password  string `json:"password,omitempty"`
	EmailCode string `json:"email_code,omitempty"`
}

// GuestAccountResponse: status is code_sent, created or exists.
type GuestAccountResponse struct {
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

const guestAccountExistsMessage = "You already have an account with this email. Sign in next time to check out faster."

// CreateGuestAccount turns a guest order into a customer account, without leaving the page.
// @Summary Create an account from a guest order
// @Description Step 1 (no email_code) emails a sign-up code to the order's email. Step 2 (password and email_code) creates the customer account in auth-api. Only the checkout session that placed the order may call it.
// @Tags Checkout
// @Accept json
// @Produce json
// @Param tenant path string true "Tenant slug"
// @Param payload body GuestAccountRequest true "Order, session, and for step 2 password and code"
// @Success 200 {object} GuestAccountResponse
// @Failure 400 {object} handlers.ErrorResponse
// @Failure 403 {object} handlers.ErrorResponse
// @Failure 429 {object} handlers.ErrorResponse
// @Router /guest/account [post]
func (h *OrderHandler) CreateGuestAccount(w http.ResponseWriter, r *http.Request) {
	if h.accountClient == nil {
		handlers.RespondError(w, http.StatusServiceUnavailable, "accounts can't be created here right now")
		return
	}
	tenantID, err := getTenantID(r)
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid tenant")
		return
	}
	var req GuestAccountRequest
	if err := decodeJSON(r, &req); err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	orderID, err := uuid.Parse(req.OrderID)
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid order_id")
		return
	}

	// Per order as well as per IP (router): one guest order is one person, so a handful of
	// tries is plenty and the order id cannot be used to spray codes.
	if h.limiter != nil {
		if ok, retry := h.limiter.Allow(r.Context(), "order:"+orderID.String(),
			ratelimit.Options{Name: "guest-account", Limit: 6, Window: 15 * time.Minute}, 1); !ok {
			w.Header().Set("Retry-After", retryAfterSeconds(retry))
			handlers.RespondError(w, http.StatusTooManyRequests, "Too many attempts. Please try again in a few minutes.")
			return
		}
	}

	order, err := h.orderService.GetOrder(r.Context(), tenantID, orderID)
	if err != nil {
		if errors.Is(err, ordering.ErrOrderNotFound) {
			handlers.RespondError(w, http.StatusForbidden, "access denied")
			return
		}
		h.handleError(w, err)
		return
	}
	// Same check as the guest order routes: only the checkout session that placed it.
	if !guestSessionOwns(order, req.SessionID) {
		handlers.RespondError(w, http.StatusForbidden, "access denied")
		return
	}

	email := strings.ToLower(strings.TrimSpace(metaString(order.Metadata, "contactEmail")))
	if email == "" {
		respondFieldError(w, "email", "email_required", "An account needs an email address. Your details can still be saved on this device.")
		return
	}
	tenantSlug := h.orderService.TenantSlug(r.Context(), order.TenantID)

	// Step 1: email a code. The password is not needed yet and is ignored if sent.
	if strings.TrimSpace(req.EmailCode) == "" {
		switch err := h.accountClient.SendSignupCode(r.Context(), email, tenantSlug); {
		case err == nil:
			handlers.RespondJSON(w, http.StatusOK, GuestAccountResponse{Status: "code_sent"})
		case errors.Is(err, authaccount.ErrAccountExists):
			handlers.RespondJSON(w, http.StatusOK, GuestAccountResponse{Status: "exists", Message: guestAccountExistsMessage})
		default:
			h.respondAccountError(w, err)
		}
		return
	}

	// Step 2: create the account.
	if utf8.RuneCountInString(req.Password) < GuestAccountMinPasswordLength {
		respondFieldError(w, "password", "password_too_short", "Use at least 8 characters.")
		return
	}
	in := authaccount.RegisterInput{
		Email:      email,
		Password:   req.Password,
		TenantSlug: tenantSlug,
		Name:       ordering.NormalizeContactName(metaString(order.Metadata, "contactName")),
		Phone:      strings.TrimSpace(metaString(order.Metadata, "contactPhone")),
	}
	// Register first: after a weak password the email is still verified (auth-api keeps it for
	// 30 minutes) and the code is already used, so a retry must not need the code again.
	err = h.accountClient.Register(r.Context(), in)
	if errors.Is(err, authaccount.ErrEmailNotVerified) {
		if verr := h.accountClient.VerifySignupCode(r.Context(), email, strings.TrimSpace(req.EmailCode)); verr != nil {
			h.respondAccountError(w, verr)
			return
		}
		err = h.accountClient.Register(r.Context(), in)
	}
	switch {
	case err == nil:
		h.log.Info("customer account created from guest order",
			zap.String("tenant_id", tenantID.String()), zap.String("order_id", orderID.String()))
		handlers.RespondJSON(w, http.StatusOK, GuestAccountResponse{Status: "created"})
	case errors.Is(err, authaccount.ErrAccountExists):
		handlers.RespondJSON(w, http.StatusOK, GuestAccountResponse{Status: "exists", Message: guestAccountExistsMessage})
	default:
		h.respondAccountError(w, err)
	}
}

// respondAccountError answers an auth-api refusal with a message for the guest. Nothing about
// other accounts is revealed.
func (h *OrderHandler) respondAccountError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, authaccount.ErrCodeInvalid):
		respondFieldError(w, "email_code", "code_invalid", "That code is not right. Check the email and try again.")
	case errors.Is(err, authaccount.ErrCodeExpired), errors.Is(err, authaccount.ErrEmailNotVerified):
		respondFieldError(w, "email_code", "code_expired", "That code has expired. Send a new one.")
	case errors.Is(err, authaccount.ErrTooManyAttempts):
		handlers.RespondError(w, http.StatusTooManyRequests, "Too many attempts. Please try again in a few minutes.")
	case errors.Is(err, authaccount.ErrWeakPassword):
		respondFieldError(w, "password", "weak_password", "That password is too easy to guess. Try a longer one with upper and lower case letters, a number and a symbol.")
	case errors.Is(err, authaccount.ErrNotAllowed):
		handlers.RespondError(w, http.StatusForbidden, "This store does not take customer accounts yet. Your details can still be saved on this device.")
	default:
		h.log.Warn("guest account step failed", zap.Error(err))
		handlers.RespondError(w, http.StatusBadGateway, "We could not create your account right now. Please try again later.")
	}
}

// respondFieldError is a 400 the storefront shows next to one form field.
func respondFieldError(w http.ResponseWriter, field, code, message string) {
	handlers.RespondJSON(w, http.StatusBadRequest, map[string]string{
		"error":   http.StatusText(http.StatusBadRequest),
		"message": message,
		"field":   field,
		"code":    code,
	})
}

func metaString(meta map[string]interface{}, key string) string {
	if meta == nil {
		return ""
	}
	s, _ := meta[key].(string)
	return s
}

func retryAfterSeconds(d time.Duration) string {
	secs := int(d.Round(time.Second) / time.Second)
	if secs < 1 {
		secs = 1
	}
	return strconv.Itoa(secs)
}
