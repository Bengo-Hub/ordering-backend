package ordering

import (
	"time"

	"github.com/google/uuid"
)

// CartStatus represents the status of a cart.
type CartStatus string

const (
	CartStatusActive     CartStatus = "active"
	CartStatusCheckedOut CartStatus = "checked_out"
	CartStatusAbandoned  CartStatus = "abandoned"
	CartStatusExpired    CartStatus = "expired"
)

// Cart represents a shopping cart.
type Cart struct {
	ID                    uuid.UUID  `json:"id"`
	TenantID              uuid.UUID  `json:"tenantId"`
	OutletID              uuid.UUID  `json:"outletId"`
	UserID                *uuid.UUID `json:"userId,omitempty"`
	SessionID             string     `json:"sessionId,omitempty"`
	Status                CartStatus `json:"status"`
	Currency              string     `json:"currency"`
	Subtotal              float64    `json:"subtotal"`
	DiscountTotal         float64    `json:"discountTotal"`
	TaxTotal              float64    `json:"taxTotal"`
	DeliveryFee           float64    `json:"deliveryFee"`
	LoyaltyPointsRedeemed int        `json:"loyaltyPointsRedeemed"`
	PromoCodeID           *uuid.UUID `json:"promoCodeId,omitempty"`
	Items                 []CartItem `json:"items,omitempty"`
	ExpiresAt             *time.Time `json:"expiresAt,omitempty"`
	CreatedAt             time.Time  `json:"createdAt"`
	UpdatedAt             time.Time  `json:"updatedAt"`
}

// CartItemModifier represents a selected modifier option on a cart item.
type CartItemModifier struct {
	GroupID         uuid.UUID `json:"group_id"`
	GroupName       string    `json:"group_name"`
	OptionID        uuid.UUID `json:"option_id"`
	OptionName      string    `json:"option_name"`
	PriceAdjustment float64  `json:"price_adjustment"`
}

// CartItem represents an item in a shopping cart.
type CartItem struct {
	ID            uuid.UUID              `json:"id"`
	CartID        uuid.UUID              `json:"cartId"`
	InventorySKU  string                 `json:"inventorySku"`
	VariantID     *uuid.UUID             `json:"variantId,omitempty"`
	NameSnapshot  string                 `json:"nameSnapshot"`
	Quantity      int                    `json:"quantity"`
	UnitPrice     float64                `json:"unitPrice"`
	Modifiers     []CartItemModifier     `json:"modifiers,omitempty"`
	ModifierTotal float64                `json:"modifierTotal"`
	TotalPrice    float64                `json:"totalPrice"`
	Notes         string                 `json:"notes,omitempty"`
	Metadata      map[string]interface{} `json:"metadata,omitempty"`
	CreatedAt     time.Time              `json:"createdAt"`
	UpdatedAt     time.Time              `json:"updatedAt"`
}

// OrderStatus represents the status of an order.
type OrderStatus string

const (
	OrderStatusPending        OrderStatus = "pending"
	OrderStatusConfirmed      OrderStatus = "confirmed"
	OrderStatusPreparing      OrderStatus = "preparing"
	OrderStatusReady          OrderStatus = "ready"
	OrderStatusOutForDelivery OrderStatus = "out_for_delivery"
	OrderStatusDelivered      OrderStatus = "delivered"
	OrderStatusCompleted      OrderStatus = "completed"
	OrderStatusCancelled      OrderStatus = "cancelled"
	OrderStatusRefunded       OrderStatus = "refunded"
)

// PaymentStatus represents the payment status of an order.
type PaymentStatus string

const (
	PaymentStatusPending           PaymentStatus = "pending"
	PaymentStatusAuthorized        PaymentStatus = "authorized"
	PaymentStatusPaid              PaymentStatus = "paid"
	PaymentStatusFailed            PaymentStatus = "failed"
	PaymentStatusRefunded          PaymentStatus = "refunded"
	PaymentStatusPartiallyRefunded PaymentStatus = "partially_refunded"
)

// OrderChannel represents the order source channel.
type OrderChannel string

const (
	OrderChannelWeb       OrderChannel = "web"
	OrderChannelMobileApp OrderChannel = "mobile_app"
	OrderChannelKiosk     OrderChannel = "kiosk"
	OrderChannelPhone     OrderChannel = "phone"
	OrderChannelAPI       OrderChannel = "api"
)

