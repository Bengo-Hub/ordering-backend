package payments

import (
	"context"

	"github.com/google/uuid"

	"github.com/bengobox/ordering-backend/internal/ent"
	"github.com/bengobox/ordering-backend/internal/ent/payment"
	"github.com/bengobox/ordering-backend/internal/ent/paymentintent"
	"github.com/bengobox/ordering-backend/internal/ent/paymentmethod"
	"github.com/bengobox/ordering-backend/internal/ent/refund"
	"github.com/bengobox/ordering-backend/internal/ent/treasuryevent"
)

// EntRepository implements Repository using Ent ORM.
type EntRepository struct {
	client *ent.Client
}

// NewEntRepository creates a new Ent-based repository.
func NewEntRepository(client *ent.Client) *EntRepository {
	return &EntRepository{client: client}
}

// ---- Payment Methods ----

func (r *EntRepository) CreatePaymentMethod(ctx context.Context, method *PaymentMethod) error {
	builder := r.client.PaymentMethod.Create().
		SetTenantID(method.TenantID).
		SetUserID(method.UserID).
		SetProvider(paymentmethod.Provider(method.Provider)).
		SetType(paymentmethod.Type(method.Type)).
		SetIsDefault(method.IsDefault)

	if method.Mask != "" {
		builder.SetMask(method.Mask)
	}
	if method.Label != "" {
		builder.SetLabel(method.Label)
	}
	if method.ExpMonth != nil {
		builder.SetExpMonth(*method.ExpMonth)
	}
	if method.ExpYear != nil {
		builder.SetExpYear(*method.ExpYear)
	}
	if method.Fingerprint != "" {
		builder.SetFingerprint(method.Fingerprint)
	}
	if method.ProviderToken != "" {
		builder.SetProviderToken(method.ProviderToken)
	}
	if method.Metadata != nil {
		builder.SetMetadata(method.Metadata)
	}

	created, err := builder.Save(ctx)
	if err != nil {
		if ent.IsConstraintError(err) {
			return ErrDuplicatePaymentMethod
		}
		return err
	}

	method.ID = created.ID
	method.CreatedAt = created.CreatedAt
	method.UpdatedAt = created.UpdatedAt
	return nil
}

func (r *EntRepository) GetPaymentMethod(ctx context.Context, tenantID, id uuid.UUID) (*PaymentMethod, error) {
	pm, err := r.client.PaymentMethod.Query().
		Where(
			paymentmethod.ID(id),
			paymentmethod.TenantID(tenantID),
		).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, ErrPaymentMethodNotFound
		}
		return nil, err
	}
	return mapEntPaymentMethod(pm), nil
}

func (r *EntRepository) GetPaymentMethodByFingerprint(ctx context.Context, tenantID uuid.UUID, fingerprint string) (*PaymentMethod, error) {
	pm, err := r.client.PaymentMethod.Query().
		Where(
			paymentmethod.TenantID(tenantID),
			paymentmethod.Fingerprint(fingerprint),
		).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, ErrPaymentMethodNotFound
		}
		return nil, err
	}
	return mapEntPaymentMethod(pm), nil
}

func (r *EntRepository) ListPaymentMethods(ctx context.Context, filter PaymentMethodFilter) ([]PaymentMethod, int, error) {
	query := r.client.PaymentMethod.Query().
		Where(paymentmethod.TenantID(filter.TenantID), paymentmethod.UserID(filter.UserID))

	if filter.Provider != nil {
		query = query.Where(paymentmethod.ProviderEQ(paymentmethod.Provider(*filter.Provider)))
	}
	if filter.Type != nil {
		query = query.Where(paymentmethod.TypeEQ(paymentmethod.Type(*filter.Type)))
	}

	total, err := query.Clone().Count(ctx)
	if err != nil {
		return nil, 0, err
	}

	if filter.Limit > 0 {
		query = query.Limit(filter.Limit)
	}
	if filter.Offset > 0 {
		query = query.Offset(filter.Offset)
	}

	methods, err := query.Order(ent.Desc(paymentmethod.FieldCreatedAt)).All(ctx)
	if err != nil {
		return nil, 0, err
	}

	result := make([]PaymentMethod, len(methods))
	for i, m := range methods {
		result[i] = *mapEntPaymentMethod(m)
	}
	return result, total, nil
}

