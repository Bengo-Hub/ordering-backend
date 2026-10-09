package ordering

import (
	"context"
	"encoding/json"
	"math"

	"github.com/google/uuid"

	"github.com/bengobox/ordering-backend/internal/platform/logistics"
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
	// DeliveryQuote is the logistics quote the delivery fee came from (delivery only).
	DeliveryQuote *logistics.DeliveryQuote `json:"delivery_quote,omitempty"`
}

// FeeConfigKey is the ServiceConfig key holding a tenant's ordering fees. The staff
// settings page writes it; delivery pricing itself belongs to logistics-api.
const FeeConfigKey = "fee_config"

// FeeConfig holds the fees ordering owns. Delivery fees come from the logistics quote;
// these only adjust an order around it. Every value defaults to off (0).
type FeeConfig struct {
	ServiceFeePercent   float64 `json:"service_fee_percent"`
	PackagingFeeFlat    float64 `json:"packaging_fee_flat"`
	SmallOrderFee       float64 `json:"small_order_fee"`
	SmallOrderThreshold float64 `json:"small_order_threshold"`
	// DeliveryDiscountPct discounts the quoted delivery fee (0.1 = 10% off).
	DeliveryDiscountPct float64 `json:"delivery_discount_pct"`
	// FreeDeliveryMinimum waives the delivery fee when the items total reaches it (0 = never).
	FreeDeliveryMinimum float64 `json:"free_delivery_minimum"`
}

// DefaultFeeConfig returns defaults when no tenant config is found: no extra fees.
func DefaultFeeConfig() FeeConfig { return FeeConfig{} }

// FeeService calculates order fees based on per-tenant configuration.
type FeeService struct {
	repo   Repository
	logger *zap.Logger
	// quoter prices deliveries through logistics-api.
	quoter DeliveryQuoter
}

// NewFeeService creates a new FeeService.
func NewFeeService(repo Repository, logger *zap.Logger) *FeeService {
	return &FeeService{
		repo:   repo,
		logger: logger.Named("FeeService"),
	}
}

// LoadFeeConfig reads the tenant's fee_config (or the platform default) from ServiceConfig.
func (s *FeeService) LoadFeeConfig(ctx context.Context, tenantID uuid.UUID) FeeConfig {
	cfg := DefaultFeeConfig()
	raw, found, err := s.repo.GetServiceConfigValue(ctx, tenantID, FeeConfigKey)
	if err != nil {
		s.logger.Warn("load fee_config failed, using defaults", zap.Error(err))
		return cfg
	}
	if !found || raw == "" {
		return cfg
	}
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		s.logger.Warn("fee_config is not valid JSON, using defaults", zap.Error(err))
		return DefaultFeeConfig()
	}
	return cfg
}

// CalculateFees computes the full fee breakdown for an order. The delivery part comes
// from PriceDelivery; this adds ordering's own fees (packaging, small order, service).
func (s *FeeService) CalculateFees(ctx context.Context, tenantID uuid.UUID, itemTotal, taxTotal float64, delivery DeliveryPrice, discountTotal float64) *FeeBreakdown {
	return computeFees(s.LoadFeeConfig(ctx, tenantID), itemTotal, taxTotal, delivery, discountTotal)
}

// computeFees is the pure fee arithmetic, for tests.
func computeFees(cfg FeeConfig, itemTotal, taxTotal float64, delivery DeliveryPrice, discountTotal float64) *FeeBreakdown {
	packagingFee := cfg.PackagingFeeFlat
	subtotal := itemTotal - discountTotal + packagingFee
	if subtotal < 0 {
		subtotal = 0
	}
	var smallOrderFee float64
	if cfg.SmallOrderThreshold > 0 && subtotal < cfg.SmallOrderThreshold {
		smallOrderFee = cfg.SmallOrderFee
	}
	serviceFee := round2(subtotal * cfg.ServiceFeePercent)
	var deliveryDiscount float64
	if delivery.Quote != nil && delivery.Quote.Fee > delivery.Fee {
		deliveryDiscount = round2(delivery.Quote.Fee - delivery.Fee)
	}
	return &FeeBreakdown{
		ItemTotal:        itemTotal,
		Discount:         discountTotal,
		PackagingFee:     packagingFee,
		Subtotal:         subtotal,
		SmallOrderFee:    smallOrderFee,
		ServiceFee:       serviceFee,
		DeliveryFee:      delivery.Fee,
		DeliveryDiscount: deliveryDiscount,
		TaxTotal:         taxTotal,
		GrandTotal:       round2(subtotal + smallOrderFee + serviceFee + delivery.Fee + taxTotal),
		DeliveryQuote:    delivery.Quote,
	}
}

// round2 rounds a float64 to 2 decimal places.
func round2(v float64) float64 {
	return math.Round(v*100) / 100
}
