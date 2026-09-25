package ordering

import (
	"errors"
	"testing"
)

func TestResolvePaymentMethod(t *testing.T) {
	m, st, meta, err := resolvePaymentMethod("mpesa_manual", " sgh7k2 l9qp ")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if m != PaymentMethodMpesa || st != PaymentStatusPending {
		t.Fatalf("manual M-Pesa should be stored as mpesa/pending, got %s/%s", m, st)
	}
	if meta[metaPaymentChannel] != PaymentChannelManualMpesa || meta[metaMpesaCode] != "SGH7K2L9QP" {
		t.Fatalf("channel/code not stamped: %v", meta)
	}

	if _, _, _, err := resolvePaymentMethod("mpesa_manual", ""); !errors.Is(err, ErrMpesaCodeRequired) {
		t.Fatalf("missing code must be rejected, got %v", err)
	}
	if _, _, _, err := resolvePaymentMethod("mpesa_manual", "ABC"); !errors.Is(err, ErrMpesaCodeInvalid) {
		t.Fatalf("short code must be rejected, got %v", err)
	}
	if m, st, meta, _ := resolvePaymentMethod("cod", ""); m != PaymentMethodCOD || st != "cod_pending" || meta != nil {
		t.Fatalf("cod mapping wrong: %s %s %v", m, st, meta)
	}
	if m, _, _, _ := resolvePaymentMethod("", ""); m != PaymentMethodMpesa {
		t.Fatalf("default should stay integrated mpesa, got %s", m)
	}
	if m, _, _, _ := resolvePaymentMethod("paystack", ""); m != PaymentMethodPaystack {
		t.Fatalf("paystack passthrough wrong: %s", m)
	}
}

func TestIsOfflinePayment(t *testing.T) {
	if !isOfflinePayment(&Order{PaymentMethod: PaymentMethodCOD}) {
		t.Fatal("COD is offline")
	}
	manual := &Order{PaymentMethod: PaymentMethodMpesa, Metadata: map[string]interface{}{metaPaymentChannel: PaymentChannelManualMpesa}}
	if !isOfflinePayment(manual) {
		t.Fatal("manual M-Pesa is offline")
	}
	if isOfflinePayment(&Order{PaymentMethod: PaymentMethodMpesa}) {
		t.Fatal("integrated M-Pesa confirms itself via the callback")
	}
}

func TestCODSettlement(t *testing.T) {
	if m, ref := codSettlement(&Order{}); m != "cod" || ref != "" {
		t.Fatalf("default is cash: %s %s", m, ref)
	}
	o := &Order{Metadata: map[string]interface{}{metaCODMethod: "mpesa", metaCODReference: "QWE1234567"}}
	if m, ref := codSettlement(o); m != PaymentChannelManualMpesa || ref != "QWE1234567" {
		t.Fatalf("M-Pesa at the door must settle as mpesa_manual with the code: %s %s", m, ref)
	}
	if m, _ := codSettlement(&Order{Metadata: map[string]interface{}{metaCODMethod: "cash"}}); m != "cod" {
		t.Fatalf("cash must settle as cod, got %s", m)
	}
}

func TestNormalizeMpesaCode(t *testing.T) {
	if got := NormalizeMpesaCode(" qwe 123 4567 "); got != "QWE1234567" {
		t.Fatalf("got %q", got)
	}
}