func (r *EntRepository) UpdatePaymentMethod(ctx context.Context, method *PaymentMethod) error {
	builder := r.client.PaymentMethod.UpdateOneID(method.ID).
		SetIsDefault(method.IsDefault)

	if method.Label != "" {
		builder.SetLabel(method.Label)
	}
	if method.Metadata != nil {
		builder.SetMetadata(method.Metadata)
	}

	_, err := builder.Save(ctx)
	return err
}

func (r *EntRepository) DeletePaymentMethod(ctx context.Context, tenantID, id uuid.UUID) error {
	_, err := r.client.PaymentMethod.Delete().
		Where(
			paymentmethod.ID(id),
			paymentmethod.TenantID(tenantID),
		).
		Exec(ctx)
	return err
}

func (r *EntRepository) SetDefaultPaymentMethod(ctx context.Context, tenantID, userID, id uuid.UUID) error {
	// First, unset all defaults for the user
	_, err := r.client.PaymentMethod.Update().
		Where(
			paymentmethod.TenantID(tenantID),
			paymentmethod.UserID(userID),
		).
		SetIsDefault(false).
		Save(ctx)
	if err != nil {
		return err
	}

	// Then, set the specified method as default
	_, err = r.client.PaymentMethod.UpdateOneID(id).
		SetIsDefault(true).
		Save(ctx)
	return err
}

// ---- Payment Intents ----

func (r *EntRepository) CreatePaymentIntent(ctx context.Context, intent *PaymentIntent) error {
	builder := r.client.PaymentIntent.Create().
		SetTenantID(intent.TenantID).
		SetOrderID(intent.OrderID).
		SetProvider(paymentintent.Provider(intent.Provider)).
		SetStatus(paymentintent.Status(intent.Status)).
		SetAmount(intent.Amount).
		SetCurrency(intent.Currency)

	if intent.PaymentMethodID != nil {
		builder.SetPaymentMethodID(*intent.PaymentMethodID)
	}
	if intent.ProviderIntentID != "" {
		builder.SetProviderIntentID(intent.ProviderIntentID)
	}
	if intent.ClientSecret != "" {
		builder.SetClientSecret(intent.ClientSecret)
	}
	if intent.Description != "" {
		builder.SetDescription(intent.Description)
	}
	if intent.IdempotencyKey != "" {
		builder.SetIdempotencyKey(intent.IdempotencyKey)
	}
	if intent.MpesaCheckoutRequestID != "" {
		builder.SetMpesaCheckoutRequestID(intent.MpesaCheckoutRequestID)
	}
	if intent.MpesaPhoneNumber != "" {
		builder.SetMpesaPhoneNumber(intent.MpesaPhoneNumber)
	}
	if intent.ExpiresAt != nil {
		builder.SetExpiresAt(*intent.ExpiresAt)
	}
	if intent.Metadata != nil {
		builder.SetMetadata(intent.Metadata)
	}

	created, err := builder.Save(ctx)
	if err != nil {
		if ent.IsConstraintError(err) {
			return ErrDuplicatePaymentIntent
		}
		return err
	}

	intent.ID = created.ID
	intent.CreatedAt = created.CreatedAt
	intent.UpdatedAt = created.UpdatedAt
	return nil
}

func (r *EntRepository) GetPaymentIntent(ctx context.Context, tenantID, id uuid.UUID) (*PaymentIntent, error) {
	pi, err := r.client.PaymentIntent.Query().
		Where(
			paymentintent.ID(id),
			paymentintent.TenantID(tenantID),
		).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, ErrPaymentIntentNotFound
		}
		return nil, err
	}
	return mapEntPaymentIntent(pi), nil
}

func (r *EntRepository) GetPaymentIntentByIdempotencyKey(ctx context.Context, tenantID uuid.UUID, key string) (*PaymentIntent, error) {
	pi, err := r.client.PaymentIntent.Query().
		Where(
			paymentintent.TenantID(tenantID),
			paymentintent.IdempotencyKey(key),
		).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, ErrPaymentIntentNotFound
		}
		return nil, err
	}
	return mapEntPaymentIntent(pi), nil
}

func (r *EntRepository) GetPaymentIntentByMpesaCheckoutRequestID(ctx context.Context, checkoutRequestID string) (*PaymentIntent, error) {
	pi, err := r.client.PaymentIntent.Query().
		Where(paymentintent.MpesaCheckoutRequestID(checkoutRequestID)).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, ErrPaymentIntentNotFound
		}
		return nil, err
	}
	return mapEntPaymentIntent(pi), nil
}