// FulfillmentType represents how the order is fulfilled.
type FulfillmentType string

const (
	FulfillmentTypeDelivery  FulfillmentType = "delivery"
	FulfillmentTypePickup    FulfillmentType = "pickup"
	FulfillmentTypeDineIn    FulfillmentType = "dine_in"
	FulfillmentTypeScheduled FulfillmentType = "scheduled"
)

// PaymentMethod represents the payment method for an order.
type PaymentMethod string

const (
	PaymentMethodMpesa   PaymentMethod = "mpesa"
	PaymentMethodPaystack PaymentMethod = "paystack"
	PaymentMethodStripe  PaymentMethod = "stripe"
	PaymentMethodCOD     PaymentMethod = "cod"
	PaymentMethodWallet  PaymentMethod = "wallet"
	PaymentMethodLoyalty PaymentMethod = "loyalty"
)

// Order represents a customer order.
type Order struct {
	ID                    uuid.UUID              `json:"id"`
	TenantID              uuid.UUID              `json:"tenantId"`
	OutletID              uuid.UUID              `json:"outletId"`
	CustomerID            *uuid.UUID             `json:"customerId,omitempty"`
	CartID                *uuid.UUID             `json:"cartId,omitempty"`
	OrderNumber           string                 `json:"orderNumber"`
	Status                OrderStatus            `json:"status"`
	PaymentStatus         PaymentStatus          `json:"paymentStatus"`
	PaymentMethod         PaymentMethod          `json:"paymentMethod"`
	FulfillmentType       FulfillmentType        `json:"fulfillmentType"`
	ScheduledFor          *time.Time             `json:"scheduledFor,omitempty"`
	Currency              string                 `json:"currency"`
	Subtotal              float64                `json:"subtotal"`
	DiscountTotal         float64                `json:"discountTotal"`
	TaxTotal              float64                `json:"taxTotal"`
	DeliveryFee           float64                `json:"deliveryFee"`
	PackagingFee          float64                `json:"packagingFee"`
	ServiceFee            float64                `json:"serviceFee"`
	SmallOrderFee         float64                `json:"smallOrderFee"`
	TipTotal              float64                `json:"tipTotal"`
	GrandTotal            float64                `json:"grandTotal"`
	ReservationID         *uuid.UUID             `json:"reservationId,omitempty"`
	LoyaltyPointsEarned   int                    `json:"loyaltyPointsEarned"`
	LoyaltyPointsRedeemed int                    `json:"loyaltyPointsRedeemed"`
	DeliveryAddressID     *uuid.UUID             `json:"deliveryAddressId,omitempty"`
	PromoCodeID           *uuid.UUID             `json:"promoCodeId,omitempty"`
	Instructions          string                 `json:"instructions,omitempty"`
	Channel               OrderChannel           `json:"channel"`
	Source                string                 `json:"source,omitempty"`
	IdempotencyKey        string                 `json:"idempotencyKey,omitempty"`
	Items                 []OrderItem            `json:"items,omitempty"`
	Events                []OrderEvent           `json:"events,omitempty"`
	DeliveryAddress       *CustomerAddress       `json:"deliveryAddress,omitempty"`
	PlacedAt              *time.Time             `json:"placedAt,omitempty"`
	ConfirmedAt           *time.Time             `json:"confirmedAt,omitempty"`
	ReadyAt               *time.Time             `json:"readyAt,omitempty"`
	DeliveredAt           *time.Time             `json:"deliveredAt,omitempty"`
	CompletedAt           *time.Time             `json:"completedAt,omitempty"`
	CancelledAt           *time.Time             `json:"cancelledAt,omitempty"`
	CancellationReason    string                 `json:"cancellationReason,omitempty"`
	Rating                *int                   `json:"rating,omitempty"`
	RatingComment         string                 `json:"ratingComment,omitempty"`
	RatedAt               *time.Time             `json:"ratedAt,omitempty"`
	Metadata              map[string]interface{} `json:"metadata,omitempty"`
	CreatedAt             time.Time              `json:"createdAt"`
	UpdatedAt             time.Time              `json:"updatedAt"`
}

