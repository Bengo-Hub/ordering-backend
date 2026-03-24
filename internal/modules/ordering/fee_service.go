package ordering

import (
	"context"
	"encoding/json"
	"math"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// FeeBreakdown is the detailed fee breakdown for an order or cart.
type FeeBreakdown struct {
	ItemTotal        float64 `json:"item_total"`
	Discount         float64 `json:"discount"`
	PackagingFee     float64 `json:"packaging_fee"`
	Subtotal         float64 `json:"subtotal"`
	SmallOrderFee    float64 `json:"small_order_fee"`
	ServiceFee       float64 `json:"service_fee"`
	DeliveryFee      float64 `json:"delivery_fee"`
	DeliveryDiscount float64 `json:"delivery_discount"`
	TaxTotal         float64 `json:"tax_total"`
	GrandTotal       float64 `json:"grand_total"`
}

// FeeConfig holds per-tenant fee configuration stored in TenantSetting.features JSON.
type FeeConfig struct {
	ServiceFeePercent   float64 `json:"service_fee_percent"`
	PackagingFeeFlat    float64 `json:"packaging_fee_flat"`
	SmallOrderFee       float64 `json:"small_order_fee"`
	SmallOrderThreshold float64 `json:"small_order_threshold"`
	DeliveryDiscountPct float64 `json:"delivery_discount_pct"`
}

// DefaultFeeConfig returns zero defaults when no tenant config is found.
// Fees are only charged when the tenant explicitly configures them in TenantSetting.features.
func DefaultFeeConfig() FeeConfig {
	return FeeConfig{
		ServiceFeePercent:   0, // 0% — tenant must configure
		PackagingFeeFlat:    0, // 0 — tenant must configure
		SmallOrderFee:       0, // 0 — tenant must configure
		SmallOrderThreshold: 0, // 0 — no threshold by default
		DeliveryDiscountPct: 0, // 0 — no delivery discount by default
	}
}

// FeeService calculates order fees based on per-tenant configuration.
type FeeService struct {
	repo   Repository
	logger *zap.Logger
}

// NewFeeService creates a new FeeService.
func NewFeeService(repo Repository, logger *zap.Logger) *FeeService {
	return &FeeService{
		repo:   repo,
		logger: logger.Named("FeeService"),
	}
}

// loadFeeConfig retrieves the FeeConfig from TenantSetting.features for the
// given tenant. If no config is found, sensible defaults are returned.
func (s *FeeService) loadFeeConfig(ctx context.Context, tenantID uuid.UUID) FeeConfig {
	featuresMap, err := s.repo.GetTenantFeatures(ctx, tenantID)
	if err != nil {
		s.logger.Debug("no tenant features found, using defaults", zap.Error(err))
		return DefaultFeeConfig()
	}

	cfg := DefaultFeeConfig()

	feeRaw, ok := featuresMap["fee_config"]
	if !ok {
		return cfg
	}

	// fee_config may be stored as a nested map; re-marshal and unmarshal to FeeConfig.
	b, err := json.Marshal(feeRaw)
	if err != nil {
		s.logger.Warn("failed to marshal fee_config", zap.Error(err))
		return cfg
	}
	if err := json.Unmarshal(b, &cfg); err != nil {
		s.logger.Warn("failed to unmarshal fee_config", zap.Error(err))
		return DefaultFeeConfig()
	}

	return cfg
}

// CalculateFees computes the full fee breakdown for an order.
func (s *FeeService) CalculateFees(
	ctx context.Context,
	tenantID uuid.UUID,
	cart *Cart,
	fulfillmentType FulfillmentType,
	discountTotal float64,
	loyaltyDiscount float64,
) (*FeeBreakdown, error) {
	cfg := s.loadFeeConfig(ctx, tenantID)

	itemTotal := cart.Subtotal
	discount := discountTotal + loyaltyDiscount
	packagingFee := cfg.PackagingFeeFlat
	subtotal := itemTotal - discount + packagingFee
	if subtotal < 0 {
		subtotal = 0
	}

	var smallOrderFee float64
	if subtotal < cfg.SmallOrderThreshold {
		smallOrderFee = cfg.SmallOrderFee
	}

	serviceFee := round2(subtotal * cfg.ServiceFeePercent)

	deliveryFee := cart.DeliveryFee
	if fulfillmentType == FulfillmentTypePickup {
		deliveryFee = 0
	}

	deliveryDiscount := round2(deliveryFee * cfg.DeliveryDiscountPct)

	taxTotal := cart.TaxTotal

	grandTotal := round2(subtotal + smallOrderFee + serviceFee + deliveryFee - deliveryDiscount + taxTotal)

	return &FeeBreakdown{
		ItemTotal:        itemTotal,
		Discount:         discount,
		PackagingFee:     packagingFee,
		Subtotal:         subtotal,
		SmallOrderFee:    smallOrderFee,
		ServiceFee:       serviceFee,
		DeliveryFee:      deliveryFee,
		DeliveryDiscount: deliveryDiscount,
		TaxTotal:         taxTotal,
		GrandTotal:       grandTotal,
	}, nil
}

// round2 rounds a float64 to 2 decimal places.
func round2(v float64) float64 {
	return math.Round(v*100) / 100
}
