package paymentshandler

import (
	"testing"

	"github.com/bengobox/ordering-backend/internal/platform/posdiscounts"
)

func TestApplyCheckoutPolicyWordsCashOption(t *testing.T) {
	resp := PaymentMethodsAggregateResponse{Gateways: []PaymentGatewayResponse{{Type: "cod"}, {Type: "mpesa"}}}
	applyCheckoutPolicy(&resp, "pickup")
	if resp.Gateways[0].Name != "Pay at the counter" {
		t.Fatalf("pickup label = %q", resp.Gateways[0].Name)
	}
	applyCheckoutPolicy(&resp, "delivery")
	if resp.Gateways[0].Name != "Pay on delivery" {
		t.Fatalf("delivery label = %q", resp.Gateways[0].Name)
	}
	if resp.Gateways[1].Name != "" {
		t.Fatalf("other gateways must be untouched")
	}
}

func TestManualMpesaGatewayInstructions(t *testing.T) {
	gw := manualMpesaGateway(&posdiscounts.OutletPaymentDetails{MpesaPaybill: "247247", MpesaAccountRef: "URBAN", ManualMpesaOnline: true})
	if gw.Type != "mpesa_manual" || gw.Instructions["paybill"] != "247247" || gw.Instructions["account_reference"] != "URBAN" {
		t.Fatalf("unexpected gateway: %+v", gw)
	}
	if _, ok := gw.Instructions["till"]; ok {
		t.Fatal("no till configured, none expected")
	}
}