// OrderItem represents an item in an order.
type OrderItem struct {
	ID           uuid.UUID              `json:"id"`
	OrderID      uuid.UUID              `json:"orderId"`
	InventorySKU string                 `json:"inventorySku"`
	VariantID    *uuid.UUID             `json:"variantId,omitempty"`
	NameSnapshot string                 `json:"nameSnapshot"`
	Quantity     int                    `json:"quantity"`
	UnitPrice    float64                `json:"unitPrice"`
	TotalPrice   float64                `json:"totalPrice"`
	Notes        string                 `json:"notes,omitempty"`
	Metadata     map[string]interface{} `json:"metadata,omitempty"`
}

// OrderEvent represents an event in the order lifecycle.
type OrderEvent struct {
	ID          uuid.UUID              `json:"id"`
	OrderID     uuid.UUID              `json:"orderId"`
	EventType   string                 `json:"eventType"`
	FromStatus  string                 `json:"fromStatus,omitempty"`
	ToStatus    string                 `json:"toStatus,omitempty"`
	Payload     map[string]interface{} `json:"payload,omitempty"`
	ActorUserID *uuid.UUID             `json:"actorUserId,omitempty"`
	ActorType   string                 `json:"actorType,omitempty"`
	IPAddress   string                 `json:"ipAddress,omitempty"`
	OccurredAt  time.Time              `json:"occurredAt"`
}

// CustomerAddress represents a customer's delivery address.
type CustomerAddress struct {
	ID           uuid.UUID `json:"id"`
	TenantID     uuid.UUID `json:"tenantId"`
	UserID       uuid.UUID `json:"userId"`
	Label        string    `json:"label"`
	AddressLine1 string    `json:"addressLine1"`
	AddressLine2 string    `json:"addressLine2,omitempty"`
	City         string    `json:"city"`
	County       string    `json:"county,omitempty"`
	PostalCode   string    `json:"postalCode,omitempty"`
	Country      string    `json:"country"`
	Latitude     *float64  `json:"latitude,omitempty"`
	Longitude    *float64  `json:"longitude,omitempty"`
	PlusCode     string    `json:"plusCode,omitempty"`
	ContactName  string    `json:"contactName,omitempty"`
	ContactPhone string    `json:"contactPhone,omitempty"`
	Instructions string    `json:"instructions,omitempty"`
	IsVerified   bool      `json:"isVerified"`
	IsDefault          bool       `json:"isDefault"`
	CreatedAt          time.Time  `json:"createdAt"`
	UpdatedAt          time.Time  `json:"updatedAt"`
}

// Tenant represents a minimalist view of a tenant.
type Tenant struct {
	ID   uuid.UUID `json:"id"`
	Slug string    `json:"slug"`
	Name string    `json:"name"`
}

// PromoCodeType represents the type of discount.
type PromoCodeType string

const (
	PromoCodeTypePercentage   PromoCodeType = "percentage"
	PromoCodeTypeFixedAmount  PromoCodeType = "fixed_amount"
	PromoCodeTypeFreeDelivery PromoCodeType = "free_delivery"
	PromoCodeTypeFreeItem     PromoCodeType = "free_item"
)

// PromoCode represents a promotional discount code.
type PromoCode struct {
	ID               uuid.UUID              `json:"id"`
	TenantID         uuid.UUID              `json:"tenantId"`
	OutletID         *uuid.UUID             `json:"outletId,omitempty"`
	Code             string                 `json:"code"`
	Name             string                 `json:"name"`
	Description      string                 `json:"description,omitempty"`
	DiscountType     PromoCodeType          `json:"discountType"`
	DiscountValue    float64                `json:"discountValue"`
	MaxDiscountAmount *float64              `json:"maxDiscountAmount,omitempty"`
	MinSubtotal      float64                `json:"minSubtotal"`
	MaxUses          *int                   `json:"maxUses,omitempty"`
	MaxUsesPerUser   *int                   `json:"maxUsesPerUser,omitempty"`
	UsageCount       int                    `json:"usageCount"`
	IsActive         bool                   `json:"isActive"`
	FirstOrderOnly   bool                   `json:"firstOrderOnly"`
	StartsAt         *time.Time             `json:"startsAt,omitempty"`
	EndsAt           *time.Time             `json:"endsAt,omitempty"`
	EligibleCategories []uuid.UUID          `json:"eligibleCategories,omitempty"`
	EligibleItems    []uuid.UUID            `json:"eligibleItems,omitempty"`
	Metadata         map[string]interface{} `json:"metadata,omitempty"`
	CreatedAt        time.Time              `json:"createdAt"`
	UpdatedAt        time.Time              `json:"updatedAt"`
}

