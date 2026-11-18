package ordering

import (
	"context"
	"strings"

	"entgo.io/ent/dialect/sql"
	"github.com/google/uuid"

	"github.com/bengobox/ordering-backend/internal/ent/order"
	"github.com/bengobox/ordering-backend/internal/ent/predicate"
)

// TenantSlug returns the tenant's slug, or "" when the tenant is unknown here.
func (s *OrderService) TenantSlug(ctx context.Context, tenantID uuid.UUID) string {
	if t, err := s.repo.GetTenantByID(ctx, tenantID); err == nil && t != nil {
		return t.Slug
	}
	return ""
}

// minLinkPhoneDigits keeps a short or partial number from matching other people's orders.
const minLinkPhoneDigits = 9

// LinkGuestOrders attaches the tenant's guest orders placed with this email or phone to the
// customer, so a guest who later signs up sees their past orders. Callers pass only a verified
// email and a verified phone. Orders already linked to someone are left alone, which also makes
// it safe to run twice. Returns how many orders were linked.
func (r *EntRepository) LinkGuestOrders(ctx context.Context, tenantID, customerID uuid.UUID, email, phone string) (int, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	digits := digitsOnly(phone)
	var match []predicate.Order
	if email != "" {
		match = append(match, guestContactEmailIs(email))
	}
	if len(digits) >= minLinkPhoneDigits {
		match = append(match, guestContactPhoneDigitsIs(digits))
	}
	if len(match) == 0 || customerID == uuid.Nil || tenantID == uuid.Nil {
		return 0, nil
	}
	// Select first, then update by id: the JSON predicates stay in a plain SELECT, and the update
	// re-checks customer_id so a concurrent link is never overwritten.
	ids, err := r.client.Order.Query().
		Where(
			order.TenantID(tenantID),
			order.CustomerIDIsNil(),
			metadataContains(map[string]any{"guest": true}),
			order.Or(match...),
		).
		IDs(ctx)
	if err != nil || len(ids) == 0 {
		return 0, err
	}
	return r.client.Order.Update().
		Where(order.IDIn(ids...), order.TenantID(tenantID), order.CustomerIDIsNil()).
		SetCustomerID(customerID).
		Save(ctx)
}

// guestContactEmailIs matches orders whose guest email equals email, ignoring case.
func guestContactEmailIs(email string) predicate.Order {
	return func(s *sql.Selector) {
		s.Where(sql.P(func(b *sql.Builder) {
			b.WriteString("lower(").WriteString(s.C(order.FieldMetadata)).WriteString(" ->> 'contactEmail') = ").Arg(email)
		}))
	}
}

// guestContactPhoneDigitsIs matches orders whose guest phone has the same digits, so
// "+254 712 345 678" and "+254712345678" are one number.
func guestContactPhoneDigitsIs(digits string) predicate.Order {
	return func(s *sql.Selector) {
		s.Where(sql.P(func(b *sql.Builder) {
			b.WriteString("regexp_replace(").WriteString(s.C(order.FieldMetadata)).
				WriteString(" ->> 'contactPhone', '[^0-9]', '', 'g') = ").Arg(digits)
		}))
	}
}

func digitsOnly(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}
