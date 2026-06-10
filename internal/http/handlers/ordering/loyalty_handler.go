package orderinghandler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	"github.com/bengobox/ordering-backend/internal/http/handlers"
	identityhandler "github.com/bengobox/ordering-backend/internal/http/handlers/identity"
	"github.com/bengobox/ordering-backend/internal/modules/ordering"
	"github.com/bengobox/ordering-backend/internal/platform/subscriptions"
)

// LoyaltyHandler exposes loyalty program HTTP endpoints.
type LoyaltyHandler struct {
	log        *zap.Logger
	loyaltySvc *ordering.LoyaltyService
}

// NewLoyaltyHandler constructs a LoyaltyHandler instance.
func NewLoyaltyHandler(log *zap.Logger, loyaltySvc *ordering.LoyaltyService) *LoyaltyHandler {
	return &LoyaltyHandler{
		log:        log.Named("ordering.LoyaltyHandler"),
		loyaltySvc: loyaltySvc,
	}
}

// Register mounts loyalty routes on the supplied router.
func (h *LoyaltyHandler) Register(r chi.Router, auth *identityhandler.Authenticator) {
	r.Route("/loyalty", func(loyaltyRouter chi.Router) {
		loyaltyRouter.Use(auth.RequireAuth)
		loyaltyRouter.Use(subscriptions.RequireFeature("loyalty_program"))

		loyaltyRouter.Get("/balance", h.GetBalance)
		loyaltyRouter.Get("/account", h.GetAccount)
		// POST /account is an idempotent "register": GetAccount is get-or-create, so the storefront's
		// registerLoyaltyAccount() (POST /loyalty/account) resolves — and creates on first call — the
		// account instead of 404ing.
		loyaltyRouter.Post("/account", h.GetAccount)
		loyaltyRouter.Get("/transactions", h.GetTransactions)
		loyaltyRouter.Get("/tier-benefits", h.GetTierBenefits)
	})
}

// --- Response Types ---

// BalanceResponse represents the loyalty balance response.
type BalanceResponse struct {
	Balance      int     `json:"balance"`
	BalanceValue float64 `json:"balanceValue"`
	Currency     string  `json:"currency"`
}

// AccountResponse represents the loyalty account response.
type AccountResponse struct {
	ID             string                 `json:"id"`
	BalancePoints  int                    `json:"balancePoints"`
	BalanceValue   float64                `json:"balanceValue"`
	Tier           string                 `json:"tier"`
	LifetimePoints int                    `json:"lifetimePoints"`
	TierBenefits   map[string]interface{} `json:"tierBenefits"`
	Currency       string                 `json:"currency"`
}

// TransactionsResponse represents the paginated transactions response.
type TransactionsResponse struct {
	Data  []ordering.LoyaltyTransaction `json:"data"`
	Total int                           `json:"total"`
	Limit int                           `json:"limit"`
	Page  int                           `json:"page"`
}

// --- Helper Functions ---

func (h *LoyaltyHandler) handleError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ordering.ErrLoyaltyAccountNotFound):
		handlers.RespondError(w, http.StatusNotFound, err.Error())

	case errors.Is(err, ordering.ErrInsufficientLoyaltyPoints),
		errors.Is(err, ordering.ErrInvalidLoyaltyPoints):
		handlers.RespondError(w, http.StatusBadRequest, err.Error())

	case errors.Is(err, ordering.ErrUnauthorized):
		handlers.RespondError(w, http.StatusForbidden, err.Error())

	default:
		h.log.Error("internal error", zap.Error(err))
		handlers.RespondError(w, http.StatusInternalServerError, "internal server error")
	}
}

func getLoyaltyPagination(r *http.Request) (limit, offset, page int) {
	limit = 50
	page = 1

	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 100 {
			limit = parsed
		}
	}
	if p := r.URL.Query().Get("page"); p != "" {
		if parsed, err := strconv.Atoi(p); err == nil && parsed > 0 {
			page = parsed
		}
	}

	offset = (page - 1) * limit
	return
}

// --- Handlers ---