// PromoRedemption represents the redemption of a promo code.
type PromoRedemption struct {
	ID             uuid.UUID `json:"id"`
	PromoCodeID    uuid.UUID `json:"promoCodeId"`
	OrderID        uuid.UUID `json:"orderId"`
	UserID         uuid.UUID `json:"userId"`
	DiscountAmount float64   `json:"discountAmount"`
	RedeemedAt     time.Time `json:"redeemedAt"`
}

// LoyaltyTier represents a loyalty program tier.
type LoyaltyTier string

const (
	LoyaltyTierBronze   LoyaltyTier = "bronze"
	LoyaltyTierSilver   LoyaltyTier = "silver"
	LoyaltyTierGold     LoyaltyTier = "gold"
	LoyaltyTierPlatinum LoyaltyTier = "platinum"
)

// LoyaltyAccount represents a customer's loyalty account.
type LoyaltyAccount struct {
	ID             uuid.UUID   `json:"id"`
	TenantID       uuid.UUID   `json:"tenantId"`
	UserID         uuid.UUID   `json:"userId"`
	BalancePoints  int         `json:"balancePoints"`
	Tier           LoyaltyTier `json:"tier"`
	LifetimePoints int         `json:"lifetimePoints"`
	CreatedAt      time.Time   `json:"createdAt"`
	UpdatedAt      time.Time   `json:"updatedAt"`
}

// LoyaltyTransactionType represents the type of loyalty transaction.
type LoyaltyTransactionType string

const (
	LoyaltyTransactionTypeEarn       LoyaltyTransactionType = "earn"
	LoyaltyTransactionTypeRedeem     LoyaltyTransactionType = "redeem"
	LoyaltyTransactionTypeExpire     LoyaltyTransactionType = "expire"
	LoyaltyTransactionTypeAdjustment LoyaltyTransactionType = "adjustment"
	LoyaltyTransactionTypeBonus      LoyaltyTransactionType = "bonus"
)

// LoyaltyTransaction represents a loyalty points transaction.
type LoyaltyTransaction struct {
	ID              uuid.UUID              `json:"id"`
	AccountID       uuid.UUID              `json:"accountId"`
	OrderID         *uuid.UUID             `json:"orderId,omitempty"`
	Points          int                    `json:"points"`
	TransactionType LoyaltyTransactionType `json:"transactionType"`
	Description     string                 `json:"description,omitempty"`
	OccurredAt      time.Time              `json:"occurredAt"`
	Metadata        map[string]interface{} `json:"metadata,omitempty"`
}

// DeliveryZone represents a delivery zone with its fee configuration.
type DeliveryZone struct {
	ID                   uuid.UUID              `json:"id"`
	TenantID             uuid.UUID              `json:"tenantId"`
	OutletID             *uuid.UUID             `json:"outletId,omitempty"`
	Name                 string                 `json:"name"`
	Slug                 string                 `json:"slug,omitempty"`
	ZonePolygon          map[string]interface{} `json:"zonePolygon,omitempty"`
	DeliveryFee          float64                `json:"deliveryFee"`
	MinimumOrder         float64                `json:"minimumOrder"`
	EstimatedTimeMinutes int                    `json:"estimatedTimeMinutes"`
	IsActive             bool                   `json:"isActive"`
	SortOrder            int                    `json:"sortOrder"`
}

// OutletRatingData represents the materialized rating aggregate for an outlet.
type OutletRatingData struct {
	ID            uuid.UUID `json:"id"`
	TenantID      uuid.UUID `json:"tenantId"`
	OutletID      uuid.UUID `json:"outletId"`
	AverageRating float64   `json:"averageRating"`
	TotalRatings  int       `json:"totalRatings"`
	TotalReviews  int       `json:"totalReviews"`
	FiveStar      int       `json:"fiveStar"`
	FourStar      int       `json:"fourStar"`
	ThreeStar     int       `json:"threeStar"`
	TwoStar       int       `json:"twoStar"`
	OneStar       int       `json:"oneStar"`
	CreatedAt     time.Time `json:"createdAt"`
	UpdatedAt     time.Time `json:"updatedAt"`
}

