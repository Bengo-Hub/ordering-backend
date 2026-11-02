package ordering

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/bengobox/ordering-backend/internal/platform/treasury"
)

func TestPayLaterEnabled(t *testing.T) {
	cases := []struct {
		name     string
		gateways []treasury.EnabledGateway
		want     bool
	}{
		{"cod enabled", []treasury.EnabledGateway{"mpesa", "cod"}, true},
		{"case and spaces", []treasury.EnabledGateway{" COD "}, true},
		{"pay now only", []treasury.EnabledGateway{"mpesa", "paystack"}, false},
		{"nothing enabled", nil, false},
	}
	for _, c := range cases {
		if got := payLaterEnabled(c.gateways); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}

func TestEnsurePayLaterAcceptedSkipsPayNow(t *testing.T) {
	// Pay-now methods never need the check, and no treasury client means nothing to ask.
	s := &OrderService{}
	if err := s.ensurePayLaterAccepted(context.Background(), uuid.New(), PaymentMethodMpesa); err != nil {
		t.Fatalf("pay now: %v", err)
	}
	if err := s.ensurePayLaterAccepted(context.Background(), uuid.New(), PaymentMethodCOD); err != nil {
		t.Fatalf("no treasury client: %v", err)
	}
}
