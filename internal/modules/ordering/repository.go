package ordering

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// Repository defines the interface for ordering data persistence.
type Repository interface {
	// Cart operations
	CreateCart(ctx context.Context, cart *Cart) error
	GetCart(ctx context.Context, tenantID, cartID uuid.UUID) (*Cart, error)
	GetActiveCartByUser(ctx context.Context, tenantID, outletID, userID uuid.UUID) (*Cart, error)
	GetActiveCartBySession(ctx context.Context, tenantID, outletID uuid.UUID, sessionID string) (*Cart, error)
	UpdateCart(ctx context.Context, cart *Cart) error
	DeleteCart(ctx context.Context, tenantID, cartID uuid.UUID) error
	ListCarts(ctx context.Context, filter CartFilter) ([]Cart, int, error)
	ExpireOldCarts(ctx context.Context, tenantID uuid.UUID) (int, error)

	// CartItem operations
	CreateCartItem(ctx context.Context, item *CartItem) error
	GetCartItem(ctx context.Context, cartID, itemID uuid.UUID) (*CartItem, error)
	GetCartItemBySKU(ctx context.Context, cartID uuid.UUID, sku string, variantID *uuid.UUID) (*CartItem, error)
	UpdateCartItem(ctx context.Context, item *CartItem) error
	DeleteCartItem(ctx context.Context, cartID, itemID uuid.UUID) error
	ListCartItems(ctx context.Context, cartID uuid.UUID) ([]CartItem, error)
	ClearCartItems(ctx context.Context, cartID uuid.UUID) error

	// Order operations
	CreateOrder(ctx context.Context, order *Order) error
	GetOrder(ctx context.Context, tenantID, orderID uuid.UUID) (*Order, error)
	GetOrderByNumber(ctx context.Context, tenantID uuid.UUID, orderNumber string) (*Order, error)
	GetOrderByIdempotencyKey(ctx context.Context, tenantID uuid.UUID, key string) (*Order, error)
	UpdateOrder(ctx context.Context, order *Order) error
	ListOrders(ctx context.Context, filter OrderFilter) ([]Order, int, error)
	GenerateOrderNumber(ctx context.Context, tenantID, outletID uuid.UUID) (string, error)
	GetAnalyticsSummary(ctx context.Context, tenantID uuid.UUID, dateFrom, dateTo time.Time) (*AnalyticsSummary, error)

	// Scheduled order operations
	ListScheduledOrdersDue(ctx context.Context, prepBuffer time.Duration) ([]Order, error)

	// OrderItem operations
	CreateOrderItem(ctx context.Context, item *OrderItem) error
	GetOrderItem(ctx context.Context, orderID, itemID uuid.UUID) (*OrderItem, error)
	ListOrderItems(ctx context.Context, orderID uuid.UUID) ([]OrderItem, error)

	// OrderEvent operations
	CreateOrderEvent(ctx context.Context, event *OrderEvent) error
	ListOrderEvents(ctx context.Context, orderID uuid.UUID) ([]OrderEvent, error)

	// CustomerAddress operations
	CreateAddress(ctx context.Context, address *CustomerAddress) error
	GetAddress(ctx context.Context, tenantID, addressID uuid.UUID) (*CustomerAddress, error)
	GetDefaultAddress(ctx context.Context, tenantID, userID uuid.UUID) (*CustomerAddress, error)
	UpdateAddress(ctx context.Context, address *CustomerAddress) error
	DeleteAddress(ctx context.Context, tenantID, addressID uuid.UUID) error
	ListAddresses(ctx context.Context, tenantID, userID uuid.UUID) ([]CustomerAddress, error)
	SetDefaultAddress(ctx context.Context, tenantID, userID, addressID uuid.UUID) error
	CountUserAddresses(ctx context.Context, tenantID, userID uuid.UUID) (int, error)

	// PromoCode operations
	CreatePromoCode(ctx context.Context, promo *PromoCode) error
	GetPromoCode(ctx context.Context, tenantID, promoID uuid.UUID) (*PromoCode, error)
	GetPromoCodeByCode(ctx context.Context, tenantID uuid.UUID, code string) (*PromoCode, error)
	UpdatePromoCode(ctx context.Context, promo *PromoCode) error
	DeletePromoCode(ctx context.Context, tenantID, promoID uuid.UUID) error
	ListPromoCodes(ctx context.Context, tenantID uuid.UUID, isActive *bool) ([]PromoCode, error)
	IncrementPromoUsage(ctx context.Context, promoID uuid.UUID) error

	// PromoRedemption operations
	CreatePromoRedemption(ctx context.Context, redemption *PromoRedemption) error
	GetPromoRedemption(ctx context.Context, redemptionID uuid.UUID) (*PromoRedemption, error)
	CountUserPromoRedemptions(ctx context.Context, promoID, userID uuid.UUID) (int, error)
	ListPromoRedemptions(ctx context.Context, promoID uuid.UUID) ([]PromoRedemption, error)

	// LoyaltyAccount operations
	CreateLoyaltyAccount(ctx context.Context, account *LoyaltyAccount) error
	GetLoyaltyAccount(ctx context.Context, tenantID, accountID uuid.UUID) (*LoyaltyAccount, error)
	GetLoyaltyAccountByUser(ctx context.Context, tenantID, userID uuid.UUID) (*LoyaltyAccount, error)
	UpdateLoyaltyAccount(ctx context.Context, account *LoyaltyAccount) error
	AddLoyaltyPoints(ctx context.Context, accountID uuid.UUID, points int) error
	DeductLoyaltyPoints(ctx context.Context, accountID uuid.UUID, points int) error

	// LoyaltyTransaction operations
	CreateLoyaltyTransaction(ctx context.Context, tx *LoyaltyTransaction) error
	ListLoyaltyTransactions(ctx context.Context, accountID uuid.UUID, limit, offset int) ([]LoyaltyTransaction, int, error)

	// DeliveryZone operations
	ListActiveDeliveryZones(ctx context.Context, tenantID uuid.UUID, outletID *uuid.UUID) ([]DeliveryZone, error)

	// OutletRating operations
	GetOutletRating(ctx context.Context, tenantID, outletID uuid.UUID) (*OutletRatingData, error)
	UpsertOutletRating(ctx context.Context, rating *OutletRatingData) error

	// GroupOrder operations
	CreateGroupOrder(ctx context.Context, g *GroupOrder) error
	GetGroupOrder(ctx context.Context, id uuid.UUID) (*GroupOrder, error)
	GetGroupOrderByInviteCode(ctx context.Context, code string) (*GroupOrder, error)
	UpdateGroupOrder(ctx context.Context, g *GroupOrder) error
	CreateGroupParticipant(ctx context.Context, p *GroupParticipant) error
	ListGroupParticipants(ctx context.Context, groupOrderID uuid.UUID) ([]GroupParticipant, error)
	CountGroupParticipants(ctx context.Context, groupOrderID uuid.UUID) (int, error)
	IsGroupParticipant(ctx context.Context, groupOrderID, userID uuid.UUID) (bool, error)

	// ActivePromos returns active promo codes for a tenant, optionally filtered by outlet
	ListActivePromoCodesForOutlet(ctx context.Context, tenantID uuid.UUID, outletID *uuid.UUID) ([]PromoCode, error)

	// Global/Cross-module lookups for stock processing
	GetTenantByID(ctx context.Context, id uuid.UUID) (*Tenant, error)
}
