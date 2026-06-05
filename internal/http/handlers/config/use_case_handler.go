package config

import (
	"net/http"

	httpware "github.com/Bengo-Hub/httpware"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/bengobox/ordering-backend/internal/ent"
	"github.com/bengobox/ordering-backend/internal/ent/outlet"
	"github.com/bengobox/ordering-backend/internal/ent/tenant"
	"github.com/bengobox/ordering-backend/internal/http/handlers"
	tenantmod "github.com/bengobox/ordering-backend/internal/modules/tenant"
)

// UseCaseHandler exposes a read-only view of the tenant's business use-case
// configuration: the tenant's primary use_case, the set of use-cases ordering
// supports, and each outlet's individual use_case.
type UseCaseHandler struct {
	client *ent.Client
	logger *zap.Logger
}

// NewUseCaseHandler creates a new UseCaseHandler.
func NewUseCaseHandler(client *ent.Client, logger *zap.Logger) *UseCaseHandler {
	return &UseCaseHandler{
		client: client,
		logger: logger.Named("use-case"),
	}
}

// useCaseOutlet is one outlet entry in the use-case config response.
type useCaseOutlet struct {
	ID      uuid.UUID `json:"id"`
	Name    string    `json:"name"`
	UseCase string    `json:"use_case"`
}

// useCaseConfigResponse is the response shape for GetUseCaseConfig.
type useCaseConfigResponse struct {
	UseCase            string          `json:"use_case"`
	AvailableUseCases  []string        `json:"available_use_cases"`
	Outlets            []useCaseOutlet `json:"outlets"`
}

// GetUseCaseConfig returns the tenant's primary use_case, the available
// use-case values ordering supports, and the per-outlet use_case mapping.
//
// @Summary Get use-case configuration
// @Description Returns the tenant's primary business use_case, the available use-case values ordering supports, and each outlet's use_case. Read-only.
// @Tags Configuration
// @Produce json
// @Param tenant path string true "Tenant slug or ID"
// @Success 200 {object} useCaseConfigResponse
// @Security BearerAuth
// @Router /admin/use-case [get]
func (h *UseCaseHandler) GetUseCaseConfig(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Available use-cases derive from the single source of truth in the tenant module.
	available := append([]string{}, tenantmod.OrderingAcceptedUseCases...)

	resp := useCaseConfigResponse{
		UseCase:           "",
		AvailableUseCases: available,
		Outlets:           []useCaseOutlet{},
	}

	// Resolve the tenant UUID. The {tenant} path param is the tenant ID for
	// authenticated admin routes; fall back to the context-resolved tenant ID.
	tenantID, ok := resolveTenantID(r)
	if !ok {
		// No resolvable tenant — return empty config gracefully.
		handlers.RespondJSON(w, http.StatusOK, resp)
		return
	}

	// Tenant primary use_case (nillable field).
	if t, err := h.client.Tenant.Query().Where(tenant.ID(tenantID)).Only(ctx); err == nil {
		if t.UseCase != nil {
			resp.UseCase = *t.UseCase
		}
	} else if !ent.IsNotFound(err) {
		h.logger.Error("failed to load tenant use_case", zap.Error(err))
		handlers.RespondError(w, http.StatusInternalServerError, "failed to load use-case config")
		return
	}

	// Outlets for the tenant with their individual use_case.
	outlets, err := h.client.Outlet.Query().
		Where(outlet.TenantID(tenantID)).
		Order(ent.Asc(outlet.FieldName)).
		All(ctx)
	if err != nil {
		h.logger.Error("failed to load outlets", zap.Error(err))
		handlers.RespondError(w, http.StatusInternalServerError, "failed to load use-case config")
		return
	}

	rows := make([]useCaseOutlet, 0, len(outlets))
	for _, o := range outlets {
		rows = append(rows, useCaseOutlet{
			ID:      o.ID,
			Name:    o.Name,
			UseCase: o.UseCase,
		})
	}
	resp.Outlets = rows

	// Fall back to the most common outlet use_case when the tenant has no
	// primary use_case recorded locally (auth-api is the SoT and may not have synced it).
	if resp.UseCase == "" {
		resp.UseCase = primaryFromOutlets(rows)
	}

	handlers.RespondJSON(w, http.StatusOK, resp)
}

// resolveTenantID extracts the tenant UUID from the {tenant} path param, falling
// back to the context tenant ID set by the TenantV2 middleware.
func resolveTenantID(r *http.Request) (uuid.UUID, bool) {
	if raw := chi.URLParam(r, "tenant"); raw != "" {
		if id, err := uuid.Parse(raw); err == nil {
			return id, true
		}
	}
	if raw := httpware.GetTenantID(r.Context()); raw != "" {
		if id, err := uuid.Parse(raw); err == nil {
			return id, true
		}
	}
	return uuid.Nil, false
}

// primaryFromOutlets returns the most frequent non-empty outlet use_case, or
// empty string when none of the outlets carry a use_case.
func primaryFromOutlets(rows []useCaseOutlet) string {
	counts := make(map[string]int)
	for _, row := range rows {
		if row.UseCase != "" {
			counts[row.UseCase]++
		}
	}
	best := ""
	bestN := 0
	for uc, n := range counts {
		if n > bestN {
			best, bestN = uc, n
		}
	}
	return best
}