// RateOrderRequest represents a request to rate an order.
type RateOrderRequest struct {
	Rating  int    `json:"rating" validate:"required,min=1,max=5"`
	Comment string `json:"comment,omitempty"`
}

// GroupOrderStatus represents the status of a group order session.
type GroupOrderStatus string

const (
	GroupOrderStatusOpen       GroupOrderStatus = "open"
	GroupOrderStatusLocked     GroupOrderStatus = "locked"
	GroupOrderStatusCheckedOut GroupOrderStatus = "checked_out"
	GroupOrderStatusExpired    GroupOrderStatus = "expired"
)

// GroupOrder represents a collaborative ordering session.
type GroupOrder struct {
	ID              uuid.UUID          `json:"id"`
	TenantID        uuid.UUID          `json:"tenantId"`
	HostUserID      uuid.UUID          `json:"hostUserId"`
	CartID          uuid.UUID          `json:"cartId"`
	InviteCode      string             `json:"inviteCode"`
	Status          GroupOrderStatus   `json:"status"`
	MaxParticipants int                `json:"maxParticipants"`
	Participants    []GroupParticipant  `json:"participants,omitempty"`
	ExpiresAt       time.Time          `json:"expiresAt"`
	CreatedAt       time.Time          `json:"createdAt"`
	UpdatedAt       time.Time          `json:"updatedAt"`
}

// GroupParticipant represents a participant in a group order.
type GroupParticipant struct {
	ID           uuid.UUID `json:"id"`
	GroupOrderID uuid.UUID `json:"groupOrderId"`
	UserID       uuid.UUID `json:"userId"`
	UserName     string    `json:"userName"`
	JoinedAt     time.Time `json:"joinedAt"`
}

// ItemImage represents an image in a multi-image gallery for a catalog item.
type ItemImage struct {
	URL          string `json:"url"`
	Type         string `json:"type"`
	IsPrimary    bool   `json:"is_primary"`
	DisplayOrder int    `json:"display_order"`
}

// DefaultCurrency is the default currency for orders.
const DefaultCurrency = "KES"

// CartExpirationDuration is the default cart expiration duration.
const CartExpirationDuration = 24 * time.Hour

// LoyaltyPointsPerHundred is the base points earned per KES 100 spent (Bronze tier).
const LoyaltyPointsPerHundred = 1 // 1 point per KES 100

// LoyaltyPointValue is the value of each loyalty point in the default currency.
const LoyaltyPointValue = 0.1 // 1 point = 0.1 KES

// Delivery fee constants
const (
	DeliveryFeeBase     = 100.0  // KES base delivery fee
	DeliveryFeePerKm    = 30.0   // KES per km
	FreeDeliveryMinimum = 2000.0 // KES - free delivery for orders above this
)

// Scheduled order constants
const (
	ScheduledMinLeadTime    = 30 * time.Minute // Minimum lead time for scheduled orders
	ScheduledPrepTimeBuffer = 30 * time.Minute // How far ahead to start preparing scheduled orders
)

// CartFilter defines filter options for listing carts.
type CartFilter struct {
	TenantID  uuid.UUID
	OutletID  *uuid.UUID
	UserID    *uuid.UUID
	SessionID string
	Status    *CartStatus
	Limit     int
	Offset    int
}

// OrderFilter defines filter options for listing orders.
type OrderFilter struct {
	TenantID      uuid.UUID
	OutletID      *uuid.UUID
	CustomerID    *uuid.UUID
	Status        *OrderStatus
	PaymentStatus *PaymentStatus
	DateFrom      *time.Time
	DateTo        *time.Time
	Search        string
	Limit         int
	Offset        int
}

// AddItemRequest represents a request to add an item to cart.
type AddItemRequest struct {
	TenantID     uuid.UUID
	OutletID     uuid.UUID
	UserID       *uuid.UUID
	SessionID    string
	InventorySKU string
	VariantID    *uuid.UUID
	Quantity     int
	Notes        string
	Modifiers    []CartItemModifier // Selected modifier options
}

// UpdateItemRequest represents a request to update a cart item.
type UpdateItemRequest struct {
	TenantID uuid.UUID
	CartID   uuid.UUID
	ItemID   uuid.UUID
	Quantity *int
	Notes    *string
}