func (r *EntRepository) ListPaymentIntents(ctx context.Context, filter PaymentIntentFilter) ([]PaymentIntent, int, error) {
	query := r.client.PaymentIntent.Query().
		Where(paymentintent.TenantID(filter.TenantID))

	if filter.OrderID != nil {
		query = query.Where(paymentintent.OrderID(*filter.OrderID))
	}
	if filter.Status != nil {
		query = query.Where(paymentintent.StatusEQ(paymentintent.Status(*filter.Status)))
	}
	if filter.Provider != nil {
		query = query.Where(paymentintent.ProviderEQ(paymentintent.Provider(*filter.Provider)))
	}

	total, err := query.Clone().Count(ctx)
	if err != nil {
		return nil, 0, err
	}

	if filter.Limit > 0 {
		query = query.Limit(filter.Limit)
	}
	if filter.Offset > 0 {
		query = query.Offset(filter.Offset)
	}

	intents, err := query.Order(ent.Desc(paymentintent.FieldCreatedAt)).All(ctx)
	if err != nil {
		return nil, 0, err
	}

	result := make([]PaymentIntent, len(intents))
	for i, pi := range intents {
		result[i] = *mapEntPaymentIntent(pi)
	}
	return result, total, nil
}

func (r *EntRepository) UpdatePaymentIntent(ctx context.Context, intent *PaymentIntent) error {
	builder := r.client.PaymentIntent.UpdateOneID(intent.ID).
		SetStatus(paymentintent.Status(intent.Status)).
		SetRetryCount(intent.RetryCount)

	if intent.ProviderIntentID != "" {
		builder.SetProviderIntentID(intent.ProviderIntentID)
	}
	if intent.ClientSecret != "" {
		builder.SetClientSecret(intent.ClientSecret)
	}
	if intent.MpesaCheckoutRequestID != "" {
		builder.SetMpesaCheckoutRequestID(intent.MpesaCheckoutRequestID)
	}
	if intent.LastRetryAt != nil {
		builder.SetLastRetryAt(*intent.LastRetryAt)
	}
	if intent.ErrorMessage != "" {
		builder.SetErrorMessage(intent.ErrorMessage)
	}
	if intent.ErrorCode != "" {
		builder.SetErrorCode(intent.ErrorCode)
	}

	_, err := builder.Save(ctx)
	return err
}

// ---- Payments ----

func (r *EntRepository) CreatePayment(ctx context.Context, p *Payment) error {
	builder := r.client.Payment.Create().
		SetTenantID(p.TenantID).
		SetPaymentIntentID(p.PaymentIntentID).
		SetOrderID(p.OrderID).
		SetAmount(p.Amount).
		SetCurrency(p.Currency).
		SetStatus(payment.Status(p.Status)).
		SetProvider(payment.Provider(p.Provider))

	if p.ProviderReference != "" {
		builder.SetProviderReference(p.ProviderReference)
	}
	if p.ProviderReceipt != "" {
		builder.SetProviderReceipt(p.ProviderReceipt)
	}
	if p.MpesaTransactionID != "" {
		builder.SetMpesaTransactionID(p.MpesaTransactionID)
	}
	if p.MpesaPhoneNumber != "" {
		builder.SetMpesaPhoneNumber(p.MpesaPhoneNumber)
	}
	if p.ProviderResponse != nil {
		builder.SetProviderResponse(p.ProviderResponse)
	}
	if p.Metadata != nil {
		builder.SetMetadata(p.Metadata)
	}
	if p.ProcessedAt != nil {
		builder.SetProcessedAt(*p.ProcessedAt)
	}
	if p.CapturedAt != nil {
		builder.SetCapturedAt(*p.CapturedAt)
	}

	created, err := builder.Save(ctx)
	if err != nil {
		return err
	}

	p.ID = created.ID
	p.CreatedAt = created.CreatedAt
	p.UpdatedAt = created.UpdatedAt
	return nil
}

func (r *EntRepository) GetPayment(ctx context.Context, tenantID, id uuid.UUID) (*Payment, error) {
	p, err := r.client.Payment.Query().
		Where(
			payment.ID(id),
			payment.TenantID(tenantID),
		).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, ErrPaymentNotFound
		}
		return nil, err
	}
	return mapEntPayment(p), nil
}

