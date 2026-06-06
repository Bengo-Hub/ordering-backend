package ordering

import "errors"

var (
	// Cart errors
	ErrCartNotFound        = errors.New("cart not found")
	ErrCartExpired         = errors.New("cart has expired")
	ErrCartAlreadyCheckedOut = errors.New("cart has already been checked out")
	ErrCartEmpty           = errors.New("cart is empty")
	ErrInvalidCartStatus   = errors.New("invalid cart status")

	// CartItem errors
	ErrCartItemNotFound       = errors.New("cart item not found")
	ErrInvalidQuantity        = errors.New("quantity must be positive")
	ErrCatalogItemUnavailable = errors.New("catalog item is not available")
	ErrVariantUnavailable     = errors.New("variant is not available")

	// Order errors
	ErrOrderNotFound          = errors.New("order not found")
	ErrOrderAlreadyExists     = errors.New("order with this idempotency key already exists")
	ErrInvalidOrderStatus     = errors.New("invalid order status")
	ErrInvalidStatusTransition = errors.New("invalid order status transition")
	ErrOrderCannotBeCancelled = errors.New("order cannot be cancelled in current status")
	ErrInvalidOrderNumber     = errors.New("invalid order number format")

	// Payment errors
	ErrPaymentPending     = errors.New("payment is still pending")
	ErrPaymentFailed      = errors.New("payment failed")
	ErrInvalidPaymentStatus = errors.New("invalid payment status")
	ErrNoPaymentIntent      = errors.New("order has no payment intent")
	ErrTreasuryNotConfigured = errors.New("payment service not configured")
	ErrPaymentInitiateFailed = errors.New("payment initiation failed")

	// Address errors
	ErrAddressNotFound     = errors.New("address not found")
	ErrAddressAlreadyExists = errors.New("address already exists")
	ErrInvalidAddress      = errors.New("invalid address")
	ErrMaxAddressesReached = errors.New("maximum number of addresses reached")

	// PromoCode errors
	ErrPromoCodeNotFound     = errors.New("promo code not found")
	ErrPromoCodeExpired      = errors.New("promo code has expired")
	ErrPromoCodeNotStarted   = errors.New("promo code is not yet active")
	ErrPromoCodeMaxUses      = errors.New("promo code has reached maximum uses")
	ErrPromoCodeUserLimit    = errors.New("user has reached maximum uses for this promo code")
	ErrPromoCodeMinSubtotal  = errors.New("cart subtotal does not meet minimum requirement")
	ErrPromoCodeInactive     = errors.New("promo code is not active")
	ErrPromoCodeInvalidCafe  = errors.New("promo code is not valid for this cafe")
	ErrPromoCodeAlreadyUsed  = errors.New("promo code has already been applied")

	// Loyalty errors
	ErrLoyaltyAccountNotFound   = errors.New("loyalty account not found")
	ErrInsufficientLoyaltyPoints = errors.New("insufficient loyalty points")
	ErrInvalidLoyaltyPoints     = errors.New("invalid loyalty points amount")
	ErrLoyaltyTransactionFailed = errors.New("loyalty transaction failed")

	// Checkout errors
	ErrCheckoutFailed              = errors.New("checkout failed")
	ErrInvalidDeliveryAddress      = errors.New("invalid delivery address")
	ErrDeliveryNotAvailable        = errors.New("delivery is not available to this address")

	// Rating errors
	ErrOrderNotRatable  = errors.New("order cannot be rated: must be delivered or completed")
	ErrAlreadyRated     = errors.New("order has already been rated")
	ErrInvalidRating    = errors.New("rating must be between 1 and 5")

	// OutletRating errors
	ErrOutletRatingNotFound = errors.New("outlet rating not found")
	ErrStockNotAvailable    = errors.New("stock not available for one or more items")
	ErrDeliveryNotServiceable = errors.New("delivery location is outside serviceable zones")

	// Scheduled order errors
	ErrScheduledForRequired     = errors.New("scheduled_for is required for scheduled orders")
	ErrScheduledForTooSoon      = errors.New("scheduled_for must be at least 30 minutes in the future")

	// Refund errors
	ErrOrderNotRefundable     = errors.New("order cannot be refunded: must be delivered or completed")
	ErrOrderAlreadyRefunded   = errors.New("order has already been refunded")
	ErrRefundFailed           = errors.New("refund processing failed")

	// Subscription errors
	ErrSubscriptionRequired = errors.New("active subscription required to place orders")

	// Group order errors
	ErrGroupOrderNotFound     = errors.New("group order not found")
	ErrGroupOrderExpired      = errors.New("group order has expired")
	ErrGroupOrderLocked       = errors.New("group order is locked")
	ErrGroupOrderNotOpen      = errors.New("group order is not open")
	ErrGroupOrderFull         = errors.New("group order has reached maximum participants")
	ErrAlreadyParticipant     = errors.New("user is already a participant")
	ErrNotGroupOrderHost      = errors.New("only the host can perform this action")
	ErrInvalidInviteCode      = errors.New("invalid invite code")

	// General errors
	ErrInvalidTenant   = errors.New("invalid tenant")
	ErrInvalidCafe     = errors.New("invalid cafe")
	ErrUnauthorized    = errors.New("unauthorized access")
	ErrInternalError   = errors.New("internal server error")
)