// CheckoutRequest represents a request to checkout a cart.
type CheckoutRequest struct {
	TenantID              uuid.UUID
	CartID                uuid.UUID
	UserID                uuid.UUID
	DeliveryAddressID     *uuid.UUID
	PromoCode             string
	LoyaltyPointsRedeemed int
	Instructions          string
	Channel               OrderChannel
	IdempotencyKey        string
	FulfillmentType       FulfillmentType
	ScheduledFor          *time.Time
	PaymentMethod         PaymentMethod
}

// CreateOrderFromItemsRequest is the request to create an order directly from a list of items (frontend contract).
type CreateOrderFromItemsRequest struct {
	TenantID        uuid.UUID
	OutletID        uuid.UUID
	UserID          uuid.UUID
	Items           []CreateOrderItemInput
	DeliveryAddress string
	DeliveryLat     *float64
	DeliveryLng     *float64
	DeliveryNotes   string
	PaymentMethod   string // "mpesa" | "cod"
	PromoCode       string
	Channel         OrderChannel
	FulfillmentType FulfillmentType
	ScheduledFor    *time.Time
}

// CreateOrderItemInput is a single line item when creating an order from items.
type CreateOrderItemInput struct {
	InventorySKU string
	Name         string
	Quantity     int
	UnitPrice    float64
	TotalPrice   float64
}

// GuestCheckoutRequest represents a request for guest checkout (no auth required).
type GuestCheckoutRequest struct {
	TenantID        uuid.UUID
	OutletID        uuid.UUID
	SessionID       string
	ContactEmail    string
	ContactPhone    string
	ContactName     string
	Items           []CreateOrderItemInput // items from frontend local cart
	DeliveryAddress string
	DeliveryLat     *float64
	DeliveryLng     *float64
	DeliveryNotes   string
	PaymentMethod   string
	Instructions    string
	Channel         OrderChannel
	FulfillmentType FulfillmentType
}

// RefundOrderRequest represents a request to refund an order.
type RefundOrderRequest struct {
	TenantID uuid.UUID
	OrderID  uuid.UUID
	Reason   string
	ActorID  *uuid.UUID
}

// PromoValidationResult represents the result of promo code validation.
type PromoValidationResult struct {
	Valid          bool          `json:"valid"`
	PromoCodeID    *uuid.UUID    `json:"promoCodeId,omitempty"`
	DiscountType   PromoCodeType `json:"discountType,omitempty"`
	DiscountValue  float64       `json:"discountValue,omitempty"`
	DiscountAmount float64       `json:"discountAmount,omitempty"`
	ErrorMessage   string        `json:"errorMessage,omitempty"`
}

// CartSummary represents a cart summary with calculated totals.
type CartSummary struct {
	Subtotal              float64 `json:"subtotal"`
	DiscountTotal         float64 `json:"discountTotal"`
	TaxTotal              float64 `json:"taxTotal"`
	DeliveryFee           float64 `json:"deliveryFee"`
	LoyaltyPointsRedeemed int     `json:"loyaltyPointsRedeemed"`
	LoyaltyDiscount       float64 `json:"loyaltyDiscount"`
	GrandTotal            float64 `json:"grandTotal"`
}

// AnalyticsSummary represents high-level metrics for the dashboard.
type AnalyticsSummary struct {
	TotalRevenue      float64            `json:"totalRevenue"`
	TotalOrders       int                `json:"totalOrders"`
	CancelledOrders    int                `json:"cancelledOrders"`
	OrdersByStatus    map[string]int     `json:"ordersByStatus"`
	RevenueByCurrency map[string]float64 `json:"revenueByCurrency"`
	TopSellingItems   []ItemSalesSummary `json:"topSellingItems"`
	Trend             []DailyMetric      `json:"trend"`
}

// ItemSalesSummary represents sales metrics for a single item.
type ItemSalesSummary struct {
	InventorySKU string  `json:"inventorySku"`
	NameSnapshot string  `json:"nameSnapshot"`
	Quantity     int     `json:"quantity"`
	Revenue      float64 `json:"revenue"`
}

// DailyMetric represents a metric for a specific day.
type DailyMetric struct {
	Date    string  `json:"date"`
	Orders  int     `json:"orders"`
	Revenue float64 `json:"revenue"`
}