// GetBalance retrieves the user's loyalty points balance.
// @Summary Get loyalty balance
// @Description Retrieves the current user's loyalty points balance
// @Tags Loyalty
// @Produce json
// @Param Authorization header string true "Bearer token"
// @Param X-Tenant-ID header string true "Tenant ID"
// @Success 200 {object} BalanceResponse
// @Failure 400 {object} handlers.ErrorResponse
// @Failure 401 {object} handlers.ErrorResponse
// @Router /loyalty/balance [get]
func (h *LoyaltyHandler) GetBalance(w http.ResponseWriter, r *http.Request) {
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

	balance, err := h.loyaltySvc.GetBalancePreferPOS(r.Context(), tenantID, user.ID, user.Phone)
	if err != nil {
		h.handleError(w, err)
		return
	}

	handlers.RespondJSON(w, http.StatusOK, BalanceResponse{
		Balance:      balance,
		BalanceValue: h.loyaltySvc.CalculatePointsValue(balance),
		Currency:     ordering.DefaultCurrency,
	})
}

// GetAccount retrieves the user's loyalty account details.
// @Summary Get loyalty account
// @Description Retrieves the current user's loyalty account with tier information
// @Tags Loyalty
// @Produce json
// @Param Authorization header string true "Bearer token"
// @Param X-Tenant-ID header string true "Tenant ID"
// @Success 200 {object} AccountResponse
// @Failure 400 {object} handlers.ErrorResponse
// @Failure 401 {object} handlers.ErrorResponse
// @Router /loyalty/account [get]
func (h *LoyaltyHandler) GetAccount(w http.ResponseWriter, r *http.Request) {
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

	account, err := h.loyaltySvc.GetAccountPreferPOS(r.Context(), tenantID, user.ID, user.Phone)
	if err != nil {
		h.handleError(w, err)
		return
	}

	handlers.RespondJSON(w, http.StatusOK, AccountResponse{
		ID:             account.ID.String(),
		BalancePoints:  account.BalancePoints,
		BalanceValue:   h.loyaltySvc.CalculatePointsValue(account.BalancePoints),
		Tier:           string(account.Tier),
		LifetimePoints: account.LifetimePoints,
		TierBenefits:   h.loyaltySvc.GetTierBenefits(account.Tier),
		Currency:       ordering.DefaultCurrency,
	})
}

// GetTransactions retrieves the user's loyalty transaction history.
// @Summary Get loyalty transactions
// @Description Retrieves the current user's loyalty points transaction history
// @Tags Loyalty
// @Produce json
// @Param Authorization header string true "Bearer token"
// @Param X-Tenant-ID header string true "Tenant ID"
// @Param limit query integer false "Page size (default 50)"
// @Param page query integer false "Page number (default 1)"
// @Success 200 {object} TransactionsResponse
// @Failure 400 {object} handlers.ErrorResponse
// @Failure 401 {object} handlers.ErrorResponse
// @Router /loyalty/transactions [get]
func (h *LoyaltyHandler) GetTransactions(w http.ResponseWriter, r *http.Request) {
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

	limit, offset, page := getLoyaltyPagination(r)

	transactions, total, err := h.loyaltySvc.GetTransactions(r.Context(), tenantID, user.ID, limit, offset)
	if err != nil {
		h.handleError(w, err)
		return
	}

	handlers.RespondJSON(w, http.StatusOK, TransactionsResponse{
		Data:  transactions,
		Total: total,
		Limit: limit,
		Page:  page,
	})
}

// GetTierBenefits retrieves the benefits for the user's current tier.
// @Summary Get tier benefits
// @Description Retrieves the benefits associated with the user's loyalty tier
// @Tags Loyalty
// @Produce json
// @Param Authorization header string true "Bearer token"
// @Param X-Tenant-ID header string true "Tenant ID"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} handlers.ErrorResponse
// @Failure 401 {object} handlers.ErrorResponse
// @Router /loyalty/tier-benefits [get]
func (h *LoyaltyHandler) GetTierBenefits(w http.ResponseWriter, r *http.Request) {
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

	account, err := h.loyaltySvc.GetOrCreateAccount(r.Context(), tenantID, user.ID)
	if err != nil {
		h.handleError(w, err)
		return
	}

	benefits := h.loyaltySvc.GetTierBenefits(account.Tier)
	benefits["tier"] = string(account.Tier)
	benefits["lifetimePoints"] = account.LifetimePoints

	// Add tier progression info
	tierThresholds := map[string]int{
		"bronze":   0,
		"silver":   1000,
		"gold":     5000,
		"platinum": 10000,
	}
	benefits["tierThresholds"] = tierThresholds

	handlers.RespondJSON(w, http.StatusOK, benefits)
}