func (r *EntRepository) GetPaymentByProviderReference(ctx context.Context, provider PaymentProvider, reference string) (*Payment, error) {
	p, err := r.client.Payment.Query().
		Where(
			payment.ProviderEQ(payment.Provider(provider)),
			payment.ProviderReference(reference),
		).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, ErrPaymentNotFound
		}
		return nil, err
	}
	return mapEntPayment(p), nil
}

func (r *EntRepository) GetPaymentByOrderID(ctx context.Context, tenantID, orderID uuid.UUID) (*Payment, error) {
	p, err := r.client.Payment.Query().
		Where(
			payment.TenantID(tenantID),
			payment.OrderID(orderID),
			payment.StatusEQ(payment.StatusSucceeded),
		).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, ErrPaymentNotFound
		}
		return nil, err
	}
	return mapEntPayment(p), nil
}

func (r *EntRepository) ListPayments(ctx context.Context, filter PaymentFilter) ([]Payment, int, error) {
	query := r.client.Payment.Query().
		Where(payment.TenantID(filter.TenantID))

	if filter.OrderID != nil {
		query = query.Where(payment.OrderID(*filter.OrderID))
	}
	if filter.Status != nil {
		query = query.Where(payment.StatusEQ(payment.Status(*filter.Status)))
	}
	if filter.Provider != nil {
		query = query.Where(payment.ProviderEQ(payment.Provider(*filter.Provider)))
	}
	if filter.DateFrom != nil {
		query = query.Where(payment.CreatedAtGTE(*filter.DateFrom))
	}
	if filter.DateTo != nil {
		query = query.Where(payment.CreatedAtLTE(*filter.DateTo))
	}

	total, err := query.Clone().Count(ctx)
	if err != nil {
		return nil, 0, err
	}

	if filter.Limit > 0 {
		query = query.Limit(filter.Limit)
	}
	if filter.Offset > 0 {
		query = query.Offset(filter.Offset)
	}

	payments, err := query.Order(ent.Desc(payment.FieldCreatedAt)).All(ctx)
	if err != nil {
		return nil, 0, err
	}

	result := make([]Payment, len(payments))
	for i, p := range payments {
		result[i] = *mapEntPayment(p)
	}
	return result, total, nil
}

func (r *EntRepository) UpdatePayment(ctx context.Context, p *Payment) error {
	builder := r.client.Payment.UpdateOneID(p.ID).
		SetStatus(payment.Status(p.Status)).
		SetRefundedAmount(p.RefundedAmount)

	if p.ProviderReference != "" {
		builder.SetProviderReference(p.ProviderReference)
	}
	if p.ProviderReceipt != "" {
		builder.SetProviderReceipt(p.ProviderReceipt)
	}
	if p.ProcessedAt != nil {
		builder.SetProcessedAt(*p.ProcessedAt)
	}
	if p.CapturedAt != nil {
		builder.SetCapturedAt(*p.CapturedAt)
	}

	_, err := builder.Save(ctx)
	return err
}

// ---- Refunds ----

func (r *EntRepository) CreateRefund(ctx context.Context, ref *Refund) error {
	builder := r.client.Refund.Create().
		SetTenantID(ref.TenantID).
		SetPaymentID(ref.PaymentID).
		SetOrderID(ref.OrderID).
		SetAmount(ref.Amount).
		SetCurrency(ref.Currency).
		SetStatus(refund.Status(ref.Status)).
		SetReason(refund.Reason(ref.Reason)).
		SetProvider(refund.Provider(ref.Provider)).
		SetRequestedAt(ref.RequestedAt)

	if ref.ReasonNotes != "" {
		builder.SetReasonNotes(ref.ReasonNotes)
	}
	if ref.RequestedBy != nil {
		builder.SetRequestedBy(*ref.RequestedBy)
	}
	if ref.Metadata != nil {
		builder.SetMetadata(ref.Metadata)
	}

	created, err := builder.Save(ctx)
	if err != nil {
		return err
	}

	ref.ID = created.ID
	ref.CreatedAt = created.CreatedAt
	ref.UpdatedAt = created.UpdatedAt
	return nil
}

