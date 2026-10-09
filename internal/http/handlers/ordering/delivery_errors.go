package orderinghandler

import (
	"errors"
	"net/http"

	"github.com/bengobox/ordering-backend/internal/http/handlers"
	"github.com/bengobox/ordering-backend/internal/modules/ordering"
)

// respondDeliveryError maps delivery pricing errors to responses the checkout can act on.
// Returns false when err is not a delivery pricing error.
func respondDeliveryError(w http.ResponseWriter, err error) bool {
	var notServiceable *ordering.DeliveryNotServiceableError
	var belowMin *ordering.BelowDeliveryMinimumError
	switch {
	case errors.As(err, &notServiceable):
		handlers.RespondJSON(w, http.StatusUnprocessableEntity, map[string]any{
			"error":           notServiceable.Error(),
			"code":            "delivery_not_serviceable",
			"reason":          notServiceable.Reason,
			"nearest_area":    notServiceable.NearestArea,
			"nearest_area_km": notServiceable.NearestAreaKm,
		})
	case errors.As(err, &belowMin):
		handlers.RespondJSON(w, http.StatusUnprocessableEntity, map[string]any{
			"error":     belowMin.Error(),
			"code":      "below_delivery_minimum",
			"area":      belowMin.Area,
			"min_order": belowMin.MinOrder,
		})
	case errors.Is(err, ordering.ErrDeliveryNotServiceable):
		handlers.RespondJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": err.Error(), "code": "delivery_not_serviceable"})
	case errors.Is(err, ordering.ErrDeliveryLocationRequired):
		handlers.RespondJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": err.Error(), "code": "delivery_location_required"})
	case errors.Is(err, ordering.ErrDeliveryPricingUnavailable):
		handlers.RespondJSON(w, http.StatusServiceUnavailable, map[string]any{"error": err.Error(), "code": "delivery_pricing_unavailable"})
	default:
		handlers.RespondError(w, http.StatusInternalServerError, "failed to calculate fees")
		return false
	}
	return true
}

func isDeliveryPricingError(err error) bool {
	var a *ordering.DeliveryNotServiceableError
	var b *ordering.BelowDeliveryMinimumError
	return errors.As(err, &a) || errors.As(err, &b) ||
		errors.Is(err, ordering.ErrDeliveryNotServiceable) ||
		errors.Is(err, ordering.ErrDeliveryLocationRequired) ||
		errors.Is(err, ordering.ErrDeliveryPricingUnavailable)
}
