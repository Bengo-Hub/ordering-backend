package payments

import "errors"

// Domain errors for the payments module.
var (
	ErrPaymentMethodNotFound      = errors.New("payment method not found")
	ErrPaymentIntentNotFound      = errors.New("payment intent not found")
	ErrPaymentNotFound            = errors.New("payment not found")
	ErrRefundNotFound             = errors.New("refund not found")
	ErrTreasuryEventNotFound      = errors.New("treasury event not found")
	ErrDuplicatePaymentMethod     = errors.New("payment method already exists")
	ErrDuplicatePaymentIntent     = errors.New("duplicate payment intent (idempotency key already used)")
	ErrDuplicateTreasuryEvent     = errors.New("duplicate treasury event")
	ErrInvalidPaymentProvider     = errors.New("invalid payment provider")
	ErrInvalidPaymentMethod       = errors.New("invalid payment method")
	ErrInvalidAmount              = errors.New("invalid amount")
	ErrInvalidPhoneNumber         = errors.New("invalid phone number")
	ErrPaymentIntentExpired       = errors.New("payment intent has expired")
	ErrPaymentIntentNotPending    = errors.New("payment intent is not pending")
	ErrPaymentAlreadyProcessed    = errors.New("payment has already been processed")
	ErrPaymentNotSucceeded        = errors.New("payment has not succeeded")
	ErrRefundAmountExceedsPayment = errors.New("refund amount exceeds payment amount")
	ErrRefundAlreadyProcessed     = errors.New("refund has already been processed")
	ErrInvalidWebhookSignature    = errors.New("invalid webhook signature")
	ErrWebhookSignatureExpired    = errors.New("webhook signature has expired")
	ErrTreasuryServiceUnavailable = errors.New("treasury service is unavailable")
	ErrOrderNotFound              = errors.New("order not found")
	ErrUnauthorized               = errors.New("unauthorized")
	ErrPaymentMethodsNotStored    = errors.New("payment methods are owned by treasury-api; use treasury-api for CRUD")
)