func (r *EntRepository) GetRefund(ctx context.Context, tenantID, id uuid.UUID) (*Refund, error) {
	ref, err := r.client.Refund.Query().
		Where(
			refund.ID(id),
			refund.TenantID(tenantID),
		).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, ErrRefundNotFound
		}
		return nil, err
	}
	return mapEntRefund(ref), nil
}

func (r *EntRepository) ListRefunds(ctx context.Context, filter RefundFilter) ([]Refund, int, error) {
	query := r.client.Refund.Query().
		Where(refund.TenantID(filter.TenantID))

	if filter.PaymentID != nil {
		query = query.Where(refund.PaymentID(*filter.PaymentID))
	}
	if filter.OrderID != nil {
		query = query.Where(refund.OrderID(*filter.OrderID))
	}
	if filter.Status != nil {
		query = query.Where(refund.StatusEQ(refund.Status(*filter.Status)))
	}

	total, err := query.Clone().Count(ctx)
	if err != nil {
		return nil, 0, err
	}

	if filter.Limit > 0 {
		query = query.Limit(filter.Limit)
	}
	if filter.Offset > 0 {
		query = query.Offset(filter.Offset)
	}

	refunds, err := query.Order(ent.Desc(refund.FieldCreatedAt)).All(ctx)
	if err != nil {
		return nil, 0, err
	}

	result := make([]Refund, len(refunds))
	for i, ref := range refunds {
		result[i] = *mapEntRefund(ref)
	}
	return result, total, nil
}

func (r *EntRepository) UpdateRefund(ctx context.Context, ref *Refund) error {
	builder := r.client.Refund.UpdateOneID(ref.ID).
		SetStatus(refund.Status(ref.Status))

	if ref.ProviderRefundID != "" {
		builder.SetProviderRefundID(ref.ProviderRefundID)
	}
	if ref.ProviderReference != "" {
		builder.SetProviderReference(ref.ProviderReference)
	}
	if ref.ApprovedBy != nil {
		builder.SetApprovedBy(*ref.ApprovedBy)
	}
	if ref.ApprovedAt != nil {
		builder.SetApprovedAt(*ref.ApprovedAt)
	}
	if ref.ProcessedAt != nil {
		builder.SetProcessedAt(*ref.ProcessedAt)
	}
	if ref.ErrorMessage != "" {
		builder.SetErrorMessage(ref.ErrorMessage)
	}
	if ref.ErrorCode != "" {
		builder.SetErrorCode(ref.ErrorCode)
	}

	_, err := builder.Save(ctx)
	return err
}

// ---- Treasury Events ----

func (r *EntRepository) CreateTreasuryEvent(ctx context.Context, event *TreasuryEvent) error {
	builder := r.client.TreasuryEvent.Create().
		SetExternalID(event.ExternalID).
		SetEventType(treasuryevent.EventType(event.EventType)).
		SetPayload(event.Payload).
		SetStatus(treasuryevent.Status(event.Status)).
		SetReceivedAt(event.ReceivedAt)

	if event.TenantID != nil {
		builder.SetTenantID(*event.TenantID)
	}
	if event.Provider != nil {
		builder.SetProvider(treasuryevent.Provider(*event.Provider))
	}
	if event.OrderID != nil {
		builder.SetOrderID(*event.OrderID)
	}
	if event.PaymentID != nil {
		builder.SetPaymentID(*event.PaymentID)
	}
	if event.PaymentIntentID != nil {
		builder.SetPaymentIntentID(*event.PaymentIntentID)
	}
	if event.RefundID != nil {
		builder.SetRefundID(*event.RefundID)
	}
	if event.Headers != nil {
		builder.SetHeaders(event.Headers)
	}
	if event.Signature != "" {
		builder.SetSignature(event.Signature)
	}
	if event.SignatureValid != nil {
		builder.SetSignatureValid(*event.SignatureValid)
	}
	if event.IPAddress != "" {
		builder.SetIPAddress(event.IPAddress)
	}

	created, err := builder.Save(ctx)
	if err != nil {
		if ent.IsConstraintError(err) {
			return ErrDuplicateTreasuryEvent
		}
		return err
	}

	event.ID = created.ID
	event.CreatedAt = created.CreatedAt
	event.UpdatedAt = created.UpdatedAt
	return nil
}

func (r *EntRepository) GetTreasuryEvent(ctx context.Context, id uuid.UUID) (*TreasuryEvent, error) {
	event, err := r.client.TreasuryEvent.Get(ctx, id)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, ErrTreasuryEventNotFound
		}
		return nil, err
	}
	return mapEntTreasuryEvent(event), nil
}

func (r *EntRepository) GetTreasuryEventByExternalID(ctx context.Context, externalID string) (*TreasuryEvent, error) {
	event, err := r.client.TreasuryEvent.Query().
		Where(treasuryevent.ExternalID(externalID)).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, ErrTreasuryEventNotFound
		}
		return nil, err
	}
	return mapEntTreasuryEvent(event), nil
}

func (r *EntRepository) UpdateTreasuryEvent(ctx context.Context, event *TreasuryEvent) error {
	builder := r.client.TreasuryEvent.UpdateOneID(event.ID).
		SetStatus(treasuryevent.Status(event.Status)).
		SetRetryCount(event.RetryCount)

	if event.PaymentID != nil {
		builder.SetPaymentID(*event.PaymentID)
	}
	if event.PaymentIntentID != nil {
		builder.SetPaymentIntentID(*event.PaymentIntentID)
	}
	if event.RefundID != nil {
		builder.SetRefundID(*event.RefundID)
	}
	if event.LastRetryAt != nil {
		builder.SetLastRetryAt(*event.LastRetryAt)
	}
	if event.ProcessedAt != nil {
		builder.SetProcessedAt(*event.ProcessedAt)
	}
	if event.ErrorMessage != "" {
		builder.SetErrorMessage(event.ErrorMessage)
	}
	if event.ErrorCode != "" {
		builder.SetErrorCode(event.ErrorCode)
	}

	_, err := builder.Save(ctx)
	return err
}

func (r *EntRepository) ListPendingTreasuryEvents(ctx context.Context, limit int) ([]TreasuryEvent, error) {
	events, err := r.client.TreasuryEvent.Query().
		Where(
			treasuryevent.StatusIn(treasuryevent.StatusPending, treasuryevent.StatusFailed),
			treasuryevent.RetryCountLT(5),
		).
		Order(ent.Asc(treasuryevent.FieldCreatedAt)).
		Limit(limit).
		All(ctx)
	if err != nil {
		return nil, err
	}

	result := make([]TreasuryEvent, len(events))
	for i, e := range events {
		result[i] = *mapEntTreasuryEvent(e)
	}
	return result, nil
}

// ---- Mappers ----

func mapEntPaymentMethod(pm *ent.PaymentMethod) *PaymentMethod {
	return &PaymentMethod{
		ID:            pm.ID,
		TenantID:      pm.TenantID,
		UserID:        pm.UserID,
		Provider:      PaymentProvider(pm.Provider),
		Type:          PaymentMethodType(pm.Type),
		Mask:          pm.Mask,
		Label:         pm.Label,
		ExpMonth:      pm.ExpMonth,
		ExpYear:       pm.ExpYear,
		IsDefault:     pm.IsDefault,
		Fingerprint:   pm.Fingerprint,
		ProviderToken: pm.ProviderToken,
		Metadata:      pm.Metadata,
		CreatedAt:     pm.CreatedAt,
		UpdatedAt:     pm.UpdatedAt,
	}
}

func mapEntPaymentIntent(pi *ent.PaymentIntent) *PaymentIntent {
	intent := &PaymentIntent{
		ID:                     pi.ID,
		TenantID:               pi.TenantID,
		OrderID:                pi.OrderID,
		Provider:               PaymentProvider(pi.Provider),
		ProviderIntentID:       pi.ProviderIntentID,
		ClientSecret:           pi.ClientSecret,
		Status:                 PaymentIntentStatus(pi.Status),
		Amount:                 pi.Amount,
		Currency:               pi.Currency,
		Description:            pi.Description,
		IdempotencyKey:         pi.IdempotencyKey,
		MpesaCheckoutRequestID: pi.MpesaCheckoutRequestID,
		MpesaPhoneNumber:       pi.MpesaPhoneNumber,
		RetryCount:             pi.RetryCount,
		ErrorMessage:           pi.ErrorMessage,
		ErrorCode:              pi.ErrorCode,
		Metadata:               pi.Metadata,
		CreatedAt:              pi.CreatedAt,
		UpdatedAt:              pi.UpdatedAt,
	}
	if pi.PaymentMethodID != nil {
		intent.PaymentMethodID = pi.PaymentMethodID
	}
	if pi.LastRetryAt != nil {
		intent.LastRetryAt = pi.LastRetryAt
	}
	if pi.ExpiresAt != nil {
		intent.ExpiresAt = pi.ExpiresAt
	}
	return intent
}

func mapEntPayment(p *ent.Payment) *Payment {
	payment := &Payment{
		ID:                 p.ID,
		TenantID:           p.TenantID,
		PaymentIntentID:    p.PaymentIntentID,
		OrderID:            p.OrderID,
		Amount:             p.Amount,
		Currency:           p.Currency,
		Status:             PaymentStatus(p.Status),
		Provider:           PaymentProvider(p.Provider),
		ProviderReference:  p.ProviderReference,
		ProviderReceipt:    p.ProviderReceipt,
		MpesaTransactionID: p.MpesaTransactionID,
		MpesaPhoneNumber:   p.MpesaPhoneNumber,
		RefundedAmount:     p.RefundedAmount,
		ProviderResponse:   p.ProviderResponse,
		Metadata:           p.Metadata,
		CreatedAt:          p.CreatedAt,
		UpdatedAt:          p.UpdatedAt,
	}
	if p.ProcessedAt != nil {
		payment.ProcessedAt = p.ProcessedAt
	}
	if p.CapturedAt != nil {
		payment.CapturedAt = p.CapturedAt
	}
	return payment
}

func mapEntRefund(ref *ent.Refund) *Refund {
	refund := &Refund{
		ID:                ref.ID,
		TenantID:          ref.TenantID,
		PaymentID:         ref.PaymentID,
		OrderID:           ref.OrderID,
		Amount:            ref.Amount,
		Currency:          ref.Currency,
		Status:            RefundStatus(ref.Status),
		Reason:            RefundReason(ref.Reason),
		ReasonNotes:       ref.ReasonNotes,
		Provider:          PaymentProvider(ref.Provider),
		ProviderRefundID:  ref.ProviderRefundID,
		ProviderReference: ref.ProviderReference,
		ErrorMessage:      ref.ErrorMessage,
		ErrorCode:         ref.ErrorCode,
		ProviderResponse:  ref.ProviderResponse,
		Metadata:          ref.Metadata,
		RequestedAt:       ref.RequestedAt,
		CreatedAt:         ref.CreatedAt,
		UpdatedAt:         ref.UpdatedAt,
	}
	if ref.RequestedBy != nil {
		refund.RequestedBy = ref.RequestedBy
	}
	if ref.ApprovedBy != nil {
		refund.ApprovedBy = ref.ApprovedBy
	}
	if ref.ApprovedAt != nil {
		refund.ApprovedAt = ref.ApprovedAt
	}
	if ref.ProcessedAt != nil {
		refund.ProcessedAt = ref.ProcessedAt
	}
	return refund
}

func mapEntTreasuryEvent(e *ent.TreasuryEvent) *TreasuryEvent {
	event := &TreasuryEvent{
		ID:           e.ID,
		ExternalID:   e.ExternalID,
		EventType:    string(e.EventType),
		Payload:      e.Payload,
		Headers:      e.Headers,
		Signature:    e.Signature,
		Status:       TreasuryEventStatus(e.Status),
		RetryCount:   e.RetryCount,
		ErrorMessage: e.ErrorMessage,
		ErrorCode:    e.ErrorCode,
		IPAddress:    e.IPAddress,
		ReceivedAt:   e.ReceivedAt,
		CreatedAt:    e.CreatedAt,
		UpdatedAt:    e.UpdatedAt,
	}
	if e.TenantID != nil {
		event.TenantID = e.TenantID
	}
	if e.Provider != "" {
		provider := PaymentProvider(e.Provider)
		event.Provider = &provider
	}
	if e.OrderID != nil {
		event.OrderID = e.OrderID
	}
	if e.PaymentID != nil {
		event.PaymentID = e.PaymentID
	}
	if e.PaymentIntentID != nil {
		event.PaymentIntentID = e.PaymentIntentID
	}
	if e.RefundID != nil {
		event.RefundID = e.RefundID
	}
	if e.SignatureValid != nil {
		event.SignatureValid = e.SignatureValid
	}
	if e.LastRetryAt != nil {
		event.LastRetryAt = e.LastRetryAt
	}
	if e.ProcessedAt != nil {
		event.ProcessedAt = e.ProcessedAt
	}
	return event
}
